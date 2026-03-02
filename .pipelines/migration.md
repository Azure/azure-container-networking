# CI Migration: Azure DevOps → GitHub Actions

This document describes the phased plan to migrate CI from Azure Pipelines (ADO/AZP) to
GitHub Actions (GHA). It is intended for evaluation, review, and distribution as a work plan.

## Background and Constraints

- **Production image publishing** has been offloaded to a partner service. This repo's CI
  will no longer produce or publish production images. Do not reproduce prod image publishing
  in GHA.
- **NPM is deprecated.** All NPM-related pipelines and workflows are frozen. No new investment
  in NPM CI — leave ADO pipelines in place and leave existing GHA cyclonus workflows running
  as-is, but do not extend them.
- **CNS/CNI cannot be tested on Kind.** They require real AKS clusters to exercise properly.
  Kind-based integration tests are not a viable substitute.
- **GHCR is the target registry** for test images built in GHA. Old images must be flushed
  regularly with a retention/cleanup workflow.
- **OneBranch / `run-pipeline.yaml`** is not a migration target and is expected to be retired
  as part of production offload (including signed binaries).
- **Binary artifacts** are not distributed from this repo. Binary build jobs are low-priority
  migration targets, useful only to prove cross-platform compilation works.

## What Already Exists in GHA

These workflows are already in `.github/workflows/` and require no migration work:

| Workflow | Purpose |
|---|---|
| `golangci.yaml` | Go linting (Linux + Windows), BPF code generation |
| `baseimages.yaml` | Validates Dockerfiles are regenerated from current base images |
| `crdgen.yaml` | Validates CRD files are regenerated |
| `codeql.yaml` | CodeQL security scanning (Linux + Windows) |
| `cyclonus-netpol-test.yaml` | NPM network policy tests on Kind — **frozen, no new development** |
| `cyclonus-netpol-extended-nightly-test.yaml` | Extended nightly NPM cyclonus — **frozen** |
| `stale.yaml` | Automated stale issue/PR management |

## What Stays in ADO Permanently

| Pipeline | Reason |
|---|---|
| `cni/ado-automation/var-pipeline.yaml` | Uses ADO REST API for variable group management |
| `swiftv2-long-running/pipeline.yaml` | Very high infra complexity; re-evaluate later |
| All `npm/` pipelines | NPM deprecated; frozen in place |

## Phased Migration Plan

### Continuity Rules

- Run ADO and GHA equivalents **in parallel** throughout each phase.
- Do not retire an ADO stage until the GHA equivalent has passed cleanly on ≥ 5 consecutive PRs.
- **PR-blocking required checks must remain covered at all times.** Update branch protection
  rules as GHA jobs come online.
- Every phase must be independently verifiable before starting the next.

### Merge Queue Coverage Model

- This repo uses merge queue; migration must preserve two execution lanes:
  - `PR fast lane`: small/high-signal checks on `pull_request` for rapid developer feedback.
  - `merge queue slow lane`: broader/slower checks on `merge_group` after approval.
- A check is not considered migrated until both lanes are covered where applicable.
- For required checks, ensure merge-queue-context checks are configured in branch protection,
  not only PR-context checks.

### Migration Governance (Applies to All Phases)

- Use a three-step rollout for each migrated check:
  - `shadow`: run in GHA but do not gate merge, compare against ADO outcomes.
  - `required`: add the GHA check to branch protection, keep ADO check required.
  - `cutover`: remove ADO required check after stability criteria are met.
- Require explicit success gates before cutover:
  - ≥ 5 consecutive PR matches between ADO and GHA for the same signal.
  - No critical false-pass cases observed in GHA.
  - Flake rate and runtime are within agreed SLO (document in PR when cutover happens).
- Keep a direct ADO→GHA mapping table in this file updated as phases complete.
- Prefer reusable workflows (`workflow_call`) for shared setup (Go toolchain, Azure auth,
  log collection) to avoid drift across multiple workflow files.

---

### Phase 0 - CI Control Plane and Baseline

**Value:** Highest leverage. Prevents migration drift and accidental coverage regressions.  
**New files:**
- `.github/workflows/_reusable-go-setup.yaml` (optional reusable setup)
- `.github/workflows/_reusable-azure-auth.yaml` (OIDC login wrapper)
- `.github/workflows/required-checks-audit.yaml` (optional weekly audit)

