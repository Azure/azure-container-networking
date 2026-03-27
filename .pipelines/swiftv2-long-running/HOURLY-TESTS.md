# SwiftV2 Hourly Zone-Aware Pod Tests

This document covers the **hourly zone-aware pod tests** — rotating pods and DaemonSet always-on pods that run persistently on the same AKS cluster used by the SwiftV2 long-running pipeline.

These tests are **integrated into the main pipeline** (`pipeline.yaml`). The `hourlyRegions` parameter maps each region to its availability zones. Zone node pool creation and per-zone test stages run alongside the existing datapath tests.

### Adding a New Region

Edit the `hourlyRegions` parameter in `pipeline.yaml`:

```yaml
hourlyRegions:
  - location: eastus2euap
    zones: ["1", "2", "3", "4"]
  - location: centraluseuap        # example: add a new region
    zones: ["1", "2", "3"]         # only 3 zones here
```

Each region gets its own `EnsureZoneNodePools` setup stage followed by parallel per-zone test stages.

---

## Hourly Zone-Aware Pod Tests

### Design

The pipeline runs tests across **all 4 availability zones** in `eastus2euap` every hour. Each zone has **1 high-NIC node** (`Standard_D16s_v3`, 7 NIC slots) running:
- **6 rotating pods** — managed by the pipeline, cycled every 6 runs (6 hours)
- **1 DaemonSet always-on pod** — self-healing via Kubernetes, runs indefinitely

All pods use the same VNet/Subnet (`cx_vnet_v1/s1`). All 4 zones run **in parallel**.

