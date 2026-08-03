#!/usr/bin/env bash
#
# Fans in every failure-agent incident.json produced by a pipeline run (one per
# E2E stage, downloaded from the failureAnalysis_* artifacts), groups them by a
# stage-independent failure signature, posts ONE consolidated Adaptive Card to
# the shared run thread, and explicitly calls out any signature that failed
# across multiple E2E stages this run.
#
# Usage (from an AzureCLI@2 step with addSpnToEnvironment: true):
#   .pipelines/failure-agent-teams-bot/scripts/notify-run-summary.sh <incident-dir> [min-confidence]
#
# <incident-dir> is a directory tree containing the downloaded failureAnalysis_*
# artifacts; each artifact folder holds one incident.json. min-confidence
# defaults to 0.65.
#
# Requires the same env as notify-bot.sh (NOTIFIER_*, NOTIFY_*). Crucially
# NOTIFY_RUN_ID must match the per-stage pings (Build.BuildId) so this card
# mutates the same root card — running last, it is the authoritative final
# state for the run.
#
# Best-effort: a missing dir, missing jq, or no confident incidents is a quiet
# no-op and never fails the build.

set -uo pipefail

INCIDENT_DIR="${1:-}"
MIN_CONFIDENCE="${2:-0.65}"

if [[ -z "$INCIDENT_DIR" ]]; then
  echo "notify-run-summary: usage: notify-run-summary.sh <incident-dir> [min-confidence]" >&2
  exit 0
fi

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
# shellcheck source=notify-bot.sh
source "$SCRIPT_DIR/notify-bot.sh"
# shellcheck source=render-verdict.sh
source "$SCRIPT_DIR/render-verdict.sh"

if ! command -v jq >/dev/null 2>&1; then
  echo "notify-run-summary: jq not found, skipping" >&2
  exit 0
fi

if [[ ! -d "$INCIDENT_DIR" ]]; then
  echo "notify-run-summary: no incident dir at $INCIDENT_DIR, skipping" >&2
  exit 0
fi

