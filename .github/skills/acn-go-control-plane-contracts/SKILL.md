---
name: acn-go-control-plane-contracts
description: "Go control-plane contract design for Azure/azure-container-networking. Use when changing CRD Go types, status/condition fields, response-code mapping, controller/reconciler bootstrap logic, or generated contract artifacts such as CRD manifests and rendered Dockerfiles. Trigger on edits under crd/, API type packages, response/status code enums, reconciler init or migration code, or any change that affects externally consumed state."
user-invocable: true
license: MIT
compatibility: Designed for Claude Code or similar AI coding agents, and for projects using Golang in Kubernetes/controller-style systems.
metadata:
  author: rbtr
  version: "1.0.0"
allowed-tools: Read Edit Write Glob Grep Bash(go:*) Bash(golangci-lint:*) Bash(git:*) Agent
---

**Persona:** You are a Go control-plane engineer. You treat `status`, `conditions`, `response codes`, and checked-in generated artifacts as public contracts. If users, controllers, dashboards, or other binaries can observe it, it is API surface and must be designed as deliberately as any exported Go type.

**Modes:**

- **Write mode** — designing or changing CRDs, response/status enums, or reconciliation/bootstrap flows. Preserve contract clarity, idempotence, and compatibility.
- **Review mode** — reviewing API/controller changes. Flag raw string status values, duplicated semantics, non-idempotent bootstrap logic, and hand-edited generated artifacts.
- **Audit mode** — auditing controller/API contract quality. Split work across: (1) typed contract surfaces, (2) reconcile/bootstrap safety, (3) generated artifact hygiene.

> **Repo-specific skill.** This focuses on the kinds of controller/API/CRD patterns repeatedly used in Azure/azure-container-networking.

# Go Control-Plane Contracts

In ACN, CRD types, response codes, and observed status are not internal implementation details. They are consumed by other code, by kubectl users, by dashboards, and by future versions of the system. Design them as contracts.

## Use when

- editing files under `crd/`
- adding or changing `Status`, `Condition`, `Reason`, or `ResponseCode` values
- wiring bootstrap / migration / init flows in reconcilers or services
- changing what gets persisted, surfaced, or generated as a checked-in artifact
- updating generated CRDs or rendered Dockerfiles

## Do not use when

- the change is purely local business logic with no externally observed state
- the question is only about low-level Go mechanics already covered by other skills

## Core Principles

1. **Status is API contract** — if users or controllers observe it, it must be typed, stable, and intentionally named.
2. **Typed public codes beat raw literals** — `Status`, `Condition`, `Reason`, and `ResponseCode` surfaces use typed constants, not ad-hoc strings/ints.
3. **Spec is desired state; status is observed state** — counts, allocations, versions, and reconciliation results belong in `status`, not `spec`.
4. **Bootstrap and migration must be safe to replay** — reconcile from prior state, use fallback/version gating when needed, and make repeated execution converge.
5. **Reconciliation must be idempotent and stable** — avoid churn, duplicate side effects, and control-loop thrash.
6. **Generated artifacts are checked-in contracts** — edit the source, regenerate the artifact, and keep rendered/generated outputs in sync.

## Status Is API Contract

CRD `status` is not just debug info. It is the system's observed truth and should be shaped for operators and other controllers.

```go
// ✅ GOOD — desired state in spec, observed state in status
// +kubebuilder:subresource:status
// +kubebuilder:printcolumn:name="Requested IPs",type=integer,JSONPath=`.spec.requestedIPCount`
// +kubebuilder:printcolumn:name="Allocated IPs",type=integer,JSONPath=`.status.assignedIPCount`
type NodeNetworkConfig struct {
    Spec   NodeNetworkConfigSpec   `json:"spec,omitempty"`
    Status NodeNetworkConfigStatus `json:"status,omitempty"`
}

type NodeNetworkConfigStatus struct {
    AssignedIPCount int                 `json:"assignedIPCount,omitempty"`
    NetworkContainers []NetworkContainer `json:"networkContainers,omitempty"`
}
```

Design `status` so it answers the operational question directly. If users need the count of allocated IPs, put the count in `status`; don't make everyone derive it by replaying internal state.

### Do not duplicate existing platform semantics

If Kubernetes or the control plane already provides the semantic, don't invent a second shadow status for it.

```go
// ❌ BAD — inventing a second "Deleting" contract
type MTPNCStatus struct {
    Status string `json:"status,omitempty"` // now who sets "Deleting"?
}

// ✅ GOOD — use the platform's deletion signal
if !obj.DeletionTimestamp.IsZero() {
    // object is deleting
}
```

Using `DeletionTimestamp.IsZero()` is the canonical contract. A home-grown `Deleted` or `Deleting` status risks drift and missed transitions.

## Typed Public Codes and States

Any value that crosses an API/controller boundary should be typed.

```go
// ✅ GOOD — public response codes are typed
type ResponseCode int

const (
    Success         ResponseCode = 0
    InvalidRequest  ResponseCode = 23
    NotFound        ResponseCode = 14
    UnexpectedError ResponseCode = 99
)
```

