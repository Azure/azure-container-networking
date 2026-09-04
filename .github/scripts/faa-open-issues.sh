#!/usr/bin/env bash
# Opens one GitHub issue per issue.md pulled from Azure DevOps, and best-effort
# pings the shared Teams notifier bot.
#
# Issues are created unassigned. A separate agentic workflow picks them up and
# authors the draft pull request; that is deliberately not this script's job.
#
# Security: issue.md is produced by a model running in Azure DevOps, so it is
# treated as untrusted input. The fingerprint is validated before it reaches a
# search or a label, labels are intersected with a fixed allowlist, and the body
# is only ever written to the issue body — never echoed to logs or the job
# summary.
#
# Inputs (env): GH_TOKEN, REPO, WORKDIR and (optional Teams) NOTIFIER_ENDPOINT,
# NOTIFIER_API_AUDIENCE, NOTIFY_TEAM_ID, NOTIFY_CHANNEL_ID, NOTIFY_CC, BUILD_ID.
set -euo pipefail

: "${GH_TOKEN:?GH_TOKEN required}"
: "${REPO:?REPO required}"
: "${WORKDIR:?WORKDIR required}"

# base_label marks every issue this workflow creates. It is also how the
# idempotency scan narrows the issue list, so it is always applied.
base_label="faa-generated"

# allowed_labels mirrors the enum in tools/failure-agent/internal/escalate. The
# model already picks from that enum, but issue.md crosses a trust boundary
# between Azure DevOps and GitHub, so the set is re-checked here.
allowed_labels=(bug regression cni cns npm ipam windows linux test-infra)

# meta <file> <key> — read a value from the issue.md metadata header.
meta() { sed -n '/<!-- faa-issue:v1/,/-->/p' "$1" | sed -n "s/^$2: //p" | head -n1; }

is_allowed_label() {
  local candidate="$1" l
  for l in "${allowed_labels[@]}"; do
    [ "$l" = "$candidate" ] && return 0
  done
  return 1
}

# ensure_label <name> — create the label if the repo does not already have it.
# `gh issue create` fails outright on an unknown label, so this must run first.
# Existing labels are left alone rather than force-updated, so this never
# clobbers a colour or description a human chose.
known_labels=""
ensure_label() {
  local l="$1"
  if [ -z "$known_labels" ]; then
    known_labels="$(gh label list --repo "$REPO" --limit 500 --json name --jq '.[].name' 2>/dev/null || true)"
  fi
  if grep -qxF "$l" <<<"$known_labels"; then
    return 0
  fi
  if gh label create "$l" --repo "$REPO" --color ededed \
      --description "Raised by the ACN Failure Analysis Agent" >/dev/null 2>&1; then
    echo "created label $l"
  fi
  known_labels="$known_labels"$'\n'"$l"
}