```
┌──────────────────────────────────────────────────────────────────┐
│                     AKS Cluster (aks-1)                          │
│                     Region: eastus2euap                          │
│                                                                  │
│  ┌──────────────────────────────────────────────────────────────┐│
│  │ Zone 1 (npz1 — 1 node, hourly-zone-pool=true)              ││
│  │  ┌────────────────────────────────────────────────────────┐  ││
│  │  │ pod-rotating-0..5  (6 pods, pipeline-managed)          │  ││
│  │  │ ds-alwayson-z1-*   (1 DaemonSet pod, self-healing)     │  ││
│  │  │ All on cx_vnet_v1/s1  — 7/7 NIC slots used            │  ││
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

### Rotating Pods (6 per zone, pipeline-managed)

**Purpose**: Simulate pod churn for NIC allocation/deallocation testing. Provides at least 1 fresh pod per hour.

| Property | Value |
|----------|-------|
| Node label | `hourly-zone-pool=true` + `topology.kubernetes.io/zone=<location>-<N>` |
| Pod count | 6 (uses 6 of 7 NIC slots) |
| Pod lifetime | 6 runs max (6 hours at hourly schedule) |
| Rotation guarantee | At least 1 pod deleted + recreated every run |
| VNet/Subnet | `cx_vnet_v1/s1` |
| Pod names | `pod-rotating-0` through `pod-rotating-5` |

**Rotation algorithm** (runs every hour):
1. Scan all 6 pod slots and check each pod's `acn-test/created-at` annotation
2. Delete any pods older than 6 runs (6 hours)
3. If no pods were deleted (none expired yet), delete the **oldest** pod to guarantee at least 1 rotation per run
4. Recreate pods for all empty slots
5. Verify all 6 pods are in Running state

**Steady-state behavior**: After 6 hours (6 runs), all rotating NIC slots will have been recycled at least once. Roughly 1-2 pods rotate per run.

### DaemonSet Always-On Pod (1 per zone, self-healing)

**Purpose**: Provide a stable long-living pod per zone for live migration and other Azure team testing. Auto-recovers without pipeline intervention.

| Property | Value |
|----------|-------|
| Node selector | `hourly-zone-pool=true` + `topology.kubernetes.io/zone=<location>-<N>` |
| Pod count | 1 (uses remaining NIC slot) |
| Pod lifetime | Indefinite (Kubernetes auto-restarts if it crashes) |
| VNet/Subnet | `cx_vnet_v1/s1` |
| DaemonSet name | `ds-alwayson-z<N>` (e.g., `ds-alwayson-z1`) |

**Why a DaemonSet?** If the pod crashes or the node reboots between pipeline runs, Kubernetes automatically restarts it — no waiting for the next pipeline run. The pipeline's always-on test simply verifies the DaemonSet pod is healthy.

**Health check** (pipeline `hourly_alwayson_test`, runs every hour):
1. Ensure PodNetwork, PodNetworkInstance, and namespace exist (idempotent)
2. Ensure DaemonSet exists (create if missing)
3. Verify DaemonSet pod is Running
4. Report status (future: alerting if not running)

### Connectivity Test (per zone)

After both pod management jobs succeed in a zone, a bidirectional TCP datapath test runs:

| Test | Source → Destination |
|------|---------------------|
| Rotating-To-AlwaysOn | `pod-rotating-0` → DaemonSet pod |
| AlwaysOn-To-Rotating | DaemonSet pod → `pod-rotating-0` |

Both use `cx_vnet_v1/s1`. Tests use TCP via delegated subnet (eth1) with netcat on port 8080.

---

## Zone Node Pool Setup

### How It Works

The `ensure_zone_nodepools.sh` script is an **idempotent** operation that runs after infrastructure verification in the main pipeline. It:

1. Checks if each zone's node pool (`npz1`, `npz2`, `npz3`, `npz4`) already exists
2. Creates missing node pools with `--zones <N>` to pin the node to a specific zone
3. Waits for all nodes to be Ready
4. Labels nodes with `nic-capacity=high-nic`, `workload-type=swiftv2-linux`, `hourly-zone-pool=true`
5. Confirms node zones via `topology.kubernetes.io/zone` label

### Node Pool Configuration

| Pool | Zones | Node Count | VM SKU | NIC Slots | Labels |
|------|-------|-----------|--------|-----------|--------|
| `npz1` | 1 | 1 | Standard_D16s_v3 | 7 | `hourly-zone-pool=true`, `nic-capacity=high-nic`, `workload-type=swiftv2-linux` |
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

The Go tests use this label combined with `hourly-zone-pool` to select the correct node:
```go
// Example label selector for the zone 3 node:
"hourly-zone-pool=true,topology.kubernetes.io/zone=eastus2euap-3"
```

---

## Pipeline Structure

The hourly tests are part of the main long-running pipeline:

```
pipeline.yaml
  └── long-running-pipeline-template.yaml
        │
        ├── AKSClusterAndNetworking (per location, idempotent)
        │   ├── VerifyInfrastructure  ← smart check, skips setup if all exists
        │   ├── CreateResourceGroup   ← conditional on !infraExists
        │   ├── CreateCluster          ...
        │   ├── NetworkingAndStorage    ...
        │   └── DeployLinuxBYON        ...
        │
        ├── EnsureZoneNodePools (per hourlyRegion, depends on infra)
        │   └── ensure_zone_nodepools.sh
        │
        ├── AcquireLease (ConfigMap-based, gates datapath tests only)
        │
        ├── DataPathTests (per location × workload type, parallel, lease-gated)
        │   └── swiftv2-linux + swiftv2-linux-byon run in parallel
        │
        ├── HourlyPodTests (per zone, parallel, NOT lease-gated)
        │   ├── Zone 1 ──┬── SetupKubeconfig
        │   │            ├── BuildMetricsBinary
        │   │            ├── RotatingPods_Z1     ──┐
        │   │            ├── AlwaysOnPods_Z1     ──┤
        │   │            └── ConnectivityTest_Z1 ──┘
        │   ├── Zone 2 ── same structure
        │   ├── Zone 3 ── same structure
        │   └── Zone 4 ── same structure
        │
        └── ReleaseLease (always runs, gates on datapath tests only)
```

All 4 zones run as **separate stages in parallel** after the `EnsureZoneNodePools` setup stage.
Within each zone, `RotatingPods` and `AlwaysOnPods` run in parallel; `ConnectivityTest` waits for both.

**Note**: Hourly pod tests are **not gated by the lease** — they only depend on zone node pool setup. This means they start immediately after infrastructure is ready, without waiting for the lease or datapath tests.

### Idempotent Infrastructure Setup

The `VerifyInfrastructure` job checks the "final products" (cluster health, VNet existence, peering state, storage accounts) before running any setup scripts. If everything exists, setup is **skipped** — saving 30+ minutes on each run.

### Lease Mechanism

A Kubernetes ConfigMap (`acn-pipeline-lease`) on `aks-1` acts as a distributed lock. The lease **only gates datapath tests** (not hourly pod tests). Each pipeline run acquires the lease before running datapath tests and releases it afterward. If a previous run still holds the lease, the new run waits (up to 30 minutes) or fails gracefully.

---

## Resource Naming

All resource names include a zone suffix (`-z<N>`) to avoid collision across zones:

```
ZONE=3, BUILD_ID=sv2-long-run-eastus2euap

