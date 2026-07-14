#!/usr/bin/env bash
#
# Posts pipeline status to the ACN Pipeline Notifier bot
# (https://acn-notifier-bot.azurewebsites.net). Each (source, runId) maps to
# one root Adaptive Card in Teams that mutates in place across stages, plus
# threaded replies for narrative events (approvals, failures, etc.).
#
# Usage from a pipeline step:
#
#     source .pipelines/dalec-release/scripts/notify-bot.sh
#
#     notify_status \
#       --status running --stage init \
#       --title "dalec-release | $COMPONENT $TAG" \
#       --summary "Orchestrator started." \
#       --run-url "$ORCHESTRATOR_RUN_URL"
#
#     notify_reply --text "Manual approval required: $reason"
#
# Required environment (passed as `env:` on the AzureCLI@2 task):
#   NOTIFIER_ENDPOINT      e.g. https://acn-notifier-bot.azurewebsites.net
#   NOTIFIER_API_AUDIENCE  e.g. api://3976eca5-3f9d-4528-987f-c20c1f9f27f7
#   NOTIFY_SOURCE          stable identifier, e.g. dalec-release
#   NOTIFY_RUN_ID          stable per-build id, e.g. $(Build.BuildId)
#   NOTIFY_TEAM_ID         Teams team groupId
#   NOTIFY_CHANNEL_ID      Teams channel id (19:...@thread.skype)
#
# Authentication: the calling task MUST be AzureCLI@2 with
# `addSpnToEnvironment: true` (or a previous `az login` against the WIF
# service connection). We exchange that login for an AAD bearer scoped to
# NOTIFIER_API_AUDIENCE and send it in Authorization: Bearer.
#
# Failures are logged but never fail the build — notifications are best-effort.

set -uo pipefail

