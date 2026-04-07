# SwiftV2 Long-Running Zone-Aware Pod Tests

This document covers the **long-running zone-aware pod tests** — rotating pods and DaemonSet always-on pods that run persistently on the same AKS cluster used by the SwiftV2 long-running pipeline.

These tests are **integrated into the main pipeline** (`pipeline.yaml`). The `longrunningRegions` parameter maps each region to its availability zones. Zone node pool creation and per-zone test stages run alongside the existing datapath tests.

### Adding a New Region or Subscription

Edit the `scenarios` parameter in `pipeline.yaml`. Each scenario entry specifies a subscription, region, unique label, accelnet flag, and long-running zones:

```yaml
scenarios:
  - subscriptionId: "37deca37-..."
    location: eastus2euap
    label: "eastus2euap"
    enableAccelnet: false
    longRunningPodZones: ["1", "2", "3", "4"]
  - subscriptionId: "9b8218f9-..."
    location: eastus2euap
    label: "eastus2euap_baseline"
    enableAccelnet: false
    longRunningPodZones: ["1", "2", "3", "4"]
```

The `label` must be unique across scenarios — it is used to generate unique stage names. Two scenarios can share the same `location` (e.g., to compare subscriptions) as long as their labels differ.

---

## Long-Running Zone-Aware Pod Tests

### Design

The pipeline runs tests across **all 4 availability zones** in `eastus2euap` every hour. Each zone has **1 high-NIC node** (`Standard_D16s_v3`, 7 NIC slots) running:
- **6 rotating Deployments** — each with 1 pod, managed by the pipeline, cycled every 6 runs (6 hours)
- **1 DaemonSet always-on pod** — self-healing via Kubernetes, runs indefinitely

All pods use the same VNet/Subnet (`cx_vnet_v1/lr`). All 4 zones run **in parallel**.

```
┌──────────────────────────────────────────────────────────────────┐
│                     AKS Cluster (aks-1)                          │
│                     Region: eastus2euap                          │
│                                                                  │
│  ┌──────────────────────────────────────────────────────────────┐│
│  │ Zone 1 (npz1 — 1 node, longrunning-zone-pool=true)              ││
│  │  ┌────────────────────────────────────────────────────────┐  ││
│  │  │ pod-rotating-0..5  (6 Deployments × 1 pod, pipeline-mgd) │  ││
│  │  │ ds-alwayson-z1-*   (1 DaemonSet pod, self-healing)     │  ││
│  │  │ All on cx_vnet_v1/lr  — 7/7 NIC slots used            │  ││
│  │  └────────────────────────────────────────────────────────┘  ││
│  └──────────────────────────────────────────────────────────────┘│
│  ┌──────────────────────────────────────────────────────────────┐│
│  │ Zone 2 (npz2)                    ...same layout...          ││
│  └──────────────────────────────────────────────────────────────┘│
│  ┌──────────────────────────────────────────────────────────────┐│
│  │ Zone 3 (npz3)                    ...same layout...          ││
│  └──────────────────────────────────────────────────────────────┘│
│  ┌──────────────────────────────────────────────────────────────┐│
│  │ Zone 4 (npz4)                    ...same layout...          ││
│  └──────────────────────────────────────────────────────────────┘│
│                                                                  │
│  Total: 4 nodes (1 per zone), 28 pods (7 per zone)              │
└──────────────────────────────────────────────────────────────────┘
```

### Rotating Deployments (6 per zone, pipeline-managed)

**Purpose**: Simulate pod churn for NIC allocation/deallocation testing. Provides at least 1 fresh NIC allocation/deallocation cycle per hour.

Each "rotating slot" is a **Kubernetes Deployment** (1 replica). The deployment name is `pod-rotating-<N>` and the actual pod name is auto-generated by Kubernetes.

| Property | Value |
|----------|-------|
| Node label | `longrunning-zone-pool=true` + `topology.kubernetes.io/zone=<location>-<N>` |
| Deployment count | 6 (uses 6 of 7 NIC slots) |
| Deployment lifetime | 6 hours max (`rotatingPodMaxAge = 6 * time.Hour`) |
| Rotation guarantee | At least 1 deployment deleted + recreated every run |
| VNet/Subnet | `cx_vnet_v1/lr` |
| Deployment names | `pod-rotating-0` through `pod-rotating-5` |

