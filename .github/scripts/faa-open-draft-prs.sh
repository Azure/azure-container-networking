#!/usr/bin/env bash
# Opens a draft PR from each fix.md prompt pulled from Azure DevOps, assigns the
# GitHub Copilot coding agent to author the fix, and best-effort pings the shared
# Teams notifier bot.
#
# Security: fix.md content is written only to the draft PR body (its intended
# destination) and committed under .github/faa-fixes/. It is never echoed to logs
# or the job summary.
#
# Inputs (env): GH_TOKEN, REPO, BASE_BRANCH, WORKDIR, BUILD_ID and (optional Teams)
# NOTIFIER_ENDPOINT, NOTIFIER_API_AUDIENCE, NOTIFY_TEAM_ID, NOTIFY_CHANNEL_ID,
# NOTIFY_CC.
set -euo pipefail

: "${GH_TOKEN:?GH_TOKEN required}"
: "${REPO:?REPO required}"
: "${BASE_BRANCH:?BASE_BRANCH required}"
: "${WORKDIR:?WORKDIR required}"

git config --local user.name "github-actions[bot]"
git config --local user.email "41898282+github-actions[bot]@users.noreply.github.com"
git remote set-url origin "https://x-access-token:${GH_TOKEN}@github.com/${REPO}.git"

# meta <file> <key> — read a value from the fix.md metadata header.
meta() { sed -n '/<!-- faa-fix:v1/,/-->/p' "$1" | sed -n "s/^$2: //p" | head -n1; }

# assign_copilot <pr-number> — hand the draft PR to the Copilot coding agent by
# assigning its bot actor. Falls back to an @copilot comment if the actor is not
# assignable with this token.
assign_copilot() {
  local pr="$1" owner repo actor_id pr_id
  owner="${REPO%%/*}"; repo="${REPO#*/}"
  actor_id="$(gh api graphql -f query='
    query($owner:String!,$name:String!){
      repository(owner:$owner,name:$name){
        suggestedActors(capabilities:[CAN_BE_ASSIGNED], first:100){
          nodes{ login __typename ... on Bot { id } ... on User { id } }
        }
      }
    }' -F owner="$owner" -F name="$repo" \
    --jq '.data.repository.suggestedActors.nodes[] | select(.login=="copilot-swe-agent" or .login=="Copilot") | .id' 2>/dev/null | head -n1 || true)"
  pr_id="$(gh pr view "$pr" --repo "$REPO" --json id --jq .id 2>/dev/null || true)"
  if [ -n "$actor_id" ] && [ -n "$pr_id" ] && gh api graphql -f query='
    mutation($assignableId:ID!,$actorIds:[ID!]!){
      replaceActorsForAssignable(input:{assignableId:$assignableId, actorIds:$actorIds}){ clientMutationId }
    }' -F assignableId="$pr_id" -F actorIds="$actor_id" >/dev/null 2>&1; then
    echo "assigned Copilot coding agent to PR #$pr"
  else
    echo "could not assign Copilot; leaving an @copilot mention instead"
    gh pr comment "$pr" --repo "$REPO" --body \
      "@copilot please implement the fix described in this PR. Keep it a draft and make the smallest correct change." || true
  fi
}