#### Scope

- Define standard workflow defaults for all new GHA workflows:
  - `permissions` set to least privilege per workflow/job.
  - `concurrency` keys to prevent duplicate expensive runs.
  - `timeout-minutes` on all jobs, especially AKS jobs.
  - Cancel superseded PR runs (`cancel-in-progress: true`) where safe.
- Add workflow authoring standards:
  - Pin third-party actions to a commit SHA where practical.
  - Use `actions/checkout` with minimal fetch depth unless history is required.
  - Upload diagnostics/artifacts on failure for all AKS jobs.
- Capture baseline metrics from ADO for migrated checks:
  - median runtime
  - failure rate
  - flaky rate (rerun-pass behavior)

#### Verification

- [ ] All new GHA workflows follow the common defaults above
- [ ] Baseline metrics are recorded before Phase 1 cutover
- [ ] Required checks rollout process (`shadow` → `required` → `cutover`) is documented in repo settings notes/PRs

---

### Phase 1 — Unit Tests & Coverage

**Value:** Highest. Zero infrastructure required. Replaces the most-run ADO stage.  
**Source ADO pipeline:** `.pipelines/templates/run-unit-tests.yaml`, `.pipelines/pipeline.yaml`  
**New file:** `.github/workflows/unit-tests.yaml`

#### Jobs

Execution profiles:
- `PR fast lane`:
  - Run Linux unit tests on `pull_request` for fast feedback.
  - Run Windows subset only if runtime stays within agreed limits.
- `merge queue slow lane`:
  - Run full Linux + Windows unit matrix and coverage generation on `merge_group`.

**Linux** (`ubuntu-latest`):
- Install BPF prerequisites (`llvm`, `clang`, `libbpf-dev`)
- `make bpf-lib`
- `go generate ./...`
- `make test-all` — runs `test-main` + `test-azure-ipam` + `test-azure-ip-masq-merger` +
  `test-azure-iptables-monitor` + `test-cilium-log-collector`
- All tests run with `-race` flag and `-covermode atomic`
- Produces `coverage-*.out` files per module
- Upload combined coverage artifact

**Windows** (`windows-latest`):
- Audit which packages have Linux-only build tags before including `./cni/...`
  (start conservatively with `./cns/... ./platform/...` and expand)
- `go test -timeout 30m -covermode atomic -coverprofile=windows-coverage.out <packages>`

**Coverage** (depends on Linux job):
- Merge `coverage-*.out` files from all modules
- Convert: `coverage.out` → `coverage.json` → `coverage.xml` (Cobertura) via `gocov` + `gocov-xml`
- Upload XML as artifact and/or push to Codecov
- Track regression: allow ≤ 0.25% variance on PRs to `master` (matches current ADO config)

#### Triggers
```yaml
on:
  pull_request:
    branches: [master, "release/*"]
  merge_group:
    types: [checks_requested]
  push:
    branches: [master]
  schedule:
    - cron: "0 2 * * *"   # nightly 2am UTC, matches ADO schedule
  workflow_dispatch:
```

Trigger intent:
- `pull_request`: fast lane.
- `merge_group`: slow lane and gate parity with current ADO post-approval behavior.

#### Reference patterns
- Go setup, BPF generate, and artifact upload: see `.github/workflows/golangci.yaml`
- Exact test commands: see `.pipelines/templates/run-unit-tests.yaml`
- Makefile targets: `test-all`, `test-main`, `bpf-lib` in `Makefile`

#### Verification
- [ ] Linux unit tests pass on a PR that passes ADO today
- [ ] Windows unit tests pass (or known-failing packages are explicitly excluded with a TODO)
- [ ] Coverage artifact is generated and visible in the Actions run summary
- [ ] GHA check is added to branch protection rules
- [ ] ADO unit test stage still runs in parallel for at least 5 PRs before being removed

#### Notes

- Keep this workflow path-filtered to code-affecting areas only if runtime requires it, but
  ensure generated-code checks (`crdgen`, `baseimages`) still run where needed.