# Each downloaded artifact folder holds one incident.json.
mapfile -t files < <(find "$INCIDENT_DIR" -type f -name 'incident.json' 2>/dev/null | sort)
if (( ${#files[@]} == 0 )); then
  echo "notify-run-summary: no incident.json under $INCIDENT_DIR, nothing to summarize"
  exit 0
fi

# Build an NDJSON stream of confident, analyzed incidents, tagging each with a
# stage-independent signature key and a stage label.
#
# The fingerprint hash embeds StageName/ClusterType/OS/CNI by design (so per
# stage recurrence and idempotent PR comments work), which means the same root
# cause on two stages hashes differently. To detect a signature that spans
# stages we group on the failing unit — which the agent is instructed to keep
# independent of the surfacing stage — falling back to the root-cause summary.
tmp="$(mktemp)"
trap 'rm -f "$tmp"' EXIT
for f in "${files[@]}"; do
  jq -c --arg f "$f" --argjson m "$MIN_CONFIDENCE" '
    def norm: (. // "") | ascii_downcase | gsub("[0-9]+"; "#") | gsub("\\s+"; " ") | trim;
    select((.analysisStatus == "analyzed")
           and ((.confidence // 0) >= $m)
           and (((.finalVerdict // "") != "") or ((.proposedFix // "") != "")))
    | {
        _file: $f,
        _key: ( (.failingUnit // "" | norm) as $fu
                | if $fu != "" then $fu
                  else (.rootCauseSummary // .finalVerdict // .fingerprint // "" | norm) end ),
        _stage: ( (.stage // "") as $s | if $s != "" then $s else (.clusterName // "unknown") end ),
        os: (.os // ""),
        cni: (.cni // ""),
        category: (.category // ""),
        confidenceBand: (.confidenceBand // ""),
        confidence: (.confidence // 0),
        failingUnit: (.failingUnit // ""),
        rootCauseSummary: (.rootCauseSummary // .finalVerdict // "")
      }
  ' "$f" 2>/dev/null >> "$tmp" || true
done

if [[ ! -s "$tmp" ]]; then
  echo "notify-run-summary: no confident analyzed incidents (threshold $MIN_CONFIDENCE), nothing to summarize"
  exit 0
fi

# Group into signatures, each with the distinct stages it hit and a
# highest-confidence representative incident for the verdict render.
groups="$(jq -s '
  group_by(._key)
  | map({
      failingUnit: (.[0].failingUnit),
      rootCause:   (.[0].rootCauseSummary),
      category:    (.[0].category),
      repFile:     (sort_by(.confidence) | last | ._file),
      stages:      ([ .[] | ._stage ] | unique),
      scenarios:   ([ .[]
                      | ._stage
                        + (if (.os // "") != ""
                           then " (" + .os + (if (.cni // "") != "" then "/" + .cni else "" end) + ")"
                           else "" end) ] | unique)
    })
  | sort_by(-(.stages | length))
' "$tmp")"

group_count="$(jq 'length' <<<"$groups")"
stage_count="$(jq '[ .[].stages[] ] | unique | length' <<<"$groups")"
multi_count="$(jq '[ .[] | select((.stages | length) >= 2) ] | length' <<<"$groups")"

# --- Run context for the card ----------------------------------------------
first="${files[0]}"
pipeline="$(jq -r '.pipelineName // ""' "$first")"
build_number="$(jq -r '.buildNumber // ""' "$first")"

run_url=""
if [[ -n "${SYSTEM_COLLECTIONURI:-}" && -n "${SYSTEM_TEAMPROJECT:-}" && -n "${BUILD_BUILDID:-}" ]]; then
  run_url="${SYSTEM_COLLECTIONURI}${SYSTEM_TEAMPROJECT}/_build/results?buildId=${BUILD_BUILDID}"
fi

# --- Consolidated root card: notify_status ---------------------------------
severity="failure"

title="Failure Analysis — ${pipeline}"
[[ -n "$build_number" ]] && title="${title} #${build_number}"
title="${title} — run summary"

summary="${stage_count} E2E stage(s) produced a confident failure analysis this run across ${group_count} distinct signature(s)."
if [[ "$multi_count" -gt 0 ]]; then
  summary="${summary} ⚠️ ${multi_count} signature(s) failing on multiple stages."
fi

status_args=(
  --status failed
  --stage summary
  --severity "$severity"
  --title "$title"
  --summary "$summary"
)
[[ -n "$run_url" ]] && status_args+=(--run-url "$run_url")
status_args+=(--fact "Stages with failures|${stage_count}")
status_args+=(--fact "Distinct signatures|${group_count}")
[[ "$multi_count" -gt 0 ]] && status_args+=(--fact "Multi-stage signatures|${multi_count}")

# Keep the same cc as the per-incident card so the final card still pings the
# initiator and the failure-analysis owners.
status_args+=(--cc-label "Initiated by")
initiator_upn="${BUILD_REQUESTEDFOREMAIL:-}"
initiator_name="${BUILD_REQUESTEDFOR:-}"
if [[ -n "$initiator_upn" ]]; then
  if [[ -n "$initiator_name" ]]; then
    status_args+=(--cc-user "${initiator_upn}|${initiator_name}")
  else
    status_args+=(--cc-user "$initiator_upn")
  fi
fi
status_args+=(--cc-user "johnpayne@microsoft.com|John Payne")
status_args+=(--cc-user "behzadm@microsoft.com|Behzad Mirkhanzadeh")

notify_status "${status_args[@]}"

# --- Consolidated threaded detail: notify_reply ----------------------------
reply="**Run summary — ${stage_count} stage(s), ${group_count} signature(s)**"

# Multi-stage callouts first — the headline of this notifier. Each carries the
# full verdict so the reader gets the answer without opening a stage.
while IFS= read -r g; do
  [[ -n "$g" ]] || continue
  n="$(jq -r '.stages | length' <<<"$g")"
  rc="$(jq -r '.rootCause // .failingUnit // "(unlabeled failure)"' <<<"$g")"
  scen="$(jq -r '.scenarios | join(", ")' <<<"$g")"
  rep="$(jq -r '.repFile' <<<"$g")"
  block="$(printf '⚠️ **Same issue failing on %s E2E stages this run:** %s\n**Stages:** %s' "$n" "$rc" "$scen")"
  verdict="$(render_verdict "$rep")"
  [[ -n "$verdict" ]] && block="$(printf '%s\n\n%s' "$block" "$verdict")"
  reply="$(printf '%s\n\n---\n\n%s' "$reply" "$block")"
done < <(jq -c '.[] | select((.stages | length) >= 2)' <<<"$groups")

# Single-stage signatures: a compact list (each already got its own per-stage
# reply carrying the full verdict, so we don't repeat it here).
single="$(jq -r '
  [ .[] | select((.stages | length) < 2) ]
  | if length == 0 then empty
    else "**Single-stage signatures**\n"
         + ( map("- " + (.rootCause // .failingUnit // "(unlabeled failure)")
                 + " — " + (.scenarios | join(", "))) | join("\n") )
    end
' <<<"$groups")"
[[ -n "$single" ]] && reply="$(printf '%s\n\n---\n\n%s' "$reply" "$single")"

notify_reply --text "$reply" --tag "run-summary" --severity "$severity"
