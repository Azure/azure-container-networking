# Failure-Agent MVP Demo Overview

## Purpose

This document describes the current demo-able MVP for the ACN failure-agent and separates the active non-OneBranch demo path from production features that remain implemented but are intentionally disabled in the pipeline for incremental rollout.

The MVP goal is:

```text
ACN pipeline failure
  -> failedE2ELogs captures the failure evidence bundle while the cluster is alive
  -> failure-agent downloads/parses the bundle
  -> failure-agent runs fixed-set read-only kubectl diagnostics
  -> fingerprinting and signature matching
  -> Azure OpenAI LLM analysis
  -> report and proposed fix
  -> GitHub PR comment upsert
```

This is not only a post-error log summarizer. The agent is positioned between pipeline failure and cluster teardown. It uses the pipeline's live-collected failure bundle plus its own supplemental fixed-set kubectl snapshot.

---

## MVP Scope

### Active for demo

| Capability | Status | Notes |
|---|---:|---|
| Failed logs/evidence bundle | Enabled | Produced by `failedE2ELogs` from the still-live failed cluster. |
| Supplemental live diagnostics | Enabled | `failure-analysis.job.yaml` defaults `live: true`. |
| Fixed kubectl command set | Enabled | The LLM does not choose arbitrary commands. |
| Read-only command policy | Enabled | Mutating, streaming, and interactive commands are denied. |
| Fingerprinting | Enabled | Used to identify the failure pattern. |
| Signature hints | Enabled | Deterministic signatures are LLM context, not fallback classification. |
| Azure OpenAI LLM analysis | Enabled | Produces summary, root cause, confidence, and proposed fix. |
| Report artifacts | Enabled | Writes `report.md` and `incident.json`. |
| GitHub PR upsert | Enabled | Posts or updates PR comment when `GITHUB_TOKEN` is available and dry-run is false. |

### Implemented but disabled for MVP

| Capability | MVP state | Why disabled now | Re-enable path |
|---|---:|---|---|
| SQL knowledge DB | Commented out in pipeline | First demo should prove AOAI + GitHub upsert without persistence complexity. | Re-enable `--knowledge-db`. |
| Duplicate skip | Disabled with knowledge DB | Requires persistent incident memory. | Re-enable after DB path persists across runs. |
| Prior incident prompt context | Disabled with knowledge DB | Requires resolved/unresolved incident history. | Re-enable with SQL store. |
| Flakiness analytics | Commented out in pipeline | Requires repeated runs and persistent history. | Re-enable `--flakiness-output`. |
| PR lifecycle sync | Not required for MVP | MVP only needs PR comment upsert. | Wire after persisted incident IDs are stable. |
| Retention enforcement / sweeper | Advisory only | Teardown gating should be added after core demo succeeds. | Consume `retentionDecision` in delete/sweeper job. |

---

## Project Flow

```text
1. ACN test job fails
   |
   v
2. failedE2ELogs runs
   - Uses kubectl while the cluster is still alive
   - Captures kube-system logs, all pods, bad pod info, node status,
     daemonset/deployment images, CNS/CNI/Cilium state where applicable
   - Publishes the failure evidence artifact
   |
   v
3. failureAnalysis job runs
   - Depends on failedE2ELogs
   - Downloads the failure evidence artifact
   - Sets up Go
   - Runs tools/failure-agent
   |
   v
4. failure-agent parses the evidence bundle
   |
   v
5. failure-agent runs supplemental fixed-set live diagnostics
   - kubectl get pods
   - kubectl get nodes
   - kubectl describe nodes
   - kubectl get events
   - kubectl get daemonsets
   - kubectl logs azure-cns
   - kubectl logs cilium
   |
   v
6. failure-agent combines evidence and computes a fingerprint
   |
   v
7. failure-agent sends evidence + signature hints to Azure OpenAI
   |
   v
8. LLM returns classification
   - Root cause summary
   - Evidence explanation
   - Confidence
   - Proposed fix
   |
   v
9. failure-agent writes artifacts
   - report.md
   - incident.json
   |
   v
10. failure-agent upserts GitHub PR comment
```

---

## Data Flow

```text
                  Live failed cluster
                           |
                           | failedE2ELogs kubectl collection
                           v
              +-----------------------------+
              | Failure evidence artifact   |
              | - kube-system logs          |
              | - pod/node state            |
              | - bad pod info              |
              | - CNS/CNI/Cilium artifacts  |
              +--------------+--------------+
                             |
                             | DownloadPipelineArtifact
                             v
              +-----------------------------+
              | failure-agent --input       |
              +--------------+--------------+
                             |
                             | Parse evidence bundle
                             v
              +-----------------------------+
              | Parsed evidence             |
              +--------------+--------------+
                             |
                             | --live fixed-set kubectl snapshot
                             v
              +-----------------------------+
              | Combined evidence           |
              | - failedE2ELogs artifact    |
              | - live diagnostic snapshot  |
              +--------------+--------------+
                             |
              +--------------+--------------+
              |                             |
              v                             v
  +-----------------------+     +-----------------------+
  | Fingerprint compute   |     | Signature matching    |
  +-----------+-----------+     +-----------+-----------+
              |                             |
              +--------------+--------------+
                             |
                             v
              +-----------------------------+
              | Azure OpenAI classifier     |
              +--------------+--------------+
                             |
                             v
              +-----------------------------+
              | Incident and report         |
              | - summary                   |
              | - proposed fix              |
              | - confidence                |
              | - evidence excerpts         |
              +--------------+--------------+
                             |
                             v
              +-----------------------------+
              | GitHub PR comment upsert    |
              +-----------------------------+
```