- Maintain race-enabled test execution for parity with current quality bar.

---

### Phase 2 — Binary Build Validation

**Value:** Low-medium. Validates cross-platform compilation. Artifacts are not distributed.  
**Source ADO pipeline:** `.pipelines/build/binary.steps.yaml`, `.pipelines/templates/setup-environment.yaml`  
**New file:** `.github/workflows/build.yaml`

> **Note:** This phase can be deferred or run in parallel with Phase 3. It is useful as a
> canary for cross-platform build breakage but is not a PR-blocking requirement.

#### Jobs (matrix)

| Target | Runner | Extra Steps |
|---|---|---|
| `linux/amd64` | `ubuntu-latest` | Install `llvm`, `clang`, `libbpf-dev`, `nftables`, `iproute2` |
| `linux/arm64` | `ubuntu-latest` | Install arm64 cross-compile deps (`gcc-aarch64-linux-gnu`, etc.) |
| `windows/amd64` | `windows-latest` | Windows-compatible targets only |

- Run `make bpf-lib all-binaries` (Linux) / equivalent `go build` targets (Windows)
- **Do not publish artifacts.** Compilation check only.

#### Triggers
```yaml
on:
  pull_request:
    branches: [master, "release/*"]
  merge_group:
    types: [checks_requested]
  workflow_dispatch:
```

Trigger intent:
- Keep build validation mostly on `merge_group` if PR runtime pressure is high.
- Allow `pull_request` runs to remain non-blocking canaries.

#### Verification
- [ ] All three matrix targets build successfully
- [ ] No pre-existing compilation failures are masked

#### Notes

- Keep this phase non-blocking unless recurring compile regressions justify making it required.

---

### Phase 3 — Test Image Builds to GHCR

**Value:** High as a prerequisite for AKS E2E (Phase 4+). Not for production use.  
**Source ADO pipeline:** `.pipelines/containers/container-template.yaml`, `.pipelines/build/images.jobs.yaml`  
**New files:** `.github/workflows/test-images.yaml`, `.github/workflows/ghcr-cleanup.yaml`

#### Scope

Build **test images only** for use by AKS E2E jobs. Do not build or push production images.

Images to build:
- `azure-cns`
- `azure-cni` (linux/amd64, linux/arm64)
- `azure-ipam`

**Do not build:** npm, nginx, cilium-log-collector, ipv6-hp-bpf (not needed for E2E gates).

#### Tag strategy

| Event | Tag |
|---|---|
| Pull request | `pr-<PR#>-<short-sha>` |
| Push to `master` | `sha-<short-sha>`, `latest-test` |

Registry: `ghcr.io/azure/azure-container-networking/<image>:<tag>`

Authentication: use `GITHUB_TOKEN` — no Azure credentials needed for GHCR.

#### Actions to use
- `docker/setup-buildx-action`
- `docker/setup-qemu-action` (for arm64 cross-build)
- `docker/login-action` with `registry: ghcr.io`
- `docker/build-push-action` with `platforms: linux/amd64,linux/arm64`

#### GHCR Cleanup workflow (`ghcr-cleanup.yaml`)

- Trigger: weekly schedule + `workflow_dispatch`
- Delete all `pr-*` tagged versions older than 14 days
- Use `actions/delete-package-versions` or GitHub REST API via `gh` CLI
- Optionally keep the 5 most recent `sha-*` tags on master
- Apply package-scoped deletion rules so only test tags are removed.
- Add a dry-run mode for manual invocation before enabling scheduled deletion.

#### Triggers for `test-images.yaml`
```yaml
on:
  pull_request:
    branches: [master, "release/*"]
  push:
    branches: [master]
  workflow_dispatch:
```

#### Verification
- [ ] Test images appear in GHCR with correct tags after a PR
- [ ] Cleanup workflow removes stale `pr-*` tags when run manually
- [ ] A downstream E2E job (Phase 4) can pull the image successfully

#### Notes

- Add image labels/annotations (commit SHA, PR number, source workflow run URL) for traceability.

---

### Phase 4 — AKS E2E: CNS/CNI Singletenancy