# notify_teams <issue-url> <title> <owner> — best-effort Teams ping via the
# shared notifier bot, using the Azure OIDC login the workflow already
# established. Never fails the run.
notify_teams() {
  local issue_url="$1" title="$2" owner="$3"
  if [ -z "${NOTIFIER_ENDPOINT:-}" ] || [ -z "${NOTIFIER_API_AUDIENCE:-}" ] || [ -z "${NOTIFY_TEAM_ID:-}" ] || [ -z "${NOTIFY_CHANNEL_ID:-}" ]; then
    echo "notifier vars incomplete; skipping Teams notify"; return 0
  fi
  local token
  token="$(az account get-access-token --resource "$NOTIFIER_API_AUDIENCE" --query accessToken -o tsv 2>/dev/null || true)"
  [ -n "$token" ] || { echo "could not mint notifier token; skipping"; return 0; }

  # cc mentions: configured UPNs plus the recommended owner when it is an email.
  local mentions='[]' csv="${NOTIFY_CC:-}"
  case "$owner" in *@*) csv="${csv:+$csv,}$owner" ;; esac
  if [ -n "$csv" ]; then
    mentions="$(echo "$csv" | tr ',' '\n' | sed 's/^ *//;s/ *$//' | grep -v '^$' | jq -R . | jq -s .)"
  fi
  local cc='null'
  if [ "$(jq 'length' <<<"$mentions")" -gt 0 ]; then
    cc="$(jq -n --argjson m "$mentions" '{label:"Owner", mentions:$m}')"
  fi

  local payload code
  payload="$(jq -n \
    --arg source "faa-github-issue" --arg runId "${BUILD_ID:-}" \
    --arg teamId "$NOTIFY_TEAM_ID" --arg channelId "$NOTIFY_CHANNEL_ID" \
    --arg title "$title" --arg issue "$issue_url" --argjson cc "$cc" \
    '{source:$source, runId:$runId, teamId:$teamId, channelId:$channelId,
      title:$title, summary:("Failure analysis raised an issue for a code fix: " + $issue),
      status:"running", severity:"warning",
      facts:[{name:"Issue", value:$issue, url:$issue}], mentionChannel:true}
     + (if $cc != null then {cc:$cc} else {} end)')"
  code="$(curl -sS -o /dev/null -w '%{http_code}' -X POST \
    -H "Authorization: Bearer ${token}" -H "Content-Type: application/json" \
    --data "$payload" "${NOTIFIER_ENDPOINT}/api/notifications" || echo 000)"
  echo "notifier POST -> HTTP $code"
}

list="$WORKDIR/issuelist.txt"
if [ ! -s "$list" ]; then
  echo "::notice::no issue.md files to process"
  exit 0
fi

ensure_label "$base_label"

# Idempotency: scan existing FAA issue bodies for the fingerprint marker rather
# than relying on GitHub's search index, which does not reliably match text
# inside HTML comments. The FAA issue population is small, so one listing is
# cheaper and more deterministic than a per-fingerprint search.
existing="$WORKDIR/existing-issues.json"
gh issue list --repo "$REPO" --state all --label "$base_label" \
  --limit 500 --json number,body > "$existing" 2>/dev/null || echo '[]' > "$existing"

created=0
declare -A seen
while IFS= read -r file; do
  [ -n "$file" ] || continue

  fp="$(meta "$file" fingerprint)"
  # Untrusted artifact: fp flows into a jq match and a log line, so require a
  # lowercase-hex digest.
  case "$fp" in
    ''|*[!0-9a-f]*) echo "skipping $file: invalid fingerprint '$fp'"; continue ;;
  esac
  if [ -n "${seen[$fp]:-}" ]; then
    echo "skipping duplicate fingerprint $fp in this build"; continue
  fi
  seen[$fp]=1

  marker="<!-- acn-faa-fingerprint:${fp} -->"
  dupe="$(jq -r --arg m "$marker" 'map(select(.body != null and (.body | contains($m)))) | .[0].number // ""' "$existing")"
  if [ -n "$dupe" ]; then
    echo "issue #$dupe already covers fingerprint ${fp:0:12}; skipping"
    continue
  fi

  title="$(meta "$file" title)"
  [ -n "$title" ] || title="[FAA] Pipeline failure needs a code fix (${fp:0:12})"
  owner="$(meta "$file" owner)"

  label_args=(--label "$base_label")
  IFS=',' read -r -a raw_labels <<< "$(meta "$file" labels)"
  for l in "${raw_labels[@]}"; do
    l="$(echo "$l" | tr -d '[:space:]' | tr '[:upper:]' '[:lower:]')"
    [ -n "$l" ] || continue
    if ! is_allowed_label "$l"; then
      echo "dropping label '$l': not in the allowlist"
      continue
    fi
    ensure_label "$l"
    label_args+=(--label "$l")
  done

  issue_url="$(gh issue create --repo "$REPO" --title "$title" --body-file "$file" "${label_args[@]}")"
  echo "opened issue: $issue_url"
  created=$((created + 1))

  notify_teams "$issue_url" "$title" "$owner"
done < "$list"

echo "created $created issue(s)"
if [ "$created" = "0" ]; then
  echo "::notice::no new issues (every fingerprint already had one)"
fi