Rotating:
  Namespace:          ns-rotating-z3-sv2-long-run-eastus2euap
  PodNetwork:         pn-rotating-z3-sv2-long-run-eastus2euap   (VNet: cx_vnet_v1/s1)
  PodNetworkInstance: pni-rotating-z3-sv2-long-run-eastus2euap
  Pods:               pod-rotating-0 .. pod-rotating-5

Always-on (DaemonSet):
  Namespace:          ns-alwayson-z3-sv2-long-run-eastus2euap
  PodNetwork:         pn-alwayson-z3-sv2-long-run-eastus2euap   (VNet: cx_vnet_v1/s1)
  PodNetworkInstance: pni-alwayson-z3-sv2-long-run-eastus2euap
  DaemonSet:          ds-alwayson-z3
  Pod:                ds-alwayson-z3-<hash>  (auto-generated by K8s)
```

---

## Environment Variables

| Variable | Description | Set By |
|----------|-------------|--------|
| `RG` | Azure resource group name | Pipeline |
| `BUILD_ID` | Stable ID for resource naming (= RG name for hourly tests; = RG + workload suffix for datapath tests) | Pipeline |
| `ZONE` | Availability zone number ("1", "2", "3", "4") | Pipeline (hourly only) |
| `LOCATION` | Azure region (e.g., "eastus2euap") | Pipeline (hourly only) |
| `WORKLOAD_TYPE` | Node workload filter ("swiftv2-linux") | Pipeline |
| `KUBECONFIG_DIR` | Directory containing kubeconfig files | Pipeline |

---

## Idempotency

All operations are designed to be safe to re-run. PodNetworks, PodNetworkInstances, namespaces, and the DaemonSet are **created once and reused** across all hourly runs — they are never deleted by the hourly pipeline. Only rotating pods are cycled. These resources are separate from the main pipeline's resources (different naming prefixes).

| Operation | Idempotency |
|-----------|-------------|
| Zone node pool creation | `az aks nodepool show` checks existence first |
| Node labeling | `--overwrite` flag; detects existing labels |
| PN/PNI/Namespace creation | Created once, reused. `kubectl get --ignore-not-found` checks existence |
| DaemonSet creation | Created once, runs indefinitely. Checks existence before creating |
| DaemonSet pod recovery | Kubernetes auto-restarts; pipeline only verifies |
| Pod rotation | Only rotating pods are deleted/recreated based on age |

---

## File Structure

```
.pipelines/swiftv2-long-running/
├── pipeline.yaml                              # Main pipeline entry point (every hour)
├── HOURLY-TESTS.md                            # This file
├── template/
│   ├── long-running-pipeline-template.yaml    # Infra setup + datapath tests + hourly tests
│   ├── datapath-tests-stage.yaml              # Per-workload datapath test stage
│   └── hourly-pod-tests-stage.yaml            # Per-zone: rotating + always-on + connectivity
└── scripts/
    ├── verify_infrastructure.sh               # Smart infra check (skip setup if exists)
    ├── ensure_zone_nodepools.sh               # Idempotent per-zone node pool creation
    ├── acquire_pipeline_lease.sh              # ConfigMap lease acquisition
    └── release_pipeline_lease.sh              # ConfigMap lease release

test/integration/swiftv2/longRunningCluster/
├── datapath_hourly_shared.go                  # Shared constants/utils for hourly tests (zone-aware)
├── datapath_hourly_rotating_test.go           # Rotating pods (tag: hourly_rotating_test)
├── datapath_hourly_alwayson_test.go           # DaemonSet always-on (tag: hourly_alwayson_test)
└── datapath_hourly_connectivity_test.go       # Hourly connectivity (tag: hourly_connectivity_test)

test/integration/manifests/swiftv2/long-running-cluster/
└── daemonset.yaml                             # DaemonSet manifest template (always-on)
```

---

## Build Tags

Each test file uses a unique Go build tag so tests can be run independently:

| Build Tag | File | Pipeline Job |
|-----------|------|-------------|
| `hourly_rotating_test` | `datapath_hourly_rotating_test.go` | RotatingPods_Z{N} |
| `hourly_alwayson_test` | `datapath_hourly_alwayson_test.go` | AlwaysOnPods_Z{N} (DaemonSet) |
| `hourly_connectivity_test` | `datapath_hourly_connectivity_test.go` | ConnectivityTest_Z{N} |