**Value:** High. Core correctness signal for CNS and CNI changes.  
**Source ADO pipeline:** `.pipelines/singletenancy/aks/e2e.stages.yaml`, `.pipelines/templates/create-cluster.yaml`  
**New file:** `.github/workflows/e2e-aks.yaml`

#### Prerequisites

- OIDC federated identity **or** service principal configured in GitHub repo secrets:
  - `AZURE_CLIENT_ID`
  - `AZURE_TENANT_ID`
  - `AZURE_SUBSCRIPTION_ID`
- Azure resource quota available in the target region
- Phase 3 test images available in GHCR
- Cost guardrails defined (max nightly clusters, allowed VM sizes, max runtime)

#### Job structure

1. **Create cluster**: `az aks create` with appropriate node SKU and CNI config
2. **Deploy**: pull test image from GHCR (`ghcr.io/...:<tag>`) and deploy CNS/CNI via kubectl/helm
3. **Run E2E**: execute e2e test suite (see `make test-k8se2e-only` and `hack/scripts/`)
4. **Cleanup**: `az group delete --name <rg> --yes --no-wait` in an `always:` post-step

Operational guardrails:
- Use deterministic resource group naming (`acn-gha-<workflow>-<runid>`) and tag all resources
  with owner, repo, workflow, and expiration metadata.
- Use GHA `concurrency` to cap parallel AKS runs (avoid quota exhaustion).
- Add a safety sweeper job (daily) to delete orphaned resource groups older than TTL.
- Always upload cluster diagnostics on failure (kubectl describe/logs, az aks diagnostics).

Start with **one scenario**: standard AKS singletenancy. Expand to additional scenarios
(swift, overlay, etc.) only after the first is stable.

#### Triggers

**Not on every PR** (cost). Gate on:
```yaml
on:
  schedule:
    - cron: "0 3 * * *"   # nightly 3am UTC
  workflow_dispatch:
    inputs:
      scenario:
        description: "E2E scenario to run"
        default: "singletenancy-aks"
```

For PR validation: consider a label-triggered approach (`e2e-required` label) rather than
running on all PRs.

#### Scenario expansion order (after first is stable)

1. `singletenancy/aks` — standard AKS ✓ (start here)
2. `singletenancy/aks-swift` — Swift overlay
3. `singletenancy/azure-cni-overlay` — Azure CNI overlay
4. `singletenancy/azure-cni-overlay-stateless` — Stateless overlay
5. Cilium variants (Phase 5)

#### Verification
- [ ] Cluster creates and deletes cleanly (no leaked resource groups)
- [ ] E2E tests produce a pass/fail result that matches ADO pipeline
- [ ] Cleanup step runs even on job failure
- [ ] ≥ 5 consecutive clean nightly runs before retiring ADO equivalent

#### Notes

- Keep Phase 4 initially non-blocking for PRs; promote to required only after stability is proven
  and cost envelope is acceptable.

---

### Phase 5 — Cilium E2E on AKS

**Value:** High for Cilium-specific correctness.  
**Source ADO pipelines:** `.pipelines/singletenancy/cilium/`, `.pipelines/cni/cilium/`  
**New file:** `.github/workflows/e2e-cilium.yaml` (or extend `e2e-aks.yaml` with a matrix)

#### Scenarios (expand iteratively)

1. `cilium-overlay` (start here — most common)
2. `cilium-ebpf`
3. `cilium-overlay-withhubble`
4. `cilium-dualstack-overlay`
5. `cilium-nodesubnet`

#### Reference templates

- `.pipelines/templates/cilium-tests.yaml` — test sequence (status, restarts, scaling, connectivity)
- `.pipelines/templates/cilium-connectivity-tests.yaml` — connectivity validation steps
- `.pipelines/templates/cilium-cli.yaml` — Cilium CLI installation

#### Triggers

Same as Phase 4: nightly + manual dispatch. Add scenario as a matrix dimension.

#### Verification
- [ ] Each cilium variant passes at least once in GHA
- [ ] Cilium CLI version pinning matches ADO
- [ ] ≥ 5 consecutive clean nightly runs per variant before retiring ADO equivalent

#### Notes

- Migrate one cilium variant at a time into required-check posture to avoid broad regressions.

---