**Rotation algorithm** (runs every hour):
1. Scan all 6 deployment slots — check each deployment's `acn-test/created-at` annotation on its pod template
2. Delete any deployments older than 6 hours (waits for MTPNC cleanup after each deletion)
3. If no deployments were deleted (none expired yet), delete the **oldest** deployment to guarantee at least 1 rotation per run
4. Recreate deployments for all empty slots with a fresh `acn-test/created-at` timestamp
5. Verify all 6 deployments are Ready

**Steady-state behavior**: After 6 hours (6 runs), all rotating NIC slots will have been recycled at least once. Roughly 1-2 deployments rotate per run.

### DaemonSet Always-On Pod (1 per zone, self-healing)

**Purpose**: Provide a stable long-living pod per zone for live migration and other Azure team testing. Auto-recovers without pipeline intervention.

| Property | Value |
|----------|-------|
| Node selector | `longrunning-zone-pool=true` + `topology.kubernetes.io/zone=<location>-<N>` |
| Pod count | 1 (uses remaining NIC slot) |
| Pod lifetime | Indefinite (Kubernetes auto-restarts if it crashes) |
| VNet/Subnet | `cx_vnet_v1/lr` |
| DaemonSet name | `ds-alwayson-z<N>` (e.g., `ds-alwayson-z1`) |

**Why a DaemonSet?** If the pod crashes or the node reboots between pipeline runs, Kubernetes automatically restarts it — no waiting for the next pipeline run. The pipeline's always-on test simply verifies the DaemonSet pod is healthy.

**Health check** (build tag `longrunning_alwayson_test`, pipeline job `AlwaysOnPods_Z<N>`, runs every hour):
1. Ensure namespace, PodNetwork, and PodNetworkInstance exist (idempotent, reused across runs)
2. Validate `ZONE` and `LOCATION` are set — fails fast if `GetZoneLabel()` returns empty
3. Ensure DaemonSet exists (create if missing)
4. Wait for DaemonSet pod to be Ready (`waitForDaemonSetReady`, returns `errDaemonSetNotReady` on timeout)
5. Verify DaemonSet pod is in Running phase

> **Metrics**: Metrics emission is currently **disabled** (commented-out TODO). Will be re-enabled once Networking-Aquarius supports a dedicated long-running metric name.

### Connectivity Test (per zone)

After both pod management jobs succeed in a zone, a bidirectional TCP datapath test runs (pipeline job `LongRunningConnectivityTest_Z<N>`):

| Test name | Source → Destination |
|-----------|---------------------|
| `LR-Rotating-To-AlwaysOn-Z<N>` | rotating deployment pod → DaemonSet pod |
| `LR-AlwaysOn-To-Rotating-Z<N>` | DaemonSet pod → rotating deployment pod |

Both pod names are resolved at runtime — the rotating deployment pod via `GetDeploymentPodName()` and the DaemonSet pod via `GetDaemonSetPodName()`. Both pods are on `cx_vnet_v1/lr`. Tests use TCP via delegated subnet (eth1) with netcat on port 8080.

---

## Zone Node Pool Setup

### How It Works

The `ensure_zone_nodepools.sh` script is an **idempotent** operation that runs after infrastructure verification in the main pipeline. It:

1. Checks if each zone's node pool (`npz1`, `npz2`, `npz3`, `npz4`) already exists
2. Creates missing node pools with `--zones <N>` to pin the node to a specific zone
3. Waits for all nodes to be Ready
4. Labels nodes with `nic-capacity=high-nic`, `workload-type=swiftv2-linux`, `longrunning-zone-pool=true`
5. Confirms node zones via `topology.kubernetes.io/zone` label

### Node Pool Configuration

| Pool | Zones | Node Count | VM SKU | NIC Slots | Labels |
|------|-------|-----------|--------|-----------|--------|
| `npz1` | 1 | 1 | Standard_D16s_v3 | 7 | `longrunning-zone-pool=true`, `nic-capacity=high-nic`, `workload-type=swiftv2-linux` |
| `npz2` | 2 | 1 | Standard_D16s_v3 | 7 | same |
| `npz3` | 3 | 1 | Standard_D16s_v3 | 7 | same |
| `npz4` | 4 | 1 | Standard_D16s_v3 | 7 | same |

### AKS Zone Labels

