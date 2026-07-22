#!/usr/bin/env bash
#
# Bridges a failure-agent incident.json to the shared ACN Pipeline Notifier bot.
#
# When the failure-analysis pipeline produces a confident, actionable diagnosis
# (analyzed + confidence >= threshold + a proposed fix), this posts one
# engineer-facing Adaptive Card (via notify_status) summarizing the diagnosis,
# plus a threaded reply (via notify_reply) carrying the proposed fix, top
# evidence, and recommended action — everything an on-call engineer needs to
# start acting without opening the run.
#
# Usage (from an AzureCLI@2 step with addSpnToEnvironment: true):
#   .pipelines/failure-agent-teams-bot/scripts/notify-incident.sh <incident.json> [min-confidence]
#
# Requires the same env as notify-bot.sh (NOTIFIER_*, NOTIFY_*), which must be
# exported by the calling task. min-confidence defaults to 0.51.
#
# Best-effort: a missing file, missing jq, or a below-threshold incident is a
# quiet no-op and never fails the build.

set -uo pipefail

INCIDENT="${1:-}"
MIN_CONFIDENCE="${2:-0.51}"

if [[ -z "$INCIDENT" ]]; then
  echo "notify-incident: usage: notify-incident.sh <incident.json> [min-confidence]" >&2
  exit 0
fi

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
# shellcheck source=notify-bot.sh
source "$SCRIPT_DIR/notify-bot.sh"

if ! command -v jq >/dev/null 2>&1; then
  echo "notify-incident: jq not found, skipping" >&2
  exit 0
fi

if [[ ! -f "$INCIDENT" ]]; then
  echo "notify-incident: no incident file at $INCIDENT, skipping" >&2
  exit 0
fi

# --- Gate: only ping on a confident, actionable analysis --------------------
analysis_status="$(jq -r '.analysisStatus // ""' "$INCIDENT")"
confidence="$(jq -r '.confidence // 0' "$INCIDENT")"
proposed_fix="$(jq -r '.proposedFix // ""' "$INCIDENT")"

meets="$(jq -n --argjson c "$confidence" --argjson m "$MIN_CONFIDENCE" '$c >= $m' 2>/dev/null || echo false)"
if [[ "$analysis_status" != "analyzed" || "$meets" != "true" || -z "$proposed_fix" ]]; then
  echo "notify-incident: skipping — status=$analysis_status confidence=$confidence fix=$([[ -n "$proposed_fix" ]] && echo yes || echo no) (threshold $MIN_CONFIDENCE)"
  exit 0
fi

# --- Read the fields an engineer needs --------------------------------------
pipeline="$(jq -r '.pipelineName // ""' "$INCIDENT")"
build_number="$(jq -r '.buildNumber // ""' "$INCIDENT")"
category="$(jq -r '.category // ""' "$INCIDENT")"
band="$(jq -r '.confidenceBand // ""' "$INCIDENT")"
root_cause="$(jq -r '.rootCauseSummary // ""' "$INCIDENT")"
owner="$(jq -r '.recommendedOwner // ""' "$INCIDENT")"
fingerprint="$(jq -r '.fingerprint // ""' "$INCIDENT")"
commit="$(jq -r '.commit // ""' "$INCIDENT")"
stage="$(jq -r '.stage // ""' "$INCIDENT")"
job="$(jq -r '.job // ""' "$INCIDENT")"
repository="$(jq -r '.repository // ""' "$INCIDENT")"
pr_number="$(jq -r '.pullRequestNumber // ""' "$INCIDENT")"
recommended_action="$(jq -r '.recommendedAction // ""' "$INCIDENT")"
retention="$(jq -r '.retentionDecision // ""' "$INCIDENT")"

# Compact confidence, e.g. "high (0.87)".
conf_pretty="$(jq -rn --argjson c "$confidence" '($c * 100 | floor) / 100 | tostring')"

# Scenario line from whichever cluster fields are populated.
scenario="$(jq -r '
  [ .clusterType, .cni, .os, .region ]
  | map(select(. != null and . != "")) | join(" · ")' "$INCIDENT")"

# --- Root card: notify_status ----------------------------------------------
severity="failure"
[[ "$category" == "known_flake" ]] && severity="warning"

title="Failure Analysis — ${pipeline}"
[[ -n "$build_number" ]] && title="${title} #${build_number}"

run_url=""
if [[ -n "${SYSTEM_COLLECTIONURI:-}" && -n "${SYSTEM_TEAMPROJECT:-}" && -n "${BUILD_BUILDID:-}" ]]; then
  run_url="${SYSTEM_COLLECTIONURI}${SYSTEM_TEAMPROJECT}/_build/results?buildId=${BUILD_BUILDID}"
fi

status_args=(
  --status failed
  --stage analysis
  --severity "$severity"
  --title "$title"
  --summary "$root_cause"
)
[[ -n "$run_url" ]] && status_args+=(--run-url "$run_url")

status_args+=(--fact "Confidence|${band} (${conf_pretty})")
[[ -n "$category" ]] && status_args+=(--fact "Category|${category}")

stage_job="$(printf '%s / %s' "$stage" "$job" | sed 's#^ / ##; s# / $##')"
[[ -n "$stage_job" ]] && status_args+=(--fact "Stage / Job|${stage_job}")
[[ -n "$scenario" ]]  && status_args+=(--fact "Scenario|${scenario}")
[[ -n "$owner" ]]     && status_args+=(--fact "Owner|${owner}")
[[ -n "$fingerprint" ]] && status_args+=(--fact "Fingerprint|${fingerprint:0:12}")
[[ -n "$commit" ]]    && status_args+=(--fact "Commit|${commit:0:12}")
if [[ -n "$pr_number" && -n "$repository" ]]; then
  status_args+=(--fact "Pull request|#${pr_number}|https://github.com/${repository}/pull/${pr_number}")
fi

# @mention whoever queued this run so the ping lands on them in the shared
# channel. Build.RequestedForEmail is the AAD UPN the notifier resolves; empty
# (some scheduled/service triggers) is a quiet skip. Build.RequestedFor is the
# display name; notify_status defaults to the email prefix when it's absent.
initiator_upn="${BUILD_REQUESTEDFOREMAIL:-}"
initiator_name="${BUILD_REQUESTEDFOR:-}"
if [[ -n "$initiator_upn" ]]; then
  status_args+=(--cc-label "Initiated by")
  if [[ -n "$initiator_name" ]]; then
    status_args+=(--cc-user "${initiator_upn}|${initiator_name}")
  else
    status_args+=(--cc-user "$initiator_upn")
  fi
fi

notify_status "${status_args[@]}"

# --- Threaded detail: notify_reply -----------------------------------------
# Bullet the top evidence lines.
evidence_md="$(jq -r '(.topEvidence // []) | map("- " + .) | join("\n")' "$INCIDENT")"

reply="$(printf '**Proposed fix**\n%s' "$proposed_fix")"
if [[ -n "$evidence_md" ]]; then
  reply="$(printf '%s\n\n**Top evidence**\n%s' "$reply" "$evidence_md")"
fi
if [[ -n "$recommended_action" ]]; then
  reply="$(printf '%s\n\n**Recommended action**\n%s' "$reply" "$recommended_action")"
fi
if [[ -n "$retention" ]]; then
  reply="$(printf '%s\n\n_Cluster retention: %s_' "$reply" "$retention")"
fi

notify_reply --text "$reply" --tag "diagnosis" --severity "$severity"