---

## Why This Is Not Just Post-Error Analysis

The failure-agent does not only read static logs after the cluster is gone.

There are two evidence layers:

1. **Primary live evidence bundle**
   - `failedE2ELogs` runs after failure while the cluster still exists.
   - It uses kubectl to capture logs, node state, pod state, bad pod details, and ACN/CNI/CNS/Cilium artifacts.
   - That artifact becomes the failure-agent input bundle.

2. **Supplemental live diagnostics**
   - The failure-agent runs an additional fixed read-only kubectl set through `--live`.
   - This gives the LLM a fresh point-in-time snapshot before teardown.

Together:

```text
failedE2ELogs artifact + failure-agent fixed-set live diagnostics = MVP evidence
```

---

## Fixed Kubectl Access

For the MVP, the LLM does not generate kubectl commands. The command set is fixed and policy-gated.

Supplemental diagnostic commands:

```text
kubectl get pods -A -o wide
kubectl get nodes -o wide
kubectl describe nodes
kubectl get events -A --sort-by=.lastTimestamp
kubectl get daemonsets -n kube-system -o wide
kubectl logs -n kube-system -l k8s-app=azure-cns --tail=200 --prefix
kubectl logs -n kube-system -l k8s-app=cilium --tail=200 --prefix
```

Command policy:

```text
Allowed:
  kubectl get
  kubectl describe
  kubectl logs
  kubectl events

Denied:
  apply
  patch
  delete
  exec
  cp
  scale
  port-forward
  rollout restart
  watch/follow/streaming flags
```

This lets the agent observe the failed cluster but prevents mutation.

---

## Non-OneBranch Pipeline Template Changes for MVP

The MVP pipeline wiring is intentionally limited to the non-OneBranch ACN PR and CNI release-test paths. OneBranch wiring is excluded for now to avoid introducing new behavior into that path while it remains unstable.

File:

```text
.pipelines/templates/failure-analysis.job.yaml
```

Active MVP setting:

```yaml
# MVP demo: keep supplemental fixed-set live diagnostics enabled.
live: true
```

Commented out production features:

```yaml
# MVP demo: knowledge-store incident memory/dedupe/prior context is
# intentionally disabled in the pipeline. The code remains in place;
# re-enable after AOAI analysis and GitHub upsert are proven.
# if [ -n "${{ parameters.knowledgeDb }}" ]; then
#   EXTRA_FLAGS="$EXTRA_FLAGS --knowledge-db ${{ parameters.knowledgeDb }}"
# fi

# MVP demo: flakiness analytics requires a persistent knowledge store
# and repeated runs, so leave it disabled for the first demo.
# if [ -n "${{ parameters.flakinessOutput }}" ]; then
#   EXTRA_FLAGS="$EXTRA_FLAGS --flakiness-output ${{ parameters.flakinessOutput }}"
# fi
```

Retention remains advisory for MVP:

```text
retentionDecision is surfaced, but teardown enforcement is re-enabled incrementally after the core demo.
```

---

## Tests Supporting the MVP

Recommended test command:

```bash
cd tools/failure-agent
go test ./...
```

Focused MVP checks:

```bash
go test ./internal/command -v
go test ./internal/live -v
go test ./internal/classify -v
go test . -v -run "TestRunEndToEnd|TestRunAnalysisFailedWhenLLMFails|TestRunMergesLiveDiagnostics"
```

What these validate:

| Area | Validates |
|---|---|
| `internal/command` | Read-only kubectl allow/deny policy. |
| `internal/live` | Fixed-set live collector and evidence merge. |
| `internal/classify` | LLM request/response and error behavior. |
| `pipeline_test.go` | End-to-end evidence, fingerprint, classify, report flow with fakes. |

---

## Demo Inputs Needed

```text
AZURE_OPENAI_ENDPOINT
AZURE_OPENAI_DEPLOYMENT
GITHUB_TOKEN
KUBECONFIG or pipeline cluster access
failedE2ELogs artifact
```

The MVP intentionally does not require:

```text
Persistent SQL DB
Knowledge graph
Flakiness dashboard
Retention sweeper
PR lifecycle state sync
```

---

## Demo Acceptance Criteria

```text
1. ACN pipeline failure triggers failedE2ELogs.
2. failedE2ELogs publishes a failure evidence artifact.
3. failureAnalysis downloads that artifact.
4. failure-agent runs with --live enabled.
5. The fixed kubectl set executes through the read-only policy.
6. The agent computes a fingerprint.
7. Azure OpenAI returns summary, root cause, confidence, and proposed fix.
8. report.md and incident.json are created.
9. A GitHub PR comment is inserted or updated.
```

---

## Incremental Re-Enable Plan

After the MVP demo works:

```text
1. Re-enable --knowledge-db
   - Persist SQLite across runs.
   - Enable incident memory.

2. Re-enable duplicate handling and prior context
   - Use active fingerprint lookup.
   - Inject resolved/unresolved incidents into the prompt.

3. Re-enable --flakiness-output
   - Publish recurring fingerprint and hotspot reports.

4. Wire PR lifecycle sync
   - Map GitHub PR state to incident lifecycle.

5. Enforce retention
   - Consume retentionDecision in delete/sweeper job.
   - Keep failed clusters when required.
   - Apply TTL/lock and later cleanup.
```

This keeps the demo focused while preserving the full production implementation plan.