AKS automatically sets `topology.kubernetes.io/zone` on every node. For eastus2euap:
```
topology.kubernetes.io/zone=eastus2euap-1
topology.kubernetes.io/zone=eastus2euap-2
topology.kubernetes.io/zone=eastus2euap-3
topology.kubernetes.io/zone=eastus2euap-4
```

The Go tests use this label combined with `longrunning-zone-pool` to select the correct node:
```go
// Example label selector for the zone 3 node:
"longrunning-zone-pool=true,topology.kubernetes.io/zone=eastus2euap-3"
```

---

## Pipeline Structure

The long-running tests are part of the main long-running pipeline:

```
pipeline.yaml
  └── long-running-pipeline-template.yaml
        │
        ├── AKSClusterAndNetworking_<loc> (per location, idempotent)
        │   ├── VerifyInfrastructure  ← smart check, skips setup if all exists
        │   ├── EnsureNodeLabels      ← re-applies labels every run (survives node replacements)
        │   ├── CreateResourceGroup   ← conditional on !infraExists
        │   ├── CreateCluster          ...
        │   ├── NetworkingAndStorage    ...
        │   └── DeployLinuxBYON        ...
        │
        ├── LongRunningPodTests_Z<N>_<loc>  (per zone, parallel, NOT lease-gated)
        │   ├── (depends on AKSClusterAndNetworking_<loc>)
        │   ├── Zone 1 stage ──┬── EnsureNodePool_Z1      ← first job: idempotent node pool setup
        │   │                  ├── SetupKubeconfig         ← depends on EnsureNodePool_Z1
        │   │                  ├── BuildMetricsBinary
        │   │                  ├── RotatingPods_Z1              ──┐ depends on Setup+Build
        │   │                  ├── AlwaysOnPods_Z1               ──┤ (parallel)
        │   │                  └── LongRunningConnectivityTest_Z1 ──┘ depends on both
        │   ├── Zone 2 stage ── same structure
        │   ├── Zone 3 stage ── same structure
        │   └── Zone 4 stage ── same structure
        │
        ├── AcquireLease_<loc> (ConfigMap-based, gates datapath tests only)
        │   └── (depends on AKSClusterAndNetworking_<loc>)
        │
        ├── DataPathTests_<workload>_<loc> (per location × workload type, parallel, lease-gated)
        │   ├── (depends on AKSClusterAndNetworking_<loc> + AcquireLease_<loc>)
        │   └── swiftv2-linux + swiftv2-linux-byon run in parallel
        │
        └── ReleaseLease_<loc> (always runs, depends on all DataPathTests)
```

All 4 zones run as **separate stages in parallel**, starting immediately after infrastructure setup — in parallel with `AcquireLease` and `DataPathTests`.
Within each zone stage, `EnsureNodePool_Z<N>` runs first; `SetupKubeconfig` and `BuildMetricsBinary` follow; `RotatingPods` and `AlwaysOnPods` run in parallel; `LongRunningConnectivityTest` waits for both.

**Note**: Long-running pod tests are **not gated by the lease** — each zone stage depends only on `AKSClusterAndNetworking_<loc>`. They start immediately after infrastructure is ready, without waiting for the lease or datapath tests.

### Idempotent Infrastructure Setup

The `VerifyInfrastructure` job checks the "final products" (cluster health, VNet existence, peering state, storage accounts) before running any setup scripts. If everything exists, setup is **skipped** — saving 30+ minutes on each run.

### Lease Mechanism

A Kubernetes ConfigMap (`acn-pipeline-lease`) on `aks-1` acts as a distributed lock. The lease **only gates datapath tests** (not long-running pod tests). Each pipeline run acquires the lease before running datapath tests and releases it afterward. If a previous run still holds the lease, the new run waits (up to 30 minutes) or fails gracefully.

---

## Resource Naming

All resource names include a zone suffix (`-z<N>`) to avoid collision across zones:

```
ZONE=3, BUILD_ID=sv2-long-run-eastus2euap

Rotating:
  Namespace:          ns-rotating-z3-sv2-long-run-eastus2euap
  PodNetwork:         pn-rotating-z3-sv2-long-run-eastus2euap   (VNet: cx_vnet_v1/lr)
  PodNetworkInstance: pni-rotating-z3-sv2-long-run-eastus2euap
  Deployments:        pod-rotating-0 .. pod-rotating-5          (pod name auto-generated by K8s)

Always-on (DaemonSet):
  Namespace:          ns-alwayson-z3-sv2-long-run-eastus2euap
  PodNetwork:         pn-alwayson-z3-sv2-long-run-eastus2euap   (VNet: cx_vnet_v1/lr)
  PodNetworkInstance: pni-alwayson-z3-sv2-long-run-eastus2euap
  DaemonSet:          ds-alwayson-z3
  Pod:                ds-alwayson-z3-<hash>  (auto-generated by K8s)
```