# notify_teams <pr-url> <title> <owner> — best-effort Teams ping via the shared
# notifier bot, using the Azure OIDC login already established by the workflow.
notify_teams() {
  local pr_url="$1" title="$2" owner="$3"
  if [ -z "${NOTIFIER_ENDPOINT:-}" ] || [ -z "${NOTIFIER_API_AUDIENCE:-}" ] || [ -z "${NOTIFY_TEAM_ID:-}" ] || [ -z "${NOTIFY_CHANNEL_ID:-}" ]; then
    echo "notifier vars incomplete; skipping Teams notify"; return 0
  fi
  local token
  token="$(az account get-access-token --resource "$NOTIFIER_API_AUDIENCE" --query accessToken -o tsv 2>/dev/null || true)"
  [ -n "$token" ] || { echo "could not mint notifier token; skipping"; return 0; }

  # cc mentions: configured UPNs plus the recommended owner when it is an email.
  local mentions='[]'
  local csv="${NOTIFY_CC:-}"
  case "$owner" in *@*) csv="${csv:+$csv,}$owner" ;; esac
  if [ -n "$csv" ]; then
    mentions="$(echo "$csv" | tr ',' '\n' | sed 's/^ *//;s/ *$//' | grep -v '^$' | jq -R . | jq -s .)"
  fi
  local cc='null'
  if [ "$(jq 'length' <<<"$mentions")" -gt 0 ]; then
    cc="$(jq -n --argjson m "$mentions" '{label:"Owner", mentions:$m}')"
  fi
  local payload
  payload="$(jq -n \
    --arg source "faa-draft-pr" --arg runId "$BUILD_ID" \
    --arg teamId "$NOTIFY_TEAM_ID" --arg channelId "$NOTIFY_CHANNEL_ID" \
    --arg title "$title" --arg pr "$pr_url" --argjson cc "$cc" \
    '{source:$source, runId:$runId, teamId:$teamId, channelId:$channelId,
      title:$title, summary:("Draft PR opened for a high-confidence regression: " + $pr),
      status:"running", severity:"warning",
      facts:[{name:"Draft PR", value:$pr, url:$pr}], mentionChannel:true}
     + (if $cc != null then {cc:$cc} else {} end)')"
  local code
  code="$(curl -sS -o /dev/null -w '%{http_code}' -X POST \
    -H "Authorization: Bearer $token" -H "Content-Type: application/json" \
    --data "$payload" "${NOTIFIER_ENDPOINT}/api/notifications" || echo 000)"
  echo "notifier POST -> HTTP $code"
}

created=0
declare -A seen
while IFS= read -r fix; do
  [ -n "$fix" ] || continue
  fp="$(meta "$fix" fingerprint)"
  title="$(meta "$fix" title)"
  owner="$(meta "$fix" owner)"
  # Untrusted artifact: fp flows into a git ref and a file path, so require a
  # lowercase-hex digest (blocks bad refs and ../ path traversal).
  case "$fp" in
    ''|*[!0-9a-f]*) echo "skipping $fix: invalid fingerprint '$fp'"; continue ;;
  esac
  if [ -n "${seen[$fp]:-}" ]; then echo "skipping duplicate fingerprint $fp"; continue; fi
  seen[$fp]=1
  [ -n "$title" ] || title="[FAA] Draft fix for regression ${fp:0:12}"

  branch="faa/fix-${fp}"

  # Idempotency: skip if an open PR or the branch already exists.
  existing="$(gh pr list --repo "$REPO" --state open --head "$branch" --json number --jq 'length' 2>/dev/null || echo 0)"
  if [ "$existing" != "0" ]; then echo "draft PR already open for $branch; skipping"; continue; fi
  if git ls-remote --exit-code --heads origin "$branch" >/dev/null 2>&1; then
    echo "branch $branch already exists; skipping"; continue
  fi

  # Create the branch carrying the prompt so it diverges from base and Copilot
  # has fix.md in-repo to work from.
  git switch -c "$branch" "origin/${BASE_BRANCH}" 2>/dev/null || git switch -c "$branch"
  mkdir -p .github/faa-fixes
  cp "$fix" ".github/faa-fixes/${fp}.md"
  git add ".github/faa-fixes/${fp}.md"
  git commit -m "chore(faa): draft fix prompt for ${fp:0:12}" >/dev/null
  git push origin "$branch"

  pr_url="$(gh pr create --repo "$REPO" --draft --head "$branch" --base "$BASE_BRANCH" \
    --title "$title" --body-file "$fix")"
  echo "opened draft PR: $pr_url"
  pr_num="${pr_url##*/}"

  assign_copilot "$pr_num"
  notify_teams "$pr_url" "$title" "$owner"
  created=$((created + 1))
done < "$WORKDIR/fixlist.txt"

echo "created $created draft PR(s)"
if [ "$created" = "0" ]; then
  echo "::notice::no new draft PRs (all fingerprints already had branches/PRs)"
fi
