# ADO ↔ GitHub Integration: Pipeline Status Bridge

## Problem Statement

The ACN scheduled release workflow (and future automation) needs to monitor Azure DevOps pipeline status from GitHub Actions. Currently, there's no authenticated path from GitHub → ADO API for the `msazure/One` project.

## Use Cases

1. **Scheduled release workflow** — After tag creation, monitor ACN PR Pipeline and CNI Release Test completion
2. **Future automation** — Any GitHub workflow that needs to know if an ADO pipeline passed/failed
3. **PR gating** — Could enable GitHub merge queue to wait for ADO pipeline results

## Options Evaluated

### Option 1: GitHub → ADO API with OIDC (Direct Polling)

**How it works:**
- Service principal authenticates to ADO REST API using OIDC Bearer token
- Polls `GET /build/builds/{id}` for status

**Requirements:**
- Service principal must be a **user** in the `msazure` ADO org (Stakeholder level, free)
- Federated credential on the SP for GitHub Actions OIDC

**Status:** ❌ Blocked — adding a service principal to `msazure` ADO org requires **Project Collection Administrator** access. The `az devops user add` CLI and portal "Add users" button are restricted.

**Action needed:** An ADO org admin must add the SP. See [setup instructions](#setup-option-1-oidc-to-ado) below.

---

### Option 2: ADO → GitHub Status Bridge (Recommended)

**How it works:**
- ADO pipelines post commit statuses back to GitHub using a **GitHub App**
- GitHub Actions workflow polls its own API (`GET /commits/{sha}/statuses`)
- No ADO authentication needed from GitHub's side

**Architecture:**
```
Tag push → ADO auto-triggers pipeline (native service connection)
         → ADO pipeline starts: posts "pending" status to GitHub
         → ADO pipeline finishes: posts "success"/"failure" to GitHub
         → GitHub workflow polls: gh api commits/<sha>/statuses
         → Sees "success" → pipeline validation complete ✅
```

**Requirements:**
- A GitHub App installed on `Azure/azure-container-networking` with `statuses:write`
- App ID + private key stored as **secret variables** in ADO pipelines
- 2 additional steps in each ADO pipeline (post status at start and end)

**Security:**
- GitHub App secrets live in ADO (not exposed in the public repo)
- ADO pipelines only trigger on protected refs (master, release/*, tags) — not fork PRs
- Even if compromised, attacker can only write commit statuses (cosmetic, no code access)
- Far less privileged than existing ADO service connections

**Status:** ✅ Pattern proven on fork (run #28116621752, all tests passing)

**Precedent:** This is the same pattern used by `cilium-private` repo.

---

### Option 3: PAT-based Authentication

**Status:** ❌ Rejected — PATs in `msazure` expire after max 5 days and cannot be extended. Not viable for automation.

---

### Option 4: Reuse Existing Build Validations SP

**Status:** ❌ Rejected — The `Azure Container Networking - Build Validations - Federated` SP (app ID `6fbbfb6b-8060-4fcc-9ed5-ce48ac11c9b3`) has full subscription access. Overscoped for read-only pipeline monitoring.

---

## Recommended Implementation: Option 2 (ADO → GitHub Bridge)

### Step 1: Create GitHub App

1. Go to `https://github.com/organizations/Azure/settings/apps/new`
2. Settings:
   - **Name:** `acn-pipeline-status` (or similar)
   - **Homepage:** `https://github.com/Azure/azure-container-networking`
   - **Permissions:** Repository → Commit statuses: Read & Write
   - **Events:** None needed
   - **Install on:** Only `Azure/azure-container-networking`
3. Generate a private key and note the App ID

### Step 2: Add Secrets to ADO Pipelines

In ADO project `msazure/One`, add pipeline variables (marked as secret):
- `GITHUB_APP_ID` — The App's numeric ID
- `GITHUB_APP_PRIVATE_KEY` — The PEM private key

These should be added to the variable group used by the ACN pipelines (e.g., `ACN-CNI-Pipeline` or a new group).

### Step 3: Add Status Posting Steps to ADO Pipelines

Add to `.pipelines/pipeline.yaml` and `.pipelines/cni/pipeline.yaml`:

```yaml
# At the start of the pipeline (after checkout)
- bash: |
    # Install GitHub App token generator
    pip install jwt cryptography requests
    
    # Generate installation token
    TOKEN=$(python3 -c "
    import jwt, time, requests
    now = int(time.time())
    payload = {'iat': now - 60, 'exp': now + 600, 'iss': '$(GITHUB_APP_ID)'}
    token = jwt.encode(payload, '''$(GITHUB_APP_PRIVATE_KEY)''', algorithm='RS256')
    # Get installation token
    r = requests.get('https://api.github.com/app/installations', headers={'Authorization': f'Bearer {token}', 'Accept': 'application/vnd.github+json'})
    install_id = r.json()[0]['id']
    r2 = requests.post(f'https://api.github.com/app/installations/{install_id}/access_tokens', headers={'Authorization': f'Bearer {token}', 'Accept': 'application/vnd.github+json'})
    print(r2.json()['token'])
    ")
    
    # Post pending status
    curl -s -X POST \
      -H "Authorization: token $TOKEN" \
      -H "Accept: application/vnd.github+json" \
      "https://api.github.com/repos/Azure/azure-container-networking/statuses/$(Build.SourceVersion)" \
      -d "{\"state\":\"pending\",\"context\":\"ado/$(Build.DefinitionName)\",\"description\":\"Pipeline running...\",\"target_url\":\"$(System.TeamFoundationCollectionUri)$(System.TeamProject)/_build/results?buildId=$(Build.BuildId)\"}"
  displayName: "Post GitHub status: pending"
  condition: always()

# At the end of the pipeline (as a finally/always step)
- bash: |
    # Same token generation as above...
    STATE="failure"
    if [ "$(Agent.JobStatus)" == "Succeeded" ]; then STATE="success"; fi
    
    curl -s -X POST \
      -H "Authorization: token $TOKEN" \
      -H "Accept: application/vnd.github+json" \
      "https://api.github.com/repos/Azure/azure-container-networking/statuses/$(Build.SourceVersion)" \
      -d "{\"state\":\"$STATE\",\"context\":\"ado/$(Build.DefinitionName)\",\"description\":\"Pipeline $STATE\",\"target_url\":\"$(System.TeamFoundationCollectionUri)$(System.TeamProject)/_build/results?buildId=$(Build.BuildId)\"}"
  displayName: "Post GitHub status: result"
  condition: always()
```

### Step 4: Update GitHub Workflow to Poll Statuses

The scheduled release workflow polls for commit statuses:

```yaml
- name: Wait for ACN PR Pipeline
  env:
    GH_TOKEN: ${{ github.token }}
    COMMIT_SHA: ${{ needs.create_tag.outputs.target_sha }}
  run: |
    CONTEXT="ado/ACN PR Pipeline"  # Must match what ADO posts
    TIMEOUT=10800  # 3 hours
    POLL=120  # 2 minutes
    elapsed=0
    while true; do
      STATE=$(gh api "repos/$GITHUB_REPOSITORY/commits/$COMMIT_SHA/statuses" \
        --jq "[.[] | select(.context == \"$CONTEXT\")][0].state // empty")
      if [[ "$STATE" == "success" ]]; then
        echo "✅ $CONTEXT passed"
        break
      elif [[ "$STATE" == "failure" || "$STATE" == "error" ]]; then
        echo "::error::$CONTEXT failed"
        exit 1
      fi
      if (( elapsed >= TIMEOUT )); then
        echo "::error::Timed out waiting for $CONTEXT"
        exit 1
      fi
      echo "Waiting for $CONTEXT... (state=${STATE:-not found yet})"
      sleep $POLL
      elapsed=$((elapsed + POLL))
    done
```

---

## Setup: Option 1 (OIDC to ADO) — For Future Reference

If an ADO org admin becomes available:

1. Add `acn-release-bot` (app ID `75013c7b-3bea-441b-952c-e13d7c9247bc`, object ID `29d8f13a-6ba6-4374-ad34-9cf875064d74`) as a Stakeholder user in `msazure` ADO org

2. Via REST API (requires org admin):
```bash
TOKEN=$(az account get-access-token --resource 499b84ac-1321-427f-aa17-267ca6975798 --query accessToken -o tsv)
curl -X POST \
  -H "Content-Type: application/json" \
  -H "Authorization: Bearer $TOKEN" \
  "https://vsaex.dev.azure.com/msazure/_apis/userentitlements?api-version=7.1-preview.1" \
  -d '{
    "principalName": "75013c7b-3bea-441b-952c-e13d7c9247bc@clients",
    "originId": "29d8f13a-6ba6-4374-ad34-9cf875064d74",
    "subjectKind": "servicePrincipal",
    "user": {
      "principalName": "75013c7b-3bea-441b-952c-e13d7c9247bc@clients",
      "subjectKind": "servicePrincipal"
    },
    "accessLevel": {
      "accountLicenseType": "stakeholder"
    }
  }'
```

3. The `wait-pipeline` CLI command is already implemented and ready to use once access is granted.

---

## Current State (as of PR #4410)

- `wait-pipeline` CLI command: ✅ Written, supports OIDC Bearer token
- Pipeline status polling test: ✅ Proven on fork (8/8 tests passing)
- ADO → GitHub bridge: ⏳ Needs GitHub App creation + ADO pipeline changes
- Direct ADO polling: ⏳ Needs org admin to add SP as ADO user

## Relevant Files

- `tools/release/internal/pipeline/pipeline.go` — ADO polling client (OIDC Bearer auth)
- `tools/release/cmd/release-cli/main.go` — `wait-pipeline` command
- `.github/workflows/scheduled-release.yaml` — Main workflow (Step 4: Pipeline Validation)
- `.pipelines/pipeline.yaml` — ACN PR Pipeline (needs status posting steps)
- `.pipelines/cni/pipeline.yaml` — CNI Release Test (needs status posting steps)

## ADO Pipeline Details

| Pipeline | Definition | Trigger |
|----------|-----------|---------|
| ACN PR Pipeline | `.pipelines/pipeline.yaml` | PR, merge queue, tags (`*`), nightly |
| CNI Release Test | `.pipelines/cni/pipeline.yaml` | Tags (`v*`, `dropgz/*`, `azure-ipam/*`) |
| GitHub Release (unsigned) | Def 393797 in `msazure/One` | Manual |

## Contacts

- **Build Validations SP owners:** Miguel Gonzalez (miguelgo@microsoft.com), John Payne (johnpayne@microsoft.com)
- **ADO org:** `msazure`, Project: `One`