---

## Environment Variables

| Variable | Description | Set By |
|----------|-------------|--------|
| `RG` | Azure resource group name | Pipeline |
| `BUILD_ID` | Stable ID for resource naming (= RG name for long-running tests; = RG + workload suffix for datapath tests) | Pipeline |
| `ZONE` | Availability zone number ("1", "2", "3", "4") | Pipeline (long-running only) |
| `LOCATION` | Azure region (e.g., "eastus2euap") | Pipeline (long-running only) |
| `WORKLOAD_TYPE` | Node workload filter ("swiftv2-linux") | Pipeline |
| `KUBECONFIG_DIR` | Directory containing kubeconfig files | Pipeline |

---

## Idempotency

All operations are designed to be safe to re-run. PodNetworks, PodNetworkInstances, namespaces, and the DaemonSet are **created once and reused** across all long-running runs — they are never deleted by the long-running pipeline. Only rotating pods are cycled. These resources are separate from the main pipeline's resources (different naming prefixes).

| Operation | Idempotency |
|-----------|-------------|
| Zone node pool creation | `az aks nodepool show` checks existence first (inside `EnsureNodePool_Z<N>` job) |
| Node labeling | `--overwrite` flag; removes labels from cordoned nodes, applies to schedulable nodepool1/nplinux nodes |
| PN/PNI/Namespace creation | Created once, reused. Go client `Get` checks existence before creating |
| DaemonSet creation | Created once, runs indefinitely. `daemonSetExists()` checks before creating |
| DaemonSet pod recovery | Kubernetes auto-restarts; pipeline only verifies readiness |
| Deployment rotation | Only rotating deployments are deleted/recreated based on `acn-test/created-at` annotation age |

---

## File Structure

```
.pipelines/swiftv2-long-running/
├── pipeline.yaml                              # Main pipeline entry point (every hour)
├── LONGRUNNING-TESTS.md                       # This file
├── template/
│   ├── long-running-pipeline-template.yaml    # Infra setup + datapath tests + long-running tests
│   ├── infrastructure-setup-stage.yaml        # Per-location infra verify + conditional setup
│   ├── datapath-tests-stage.yaml              # Per-workload datapath test stage
│   └── long-running-pod-tests-stage.yaml      # Per-zone: EnsureNodePool + rotating + always-on + connectivity
└── scripts/
    ├── verify_infrastructure.sh               # Smart infra check (skip setup if exists)
    ├── ensure_zone_nodepools.sh               # Idempotent per-zone node pool creation
    ├── acquire_pipeline_lease.sh              # ConfigMap lease acquisition
    └── release_pipeline_lease.sh              # ConfigMap lease release

test/integration/swiftv2/longRunningCluster/
├── datapath_longrunning_shared.go             # Shared constants/utils for long-running tests (zone-aware)
├── datapath_longrunning_rotating_test.go      # Rotating deployments (tag: longrunning_rotating_test)
├── datapath_longrunning_alwayson_test.go      # DaemonSet always-on (tag: longrunning_alwayson_test)
├── datapath_longrunning_connectivity_test.go  # Long-running connectivity (tag: longrunning_connectivity_test)
├── datapath.go                                # DaemonSetData, DeploymentData, CreateDaemonSet(), CreateDeployment()
└── k8s_client.go                              # waitForDaemonSetReady(), getDeploymentPodName(), etc.

test/integration/manifests/swiftv2/long-running-cluster/
└── daemonset.yaml                             # DaemonSet manifest template (always-on)
```

---

## Build Tags

Each test file uses a unique Go build tag so tests can be run independently:

| Build Tag | File | Pipeline Job |
|-----------|------|-------------|
| `longrunning_rotating_test` | `datapath_longrunning_rotating_test.go` | `RotatingPods_Z{N}` |
| `longrunning_alwayson_test` | `datapath_longrunning_alwayson_test.go` | `AlwaysOnPods_Z{N}` |
| `longrunning_connectivity_test` | `datapath_longrunning_connectivity_test.go` | `LongRunningConnectivityTest_Z{N}` |
