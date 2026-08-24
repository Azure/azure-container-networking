#!/usr/bin/env bash
#
# Collects only the report.md from every failureAnalysis_* artifact whose build
# finished within a fixed [minTime, maxTime] window, one file per build+artifact,
# so the caller can zip them into a single downloadable bundle.
#
# Unlike collect-week-artifacts.sh (rolling N-day window, keeps report.md +
# incident.json for the weekly digest), this variant takes an explicit date
# range and keeps report.md only.
#
# Usage:
#   collect-reports-range.sh <output-dir> <minTime-iso> <maxTime-iso> <definition-ids-csv>
#
# Required environment (present in every ADO job when 'Allow scripts to access
# the OAuth token' is enabled):
#   SYSTEM_COLLECTIONURI   e.g. https://dev.azure.com/<org>/
#   SYSTEM_TEAMPROJECT     e.g. mariner
#   SYSTEM_ACCESSTOKEN     the job's OAuth bearer
#
# Requires curl, jq, and unzip on the agent.
#
# Best-effort: individual build/artifact failures are logged and skipped so one
# bad artifact never aborts the collection.

set -uo pipefail

OUT_DIR="${1:-}"
MIN_TIME="${2:-}"
MAX_TIME="${3:-}"
DEF_IDS_CSV="${4:-}"

if [[ -z "$OUT_DIR" || -z "$MIN_TIME" || -z "$MAX_TIME" || -z "$DEF_IDS_CSV" ]]; then
  echo "collect-reports-range: usage: collect-reports-range.sh <output-dir> <minTime-iso> <maxTime-iso> <definition-ids-csv>" >&2
  exit 0
fi

for tool in curl jq unzip; do
  if ! command -v "$tool" >/dev/null 2>&1; then
    echo "collect-reports-range: $tool not found, skipping" >&2
    exit 0
  fi
done

if [[ -z "${SYSTEM_COLLECTIONURI:-}" || -z "${SYSTEM_TEAMPROJECT:-}" || -z "${SYSTEM_ACCESSTOKEN:-}" ]]; then
  echo "collect-reports-range: missing SYSTEM_COLLECTIONURI / SYSTEM_TEAMPROJECT / SYSTEM_ACCESSTOKEN, skipping" >&2
  exit 0
fi

mkdir -p "$OUT_DIR"

# ADO REST wants the collection URI without a trailing slash.
COLLECTION="${SYSTEM_COLLECTIONURI%/}"
API_BASE="${COLLECTION}/${SYSTEM_TEAMPROJECT}/_apis"
AUTH_HEADER="Authorization: Bearer ${SYSTEM_ACCESSTOKEN}"

echo "collect-reports-range: collecting report.md from failureAnalysis_* artifacts finished in [$MIN_TIME, $MAX_TIME] (defs: $DEF_IDS_CSV)"

collected=0

IFS=',' read -r -a DEF_IDS <<<"$DEF_IDS_CSV"
for def_id in "${DEF_IDS[@]}"; do
  def_id="$(printf '%s' "$def_id" | tr -d '[:space:]')"
  [[ -n "$def_id" ]] || continue

  # Enumerate every build in the window, following ADO's continuation-token
  # header across pages so a busy range isn't silently truncated to page one.
  build_ids=()
  cont_token=""
  builds_url="${API_BASE}/build/builds?definitions=${def_id}&minTime=${MIN_TIME}&maxTime=${MAX_TIME}&statusFilter=completed&queryOrder=finishTimeDescending&\$top=1000&api-version=7.1"
  while :; do
    url="$builds_url"
    [[ -n "$cont_token" ]] && url="${url}&continuationToken=${cont_token}"

    hdr_file="$(mktemp)"
    if ! builds_json="$(curl -sS -D "$hdr_file" -H "$AUTH_HEADER" "$url" 2>/dev/null)"; then
      echo "collect-reports-range: failed to list builds for definition $def_id" >&2
      rm -f "$hdr_file"
      break
    fi

    mapfile -t -O "${#build_ids[@]}" build_ids < <(printf '%s' "$builds_json" | jq -r '.value[]?.id // empty')

    cont_token="$(grep -i '^x-ms-continuationtoken:' "$hdr_file" | tail -n1 | sed 's/^[^:]*:[[:space:]]*//; s/[[:space:]]*$//' | tr -d '\r')"
    rm -f "$hdr_file"
    [[ -n "$cont_token" ]] || break
  done
  echo "collect-reports-range: definition $def_id -> ${#build_ids[@]} build(s) in window"

  for build_id in "${build_ids[@]}"; do
    artifacts_json="$(curl -sS -H "$AUTH_HEADER" \
      "${API_BASE}/build/builds/${build_id}/artifacts?api-version=7.1" 2>/dev/null)" || {
      echo "collect-reports-range: failed to list artifacts for build $build_id" >&2
      continue
    }

    # Emit "name<TAB>downloadUrl" for each failureAnalysis_* artifact.
    while IFS=$'\t' read -r art_name download_url; do
      [[ -n "$art_name" && -n "$download_url" ]] || continue

      tmp_dir="$(mktemp -d)"
      zip_path="${tmp_dir}/artifact.zip"

      # The downloadUrl already targets the artifact; request the zip format.
      sep='?'
      [[ "$download_url" == *"?"* ]] && sep='&'
      if ! curl -sS -L -H "$AUTH_HEADER" -o "$zip_path" "${download_url}${sep}\$format=zip" 2>/dev/null; then
        echo "collect-reports-range: failed to download $art_name from build $build_id" >&2
        rm -rf "$tmp_dir"
        continue
      fi
      if ! unzip -o -q "$zip_path" -d "$tmp_dir" 2>/dev/null; then
        echo "collect-reports-range: failed to unzip $art_name from build $build_id" >&2
        rm -rf "$tmp_dir"
        continue
      fi

      # Keep only report.md, uniquely named per build+artifact so nothing collides in the zip.
      report="$(find "$tmp_dir" -type f -name report.md | head -n1)"
      if [[ -n "$report" ]]; then
        cp "$report" "${OUT_DIR}/${build_id}_${art_name}_report.md"
        collected=$((collected + 1))
      else
        echo "collect-reports-range: no report.md in $art_name from build $build_id" >&2
      fi
      rm -rf "$tmp_dir"
    done < <(printf '%s' "$artifacts_json" \
      | jq -r '.value[]? | select((.name // "") | startswith("failureAnalysis_")) | [.name, (.resource.downloadUrl // "")] | @tsv')
  done
done

echo "collect-reports-range: collected $collected report.md file(s) into $OUT_DIR"
