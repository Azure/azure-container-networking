#!/usr/bin/env bash
#
# Bridges the failure-agent's weekly trends digest (weekly-incident.json) to the
# shared ACN Pipeline Notifier bot.
#
# Unlike the high-volume per-run incident card (notify-incident.sh, which posts
# to the channel with no @mentions), the weekly digest is low-frequency, so it
# DOES @mention the failure-analysis owners — this is the one card that pings
# them directly, carrying the week's trends they should review.
#
# Usage (from an AzureCLI@2 step with addSpnToEnvironment: true):
#   .pipelines/failure-agent-teams-bot/scripts/notify-weekly.sh <weekly-incident.json>
#
# Requires the same env as notify-bot.sh (NOTIFIER_*, NOTIFY_*), which must be
# exported by the calling task.
#
# Best-effort: a missing file or missing jq is a quiet no-op and never fails the
# build.

set -uo pipefail

WEEKLY="${1:-}"

if [[ -z "$WEEKLY" ]]; then
  echo "notify-weekly: usage: notify-weekly.sh <weekly-incident.json>" >&2
  exit 0
fi

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
# shellcheck source=notify-bot.sh
source "$SCRIPT_DIR/notify-bot.sh"
# shellcheck source=render-verdict.sh
source "$SCRIPT_DIR/render-verdict.sh"

if ! command -v jq >/dev/null 2>&1; then
  echo "notify-weekly: jq not found, skipping" >&2
  exit 0
fi

if [[ ! -f "$WEEKLY" ]]; then
  echo "notify-weekly: no weekly file at $WEEKLY, skipping" >&2
  exit 0
fi

# --- Read the digest fields -------------------------------------------------
total="$(jq -r '.totalIncidents // 0' "$WEEKLY")"
window="$(jq -r '.windowDays // 7' "$WEEKLY")"
headline="$(jq -r '.headline // ""' "$WEEKLY")"
narrative="$(jq -r '.narrative // ""' "$WEEKLY")"
window_start="$(jq -r '.windowStart // "" | if type == "string" then (.[0:10]) else "" end' "$WEEKLY")"

# Top category and top recurring signature as at-a-glance facts.
top_category="$(jq -r '(.stats.categoryCounts // [])[0] // {} | if (.label // "") != "" then (.label + " (" + ((.count // 0)|tostring) + ")") else "" end' "$WEEKLY")"
top_recurring="$(jq -r '(.stats.topRecurring // [])[0] // {} | if (.fingerprint // "") != "" then ((.fingerprint[0:12]) + " ×" + ((.count // 0)|tostring)) else "" end' "$WEEKLY")"

# --- Root card: notify_status ----------------------------------------------
title="Failure Analysis — Weekly Trends"

summary_text="$headline"
[[ -z "$summary_text" ]] && summary_text="${total} failure incidents analyzed in the last ${window} days"

status_args=(
  --status succeeded
  --stage weekly
  --severity info
  --title "$title"
  --summary "$summary_text"
)

window_fact="last ${window} days"
[[ -n "$window_start" ]] && window_fact="${window_fact} (since ${window_start})"
status_args+=(--fact "Window|${window_fact}")
status_args+=(--fact "Incidents|${total}")
[[ -n "$top_category" ]]  && status_args+=(--fact "Top category|${top_category}")
[[ -n "$top_recurring" ]] && status_args+=(--fact "Top recurring|${top_recurring}")

# @mention the failure-analysis owners: the weekly digest is the low-frequency
# card meant for their review, so this is where owner mentions live.
status_args+=(--cc-label "For review")
status_args+=(--cc-user "johnpayne@microsoft.com|John Payne")
status_args+=(--cc-user "behzadm@microsoft.com|Behzad Mirkhanzadeh")

notify_status "${status_args[@]}"

# --- Threaded detail: notify_reply -----------------------------------------
# The narrative is the self-contained trends writeup; append the key trends and
# recommendations bullet lists when the model provided them.
reply="$narrative"

key_trends_md="$(jq -r '(.keyTrends // []) | map("- " + .) | join("\n")' "$WEEKLY")"
if [[ -n "$key_trends_md" ]]; then
  reply="$(printf '%s\n\n**Key trends**\n%s' "$reply" "$key_trends_md")"
fi

recs_md="$(jq -r '(.recommendations // []) | map("- " + .) | join("\n")' "$WEEKLY")"
if [[ -n "$recs_md" ]]; then
  reply="$(printf '%s\n\n**Recommendations**\n%s' "$reply" "$recs_md")"
fi

if [[ -z "$(printf '%s' "$reply" | tr -d '[:space:]')" ]]; then
  reply="No incidents were recorded in the last ${window} days."
fi

# Downgrade the trends narrative to the Markdown subset a Teams Adaptive Card
# TextBlock renders (see render-verdict.sh): the model can emit tables/code
# fences that otherwise collapse in the threaded reply.
reply="$(printf '%s' "$reply" | _sanitize_teams_md)"

notify_reply --text "$reply" --tag "weekly-trends" --severity info
