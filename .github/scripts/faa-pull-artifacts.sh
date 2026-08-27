#!/usr/bin/env bash
# Pulls the Failure Analysis Agent "failureAnalysis" artifacts from an Azure
# DevOps build and lists every issue.md the build produced, for the next workflow
# step to turn into GitHub issues.
#
# The FAA runs in Azure DevOps, which cannot obtain a GitHub token, so it only
# ever publishes issue.md as an artifact. Nothing is dispatched from ADO; this is
# the GitHub-side half that reaches in and collects it.
#
# Auth: an Entra (OIDC) bearer token minted from the workflow's azure/login — no
# stored PAT. The federated identity must be a member of the ADO org with Build
# (read).
#
# Security: downloaded artifacts stay in $RUNNER_TEMP (ephemeral, wiped after the
# run). They are never re-uploaded and their contents are never echoed to logs or
# the job summary — only filenames and counts are.
#
# Inputs (env): ADO_ORG, ADO_PROJECT, BUILD_ID.
# Outputs ($GITHUB_OUTPUT): count, workdir.
set -euo pipefail

: "${ADO_ORG:?ADO_ORG required}"
: "${ADO_PROJECT:?ADO_PROJECT required}"
: "${BUILD_ID:?BUILD_ID required}"

case "$BUILD_ID" in
  ''|*[!0-9]*) echo "::error::buildId must be numeric: $BUILD_ID"; exit 1 ;;
esac

# ADO_ORG / ADO_PROJECT are interpolated into the Azure DevOps URL that carries
# the OIDC bearer token, so constrain them to a safe charset (rejects spaces,
# slashes, and other URL-significant characters) as defense in depth.
case "$ADO_ORG" in
  *[!A-Za-z0-9._-]*) echo "::error::ADO_ORG has unexpected characters: $ADO_ORG"; exit 1 ;;
esac
case "$ADO_PROJECT" in
  *[!A-Za-z0-9._-]*) echo "::error::ADO_PROJECT has unexpected characters: $ADO_PROJECT"; exit 1 ;;
esac

# Azure DevOps resource app ID — constant across tenants. Mint a short-lived
# token for it from the OIDC login instead of storing a PAT.
ado_resource="499b84ac-1321-427f-aa17-267ca6975798"
ado_token="$(az account get-access-token --resource "$ado_resource" --query accessToken -o tsv)"
if [ -z "$ado_token" ]; then
  echo "::error::could not mint an Azure DevOps token from the OIDC login"
  exit 1
fi
auth=(-H "Authorization: Bearer ${ado_token}")

api="https://dev.azure.com/${ADO_ORG}/${ADO_PROJECT}/_apis"
work="$RUNNER_TEMP/faa"
mkdir -p "$work/extracted"

echo "count=0" >> "$GITHUB_OUTPUT"
echo "workdir=$work" >> "$GITHUB_OUTPUT"

# Verify this is a non-PR build. PR builds already get the inline PR comment and
# their fix belongs in the change under review, so they must not spawn an issue
# here. Defense in depth — the agent already gates issue.md on non-PR builds.
build="$(curl -sS -fL "${auth[@]}" "${api}/build/builds/${BUILD_ID}?api-version=7.1")"
reason="$(jq -r '.reason // ""' <<<"$build")"
echo "build $BUILD_ID reason=$reason"
if [ "$reason" = "pullRequest" ]; then
  echo "::notice::build $BUILD_ID is a pull-request build; nothing to do"
  exit 0
fi

arts="$(curl -sS -fL "${auth[@]}" "${api}/build/builds/${BUILD_ID}/artifacts?api-version=7.1")"
mapfile -t rows < <(jq -r '.value[] | select(.name | startswith("failureAnalysis")) | "\(.name)\t\(.resource.downloadUrl)"' <<<"$arts")
if [ "${#rows[@]}" -eq 0 ]; then
  echo "::notice::no failureAnalysis artifacts on build $BUILD_ID"
  exit 0
fi

for row in "${rows[@]}"; do
  name="${row%%$'\t'*}"
  url="${row#*$'\t'}"
  [ -n "$url" ] || continue
  # Artifact names come from the pipeline template and land in a filesystem path,
  # so reject anything that could escape $work.
  case "$name" in
    *[!A-Za-z0-9._-]*) echo "skipping artifact with unexpected name: $name"; continue ;;
  esac
  echo "downloading artifact $name"
  curl -sS -fL "${auth[@]}" -o "$work/${name}.zip" "$url"
  unzip -q -o "$work/${name}.zip" -d "$work/extracted/${name}"
done

# Collect every issue.md the build produced — one per scenario the escalation
# gate judged to need a code fix. Most builds produce none.
: > "$work/issuelist.txt"
find "$work/extracted" -type f -name issue.md >> "$work/issuelist.txt" || true
count="$(grep -c . "$work/issuelist.txt" || true)"
count="${count:-0}"
echo "found $count issue.md file(s)"
echo "count=$count" >> "$GITHUB_OUTPUT"