The same rule applies to `Status`, `Condition`, `Reason`, and other public contract values:

```go
type PodNetworkStatus string

const (
    PodNetworkStatusPending PodNetworkStatus = "Pending"
    PodNetworkStatusReady   PodNetworkStatus = "Ready"
)

func IsTerminalStatus(s PodNetworkStatus) bool {
    return s == PodNetworkStatusReady
}
```

### Map internal errors to typed public surfaces

Internal Go errors are for code. Public response codes and status values are for contracts.

```go
// ✅ GOOD — internal error mapped to public code
respCode := types.Success
if err := req.Validate(); err != nil {
    logger.Error("invalid request", zap.Error(err))
    respCode = types.InvalidRequest
}

response := cns.PostNetworkContainersResponse{
    Response: cns.Response{
        ReturnCode: respCode,
    },
}
```

Do not expose behavior by matching on `err.Error()` strings at the boundary. Convert once into a typed public code/status.

## Bootstrap and Migration Safety

Control-plane startup often needs to restore or reconcile from prior state. That logic must be safe, restartable, and version-aware.

```go
// ✅ GOOD — bootstrap from prior provider, then converge normally
func reconcileInitialCNSState(
    nnc *v1alpha.NodeNetworkConfig,
    ipamReconciler ipamStateReconciler,
    podInfoByIPProvider cns.PodInfoByIPProvider,
    isSwiftV2 bool,
) error {
    if len(nnc.Status.NetworkContainers) == 0 {
        return errors.New("no network containers found in NNC status")
    }
    // restore prior PodInfo state
    // transform to current request format
    // hand off to normal reconciliation path
    return nil
}
```

Patterns to prefer:

- reconcile from existing provider/state instead of rebuilding blindly
- validate capability/version before switching modes
- keep migration logic explicit and isolated
- let existing controller retry/backoff machinery do retries when possible

Patterns to avoid:

- startup code with hidden side effects or ad-hoc retry trees
- parallel bootstrap paths that can disagree on contract interpretation
- silently tolerating impossible prior state

## Stable, Idempotent Reconciliation

A reconcile loop should converge to the desired observed state. Re-running it should not produce duplicated side effects or flap external state.

```go
// ✅ GOOD — state-derived, replay-safe reconciliation
for _, nc := range nnc.Status.NetworkContainers {
    if err := sink(nc); err != nil {
        return reconcile.Result{}, errors.Wrap(err, "failed to push NC update")
    }
}
```

Keep reconciliation:

- **idempotent** — safe to repeat
- **state-driven** — derived from current observed/desired data, not hidden history
- **stable** — use in-flight tracking or thresholds when the source of truth lags and the loop would otherwise thrash

If a loop can flap because upstream state trails reality, prefer sticky state or hysteresis rather than churn. See `acn-go-design-boundaries` for the detailed hysteresis rule.

## Generated Artifact Hygiene

Generated artifacts in this repo are checked-in contract surfaces. Do not hand-edit the rendered output.

### CRDs

CRD manifests are generated from Go types and checked in. Change the API type, then regenerate:

```bash
make regenerate-crd
```

If you update `crd/**/api/...` without regenerating the manifest, the checked-in contract is stale.

### Rendered Dockerfiles

Rendered Dockerfiles explicitly point back to their source template:

```Dockerfile
# SOURCE: cns/Dockerfile.tmpl
```

Edit the template, not the rendered file, then regenerate rendered outputs through the repo tooling. The checked-in rendered file is contract output, not authoring source.

### Shared build tools are repo-owned

Repo tooling is versioned and reproducible (`build/tools/go.mod`, `go tool -modfile=...`, renderkit, golangci-lint). Do not assume local global tools are the source of truth.

## Common Mistakes

| Mistake | Fix |
| --- | --- |
| Raw string status values in public structs | Define typed `Status`/`Condition`/`Reason` constants |
| Returning behavior via `err.Error()` matching | Map internal errors to typed `ResponseCode` or public status |
| Putting observed counts in `spec` | Put observed state in `status` |
| Inventing `Deleting` status when metadata already signals deletion | Use `DeletionTimestamp.IsZero()` |
| Startup bootstrap with hidden retries and side effects | Make bootstrap explicit, replay-safe, and idempotent |
| Hand-editing generated CRD manifest | Edit API type, regenerate CRD |
| Editing rendered Dockerfile directly | Edit `Dockerfile.tmpl`, regenerate rendered file |
| Reconcile loop that keeps churning external state | Use state-derived idempotence and stable convergence |

## Cross-References

- → See `acn-go-types-parsing` for typed enums, predicate helpers, and strong domain types
- → See `acn-go-design-boundaries` for behavior-vs-scenario config, fail-fast invariants, and hysteresis
- → See `acn-go-interfaces-dependencies` for narrow dependency surfaces and migration shims
- → See `acn-go-context-lifecycle` for bootstrap ownership and controller/process lifecycle
