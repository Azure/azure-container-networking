#!/usr/bin/env bash
#
# Collects the failure-analysis artifacts published across the last N days of
# builds into a single directory, so the failure-agent's --weekly-report mode
# can aggregate them.
#
# Each per-run failure-analysis job publishes an artifact named
# `failureAnalysis_<os>_<cluster>` containing report.md + incident.json. This
# script walks the recent builds of the given pipeline definition(s) via the
# Azure DevOps REST API, downloads every `failureAnalysis_*` artifact, and
# unzips each into <output-dir>/<buildId>_<artifactName>/ so the incident.json
# files are discoverable by a recursive walk.
#
# Usage:
#   collect-week-artifacts.sh <output-dir> <window-days> <definition-ids-csv>
#
# Required environment (every ADO job has these when 'Allow scripts to access
# the OAuth token' is enabled):
#   SYSTEM_COLLECTIONURI   e.g. https://dev.azure.com/<org>/
#   SYSTEM_TEAMPROJECT     e.g. mariner
#   SYSTEM_ACCESSTOKEN     the job's OAuth bearer
#
# Requires curl, jq, and unzip on the agent.
#
# Best-effort: individual build/artifact failures are logged and skipped so a
# single bad artifact never aborts the weekly digest.

set -uo pipefail

OUT_DIR="${1:-}"
WINDOW_DAYS="${2:-7}"
DEF_IDS_CSV="${3:-}"

if [[ -z "$OUT_DIR" ]]; then
  echo "collect-week-artifacts: usage: collect-week-artifacts.sh <output-dir> <window-days> <definition-ids-csv>" >&2
  exit 0
fi

for tool in curl jq unzip; do
  if ! command -v "$tool" >/dev/null 2>&1; then
    echo "collect-week-artifacts: $tool not found, skipping" >&2
    exit 0
  fi
done

if [[ -z "${SYSTEM_COLLECTIONURI:-}" || -z "${SYSTEM_TEAMPROJECT:-}" || -z "${SYSTEM_ACCESSTOKEN:-}" ]]; then
  echo "collect-week-artifacts: missing SYSTEM_COLLECTIONURI / SYSTEM_TEAMPROJECT / SYSTEM_ACCESSTOKEN, skipping" >&2
  exit 0
fi

if [[ -z "$DEF_IDS_CSV" ]]; then
  echo "collect-week-artifacts: no pipeline definition ids configured (arg 3), skipping" >&2
  exit 0
fi

mkdir -p "$OUT_DIR"

# ADO REST wants the collection URI without a trailing slash.
COLLECTION="${SYSTEM_COLLECTIONURI%/}"
API_BASE="${COLLECTION}/${SYSTEM_TEAMPROJECT}/_apis"
AUTH_HEADER="Authorization: Bearer ${SYSTEM_ACCESSTOKEN}"

# minTime as UTC ISO-8601, portable across GNU and BSD date.
if min_time="$(date -u -d "-${WINDOW_DAYS} days" +%Y-%m-%dT%H:%M:%SZ 2>/dev/null)"; then
  :
elif min_time="$(date -u -v-"${WINDOW_DAYS}"d +%Y-%m-%dT%H:%M:%SZ 2>/dev/null)"; then
  :
else
  echo "collect-week-artifacts: could not compute min time, skipping" >&2
  exit 0
fi
echo "collect-week-artifacts: collecting failureAnalysis_* artifacts since $min_time (defs: $DEF_IDS_CSV)"

downloaded=0

IFS=',' read -r -a DEF_IDS <<<"$DEF_IDS_CSV"
for def_id in "${DEF_IDS[@]}"; do
  def_id="$(printf '%s' "$def_id" | tr -d '[:space:]')"
  [[ -n "$def_id" ]] || continue

  # Enumerate every build in the window, following ADO's continuation-token
  # header across pages. A single unpaginated request returns only the first
  # page, silently dropping the overflow on a busy week and undercounting the
  # trends — so page until the token is gone. $top widens each page to cut
  # round-trips; queryOrder makes truncation (if any) deterministic.
  build_ids=()
  cont_token=""
  builds_url="${API_BASE}/build/builds?definitions=${def_id}&minTime=${min_time}&statusFilter=completed&queryOrder=finishTimeDescending&\$top=1000&api-version=7.1"
  while :; do
    url="$builds_url"
    [[ -n "$cont_token" ]] && url="${url}&continuationToken=${cont_token}"

    hdr_file="$(mktemp)"
    if ! builds_json="$(curl -sS -D "$hdr_file" -H "$AUTH_HEADER" "$url" 2>/dev/null)"; then
      echo "collect-week-artifacts: failed to list builds for definition $def_id" >&2
      rm -f "$hdr_file"
      break
    fi

    mapfile -t -O "${#build_ids[@]}" build_ids < <(printf '%s' "$builds_json" | jq -r '.value[]?.id // empty')

    cont_token="$(grep -i '^x-ms-continuationtoken:' "$hdr_file" | tail -n1 | sed 's/^[^:]*:[[:space:]]*//; s/[[:space:]]*$//' | tr -d '\r')"
    rm -f "$hdr_file"
    [[ -n "$cont_token" ]] || break
  done
  echo "collect-week-artifacts: definition $def_id -> ${#build_ids[@]} build(s) in window"

  for build_id in "${build_ids[@]}"; do
    artifacts_json="$(curl -sS -H "$AUTH_HEADER" \
      "${API_BASE}/build/builds/${build_id}/artifacts?api-version=7.1" 2>/dev/null)" || {
      echo "collect-week-artifacts: failed to list artifacts for build $build_id" >&2
      continue
    }

    # Emit "name<TAB>downloadUrl" for each failureAnalysis_* artifact.
    while IFS=$'\t' read -r art_name download_url; do
      [[ -n "$art_name" && -n "$download_url" ]] || continue

      dest="${OUT_DIR}/${build_id}_${art_name}"
      mkdir -p "$dest"
      zip_path="${dest}.zip"

      # The downloadUrl already targets the artifact; request the zip format.
      sep='?'
      [[ "$download_url" == *"?"* ]] && sep='&'
      if ! curl -sS -L -H "$AUTH_HEADER" -o "$zip_path" "${download_url}${sep}\$format=zip" 2>/dev/null; then
        echo "collect-week-artifacts: failed to download $art_name from build $build_id" >&2
        rm -f "$zip_path"
        continue
      fi
      if unzip -o -q "$zip_path" -d "$dest" 2>/dev/null; then
        downloaded=$((downloaded + 1))
      else
        echo "collect-week-artifacts: failed to unzip $art_name from build $build_id" >&2
      fi
      rm -f "$zip_path"
    done < <(printf '%s' "$artifacts_json" \
      | jq -r '.value[]? | select((.name // "") | startswith("failureAnalysis_")) | [.name, (.resource.downloadUrl // "")] | @tsv')
  done
done

echo "collect-week-artifacts: downloaded $downloaded failureAnalysis_* artifact(s) into $OUT_DIR"