notify_require_env() {
  local missing=()
  for var in NOTIFIER_ENDPOINT NOTIFIER_API_AUDIENCE NOTIFY_SOURCE NOTIFY_RUN_ID NOTIFY_TEAM_ID NOTIFY_CHANNEL_ID; do
    if [[ -z "${!var:-}" ]]; then
      missing+=("$var")
    fi
  done
  if (( ${#missing[@]} > 0 )); then
    echo "notify-bot: skipping — missing env: ${missing[*]}" >&2
    return 1
  fi
}

notify_mint_token() {
  local audience="$NOTIFIER_API_AUDIENCE"
  local token
  if ! token="$(az account get-access-token --resource "$audience" --query accessToken -o tsv 2>/dev/null)"; then
    echo "notify-bot: failed to mint AAD token for $audience" >&2
    return 1
  fi
  if [[ -z "$token" ]]; then
    echo "notify-bot: empty AAD token returned for $audience" >&2
    return 1
  fi
  printf '%s' "$token"
}

# notify_ado_link <ado-build-url>
#
# Echoes a markdown-labeled link for an Azure DevOps build URL by looking up
# the pipeline name and build number via the ADO REST API. Falls back to the
# raw URL on parse or API failure. Requires $SYSTEM_ACCESSTOKEN (every ADO
# job has it when 'Allow scripts to access OAuth token' is enabled).
#
# Supports:
#   https://dev.azure.com/<org>/<project>/_build/results?buildId=<id>...
#   https://<org>.visualstudio.com/<project>/_build/results?buildId=<id>...
notify_ado_link() {
  local url="$1"
  if [[ -z "$url" ]]; then return 0; fi

  if [[ -z "${SYSTEM_ACCESSTOKEN:-}" ]] || ! command -v jq >/dev/null 2>&1; then
    printf '%s' "$url"
    return 0
  fi

  local build_id host org project api_base
  build_id="$(printf '%s' "$url" | sed -n 's/.*[?&]buildId=\([0-9]\+\).*/\1/p')"
  host="$(printf '%s' "$url" | sed -n 's#https\?://\([^/]*\)/.*#\1#p')"
  if [[ -z "$build_id" || -z "$host" ]]; then
    printf '%s' "$url"
    return 0
  fi

  if [[ "$host" == *.visualstudio.com ]]; then
    org="${host%%.visualstudio.com}"
    project="$(printf '%s' "$url" | sed -n 's#https\?://[^/]*/\([^/]*\)/.*#\1#p')"
  elif [[ "$host" == "dev.azure.com" ]]; then
    org="$(printf '%s' "$url" | sed -n 's#https\?://dev\.azure\.com/\([^/]*\)/.*#\1#p')"
    project="$(printf '%s' "$url" | sed -n 's#https\?://dev\.azure\.com/[^/]*/\([^/]*\)/.*#\1#p')"
  else
    printf '%s' "$url"
    return 0
  fi

  if [[ -z "$org" || -z "$project" ]]; then
    printf '%s' "$url"
    return 0
  fi

  api_base="https://dev.azure.com/${org}/${project}"

  local meta
  if ! meta="$(curl -sS -f \
    -H "Authorization: Bearer $SYSTEM_ACCESSTOKEN" \
    "${api_base}/_apis/build/builds/${build_id}?api-version=7.1" 2>/dev/null)"; then
    printf '%s' "$url"
    return 0
  fi

  local name number label
  name="$(printf '%s' "$meta" | jq -r '.definition.name // empty')"
  number="$(printf '%s' "$meta" | jq -r '.buildNumber // empty')"
  if [[ -z "$name" ]]; then
    printf '%s' "$url"
    return 0
  fi

  label="$name"
  [[ -n "$number" ]] && label="${label} #${number}"
  # Escape markdown brackets in the label so it renders verbatim.
  label="${label//\[/\\[}"
  label="${label//\]/\\]}"

  printf '[%s](%s)' "$label" "$url"
}

# notify_status --status <s> --stage <s> --title <t> [--summary <s>] [--run-url <u>] [--severity <s>]
#                [--fact "Name|Value[|URL]"]... [--cc-label <text>] [--cc-user "<upn>[|name]"]... [--mention-channel]
#
# Status enum:   queued | running | succeeded | failed | canceled
# Severity enum: info | success | warning | failure
#
# Each --fact adds one row to the card's FactSet. URL is optional; when given,
# the value renders as a markdown link.
# Each --cc-user adds one Teams mention to the cc line. Display name defaults
# to the email prefix when not provided.
notify_status() {
  notify_require_env || return 0
  if ! command -v jq >/dev/null 2>&1; then
    echo "notify-bot: jq not found, skipping" >&2
    return 0
  fi

  local status="" stage="" title="" summary="" run_url="" severity="" mention_channel="" cc_label=""
  local -a facts=()
  local -a cc_users=()
  while (( $# > 0 )); do
    case "$1" in
      --status)          status="$2";   shift 2 ;;
      --stage)           stage="$2";    shift 2 ;;
      --title)           title="$2";    shift 2 ;;
      --summary)         summary="$2";  shift 2 ;;
      --run-url)         run_url="$2";  shift 2 ;;
      --severity)        severity="$2"; shift 2 ;;
      --fact)            facts+=("$2"); shift 2 ;;
      --cc-label)        cc_label="$2"; shift 2 ;;
      --cc-user)         cc_users+=("$2"); shift 2 ;;
      --mention-channel) mention_channel="true"; shift ;;
      *) echo "notify-bot: unknown arg $1" >&2; return 0 ;;
    esac
  done

  if [[ -z "$title" ]]; then
    echo "notify-bot: --title is required" >&2
    return 0
  fi

  local token
  token="$(notify_mint_token)" || return 0

  # Build facts JSON array. Each entry: "Name|Value" or "Name|Value|URL".
  local facts_json="[]"
  if [[ -n "$run_url" || ${#facts[@]} -gt 0 ]]; then
    facts_json="$({
      if [[ -n "$run_url" ]]; then
        jq -n --arg url "$run_url" '{name:"Run", value:$url, url:$url}'
      fi
      for f in "${facts[@]}"; do
        local fname fval furl
        fname="${f%%|*}"
        local rest="${f#*|}"
        if [[ "$rest" == "$f" ]]; then
          fval=""
          furl=""
        elif [[ "$rest" == *"|"* ]]; then
          fval="${rest%%|*}"
          furl="${rest#*|}"
        else
          fval="$rest"
          furl=""
        fi
        jq -n --arg n "$fname" --arg v "$fval" --arg u "$furl" \
          'if $u != "" then {name:$n, value:$v, url:$u} else {name:$n, value:$v} end'
      done
    } | jq -s .)"
  fi

  # Build cc JSON object when cc users were supplied.
  local cc_json="null"
  if (( ${#cc_users[@]} > 0 )); then
    local mentions_json
    mentions_json="$(printf '%s\n' "${cc_users[@]}" | jq -R . | jq -s .)"
    cc_json="$(jq -n --arg lbl "$cc_label" --argjson m "$mentions_json" \
      'if $lbl != "" then {label: $lbl, mentions: $m} else {mentions: $m} end')"
  fi

  local payload
  payload="$(jq -n \
    --arg     source         "$NOTIFY_SOURCE" \
    --arg     runId          "$NOTIFY_RUN_ID" \
    --arg     teamId         "$NOTIFY_TEAM_ID" \
    --arg     channelId      "$NOTIFY_CHANNEL_ID" \
    --arg     title          "$title" \
    --arg     summary        "$summary" \
    --arg     status         "$status" \
    --arg     stage          "$stage" \
    --arg     severity       "$severity" \
    --argjson factsArr       "$facts_json" \
    --argjson ccObj          "$cc_json" \
    --arg     mentionChannel "$mention_channel" \
    '{
      source: $source,
      runId: $runId,
      teamId: $teamId,
      channelId: $channelId,
      title: $title
    }
    + (if $summary        != ""     then {summary:  $summary}  else {} end)
    + (if $status         != ""     then {status:   $status}   else {} end)
    + (if $stage          != ""     then {stage:    $stage}    else {} end)
    + (if $severity       != ""     then {severity: $severity} else {} end)
    + (if ($factsArr | length) > 0  then {facts: $factsArr}    else {} end)
    + (if $ccObj          != null   then {cc: $ccObj}          else {} end)
    + (if $mentionChannel == "true" then {mentionChannel: true} else {} end)
    ')"

  local http_code
  http_code="$(curl -sS -o /tmp/notify-bot.out -w '%{http_code}' \
    -X POST \
    -H "Authorization: Bearer $token" \
    -H "Content-Type: application/json" \
    --data "$payload" \
    "$NOTIFIER_ENDPOINT/api/notifications" || echo "000")"

  if [[ "$http_code" -lt 200 || "$http_code" -ge 300 ]]; then
    echo "notify-bot: /api/notifications -> HTTP $http_code" >&2
    head -c 500 /tmp/notify-bot.out >&2 || true
    echo "" >&2
    return 0
  fi
  echo "notify-bot: posted status=$status stage=$stage (HTTP $http_code)"
}

# notify_reply --text <message> [--tag <short tag>] [--severity <s>] [--ado-url <ado-build-url>] [--mention-channel] [--mention-user <upn>]...
#
# If --ado-url is given, a markdown-labeled link to the build (pipeline name +
# build number) is appended to --text via notify_ado_link. Callers don't need
# to pre-compute the label.
#
# --mention-channel emits a Teams @channel mention. --mention-user can be
# passed multiple times to attempt user mentions by UPN/email (best-effort).
notify_reply() {
  notify_require_env || return 0
  if ! command -v jq >/dev/null 2>&1; then
    echo "notify-bot: jq not found, skipping" >&2
    return 0
  fi

  local text="" tag="" severity="" ado_url="" mention_channel=""
  local -a mention_upns=()
  while (( $# > 0 )); do
    case "$1" in
      --text)             text="$2";     shift 2 ;;
      --tag)              tag="$2";      shift 2 ;;
      --severity)         severity="$2"; shift 2 ;;
      --ado-url)          ado_url="$2";  shift 2 ;;
      --mention-channel)  mention_channel="true"; shift ;;
      --mention-user)     mention_upns+=("$2"); shift 2 ;;
      *) echo "notify-bot: unknown arg $1" >&2; return 0 ;;
    esac
  done

  if [[ -n "$ado_url" ]]; then
    local link
    link="$(notify_ado_link "$ado_url")"
    if [[ -n "$text" ]]; then
      text="${text} ${link}"
    else
      text="$link"
    fi
  fi

  if [[ -z "$text" ]]; then
    echo "notify-bot: --text or --ado-url is required" >&2
    return 0
  fi

  local token
  token="$(notify_mint_token)" || return 0

  # Build mentionUpns as a JSON array from the bash array.
  local mention_upns_json="[]"
  if (( ${#mention_upns[@]} > 0 )); then
    mention_upns_json="$(printf '%s\n' "${mention_upns[@]}" | jq -R . | jq -s .)"
  fi

  local payload
  payload="$(jq -n \
    --arg source         "$NOTIFY_SOURCE" \
    --arg runId          "$NOTIFY_RUN_ID" \
    --arg text           "$text" \
    --arg tag            "$tag" \
    --arg severity       "$severity" \
    --argjson mentionUpns "$mention_upns_json" \
    --arg mentionChannel "$mention_channel" \
    '{source: $source, runId: $runId, text: $text}
     + (if $tag      != "" then {tag: $tag}           else {} end)
     + (if $severity != "" then {severity: $severity} else {} end)
     + (if $mentionChannel == "true" then {mentionChannel: true} else {} end)
     + (if ($mentionUpns | length) > 0 then {mentionUpns: $mentionUpns} else {} end)')"

  local http_code
  http_code="$(curl -sS -o /tmp/notify-bot.out -w '%{http_code}' \
    -X POST \
    -H "Authorization: Bearer $token" \
    -H "Content-Type: application/json" \
    --data "$payload" \
    "$NOTIFIER_ENDPOINT/api/notifications/reply" || echo "000")"

  if [[ "$http_code" -lt 200 || "$http_code" -ge 300 ]]; then
    echo "notify-bot: /api/notifications/reply -> HTTP $http_code" >&2
    head -c 500 /tmp/notify-bot.out >&2 || true
    echo "" >&2
    return 0
  fi
  echo "notify-bot: replied (HTTP $http_code)"
}