### Phase 6 — SwiftV2 / Multitenancy Long-Running (Decision Point)

**Source ADO pipeline:** `.pipelines/swiftv2-long-running/pipeline.yaml`

This pipeline runs every 3 hours and creates a full multi-cluster VNet topology (multiple AKS
clusters, customer VNets, peerings, private endpoints, NSGs). It is the most infrastructure-
intensive pipeline in the repository.

**Recommendation: keep in ADO** unless there is a specific operational reason to move it.
GHA would require:
- Persistent state management across runs (GHA Environments or external state store)
- Service principal with broad Azure networking permissions
- Equivalent YAML for 10+ stage infrastructure setup

Revisit this decision after Phases 4 and 5 are stable and operational experience with
Azure credentials in GHA has been established.

---

## NPM — Frozen

NPM is deprecated. The following applies immediately:

| Item | Action |
|---|---|
| `.pipelines/npm/` ADO pipelines | Freeze — no new changes, leave running |
| `.github/workflows/cyclonus-netpol-test.yaml` | Freeze — no new development |
| `.github/workflows/cyclonus-netpol-extended-nightly-test.yaml` | Freeze — no new development |
| Any new NPM test work | Not in scope |

---

## Out of Scope (Permanently)

| Item | Reason |
|---|---|
| Production image publishing | Offloaded to partner service |
| Binary artifact distribution | Not distributed from this repo |
| `cni/ado-automation/var-pipeline.yaml` | ADO REST API — cannot migrate |

---

## Summary Table

| Phase | New GHA File | ADO Source | Infra Required | Priority |
|---|---|---|---|---|
| 0 | Reusable/audit workflows | N/A | None | **Highest** |
| 1 | `unit-tests.yaml` | `run-unit-tests.yaml` | None | **Highest** |
| 2 | `build.yaml` | `binary.steps.yaml` | None | Low |
| 3 | `test-images.yaml`, `ghcr-cleanup.yaml` | `images.jobs.yaml` | None (GHCR) | High (blocks Ph4) |
| 4 | `e2e-aks.yaml` | `singletenancy/aks/` | AKS + Azure creds | High |
| 5 | `e2e-cilium.yaml` | `singletenancy/cilium/` | AKS + Azure creds | Medium |
| 6 | TBD or stay ADO | `swiftv2-long-running/` | Full VNet stack | Defer |

## ADO to GHA Coverage Mapping (Living Section)

Use this section to track parity status during migration.

| Signal | Current Source | Target GHA Workflow | Status |
|---|---|---|---|
| Lint | `.github/workflows/golangci.yaml` | `golangci.yaml` | complete |
| Dockerfile regeneration | `.github/workflows/baseimages.yaml` | `baseimages.yaml` | complete |
| CRD regeneration | `.github/workflows/crdgen.yaml` | `crdgen.yaml` | complete |
| Unit tests + coverage | `.pipelines/templates/run-unit-tests.yaml` | `unit-tests.yaml` | planned |
| Build validation | `.pipelines/build/binary.steps.yaml` | `build.yaml` | planned |
| Test image build/publish | `.pipelines/build/images.jobs.yaml` | `test-images.yaml` | planned |
| AKS singletenancy E2E | `.pipelines/singletenancy/aks/e2e.stages.yaml` | `e2e-aks.yaml` | planned |
| Cilium E2E variants | `.pipelines/singletenancy/cilium/*` | `e2e-cilium.yaml` | planned |
| SwiftV2 long-running | `.pipelines/swiftv2-long-running/pipeline.yaml` | N/A or future | deferred |
| NPM pipelines | `.pipelines/npm/*` | N/A | frozen |
| OneBranch run pipeline | `.pipelines/run-pipeline.yaml` | N/A | retire with prod offload |

## Open Decisions (Resolve Before Phase 4)

- Azure auth model for AKS E2E: OIDC federation preferred; identify owning team and setup timeline.
- AKS region and quota budget for nightly runs: define default region(s), VM SKUs, and max concurrent runs.
- PR-on-demand E2E trigger model: label-triggered, comment-triggered, or manual-dispatch only.
- GHCR retention policy: exact TTL for `pr-*` tags and keep-count for `sha-*` tags.
