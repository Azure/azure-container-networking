# AKS Migrate - Usage Guide

Detailed CLI reference for the `aksmigrate` tool.

---

## Table of Contents

1. [aksmigrate audit](#aksmigrate-audit)
2. [aksmigrate translate](#aksmigrate-translate)
3. [aksmigrate conntest](#aksmigrate-conntest)
4. [aksmigrate discover](#aksmigrate-discover)
5. [aksmigrate migrate](#aksmigrate-migrate)
6. [Common Workflows](#common-workflows)
7. [Grafana Dashboard](#grafana-dashboard)
8. [Audit Rules Reference](#audit-rules-reference)

---

## aksmigrate audit

Scans NetworkPolicies and cluster resources to detect breaking changes that will occur when migrating from Azure NPM to Cilium.

### Usage

```
aksmigrate audit [flags]
```

### Flags

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--kubeconfig` | string | | Path to kubeconfig file |
| `--input-dir` | string | | Path to directory containing Kubernetes YAML manifests |
| `--output` | string | `table` | Output format: `table` or `json` |
| `--k8s-version` | string | `1.29` | Target Kubernetes version (determines Cilium version) |

### Input Source Priority

1. `--input-dir` if specified (reads YAML files from the directory)
2. `--kubeconfig` if specified (connects to live cluster)
3. Default kubeconfig (uses `KUBECONFIG` env or `~/.kube/config`)

### Examples

**Audit from YAML files:**

```bash
aksmigrate audit --input-dir ./my-policies/ --k8s-version 1.29
```

**Audit from a live cluster:**

```bash
aksmigrate audit --kubeconfig ~/.kube/config --output json
```

**Audit using default kubeconfig:**

```bash
aksmigrate audit
```

### Output

The table format displays findings grouped by severity:

```
NPM-to-Cilium Migration Audit Report
=====================================
Cluster:   my-cluster
Timestamp: 2026-02-23T12:00:00Z
Policies:  15

Summary
-------
  FAIL: 3
  WARN: 2
  INFO: 1

[X] FAIL Findings (3)
------------------------------------------------------------

  [CILIUM-001] default/catch-all-egress
  ipBlock with broad CIDR 0.0.0.0/0 used without selector peers
  Fields: spec.egress[0].to[0].ipBlock.cidr
  Fix: Add a namespaceSelector peer alongside the ipBlock to ensure in-cluster traffic is still allowed

...

RESULT: MIGRATION BLOCKED - 3 critical issues must be resolved before migration.
```

### Exit Codes

- `0` - No FAIL findings (migration is safe or has only warnings)
- `1` - One or more FAIL findings detected (migration blocked)

---

## aksmigrate translate

Patches existing Kubernetes NetworkPolicies and generates supplementary CiliumNetworkPolicies to maintain behavioral equivalence after migration.

### Usage

```
aksmigrate translate [flags]
```

### Flags

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--kubeconfig` | string | | Path to kubeconfig file |
| `--input-dir` | string | | Path to directory containing Kubernetes YAML manifests |
| `--output-dir` | string | `./cilium-patches` | Output directory for generated YAML files |
| `--k8s-version` | string | `1.29` | Target Kubernetes version (determines Cilium version) |

### What It Does

1. **Patches ipBlock catch-all rules** - Adds `namespaceSelector: {}` peer alongside broad CIDR ipBlock rules so in-cluster traffic isn't blocked by Cilium's identity-based enforcement.

2. **Resolves named ports** - Looks up named ports (e.g., `http-api`) in pod specs and replaces them with numeric port values, since Cilium doesn't support named ports.

3. **Expands endPort ranges** - On Cilium versions before 1.17, expands `port`/`endPort` ranges into individual port entries (limited to ranges of 100 or fewer ports).

4. **Generates host egress CiliumNetworkPolicies** - Creates `allow-host-egress` CiliumNetworkPolicies with `toEntities: [host, remote-node]` for namespaces with restrictive egress policies (compensates for Cilium removing implicit local node access).

5. **Generates LB ingress CiliumNetworkPolicies** - Creates CiliumNetworkPolicies to allow `fromEntities: [world]` on specific ports for pods behind LoadBalancer/NodePort services with `externalTrafficPolicy: Cluster`.

### Examples

**Translate from YAML files:**

```bash
aksmigrate translate --input-dir ./my-policies/ --output-dir ./cilium-patches/
```

**Translate from a live cluster:**

```bash
aksmigrate translate --kubeconfig ~/.kube/config --output-dir ./cilium-patches/
```

### Output Directory Structure

```
cilium-patches/
├── patched/                          # Modified K8s NetworkPolicies
│   ├── default-catch-all-egress.yaml
│   └── default-named-port-policy.yaml
└── cilium/                           # New CiliumNetworkPolicies
    ├── production-allow-host-egress.yaml
    └── default-allow-lb-ingress-web-svc.yaml
```

---

## aksmigrate conntest

Captures connectivity snapshots before and after migration and validates that no regressions have been introduced.

### Subcommands

### `snapshot` - Capture connectivity state

```
aksmigrate conntest snapshot [flags]
```

| Flag | Type | Default | Required | Description |
|------|------|---------|----------|-------------|
| `--kubeconfig` | string | | No | Path to kubeconfig file |
| `--output` | string | | **Yes** | File path to save the snapshot JSON |
| `--phase` | string | | **Yes** | `pre-migration` or `post-migration` |

**Example:**

```bash
aksmigrate conntest snapshot \
  --kubeconfig ~/.kube/config \
  --phase pre-migration \
  --output ./pre-snapshot.json
```

### `validate` - Run post-migration validation

Captures a new post-migration snapshot, loads a pre-migration baseline, and compares them.

```
aksmigrate conntest validate [flags]
```

| Flag | Type | Default | Required | Description |
|------|------|---------|----------|-------------|
| `--kubeconfig` | string | | No | Path to kubeconfig file |
| `--pre-snapshot` | string | | **Yes** | Path to pre-migration snapshot JSON |
| `--output` | string | | No | Path to save the post-migration snapshot |

**Example:**

```bash
aksmigrate conntest validate \
  --kubeconfig ~/.kube/config \
  --pre-snapshot ./pre-snapshot.json \
  --output ./post-snapshot.json
```

**Exit code 1** if any regressions are detected.

### `diff` - Offline snapshot comparison

Compares two previously saved snapshots without cluster access.

```
aksmigrate conntest diff [flags]
```

| Flag | Type | Default | Required | Description |
|------|------|---------|----------|-------------|
| `--pre` | string | | **Yes** | Path to pre-migration snapshot JSON |
| `--post` | string | | **Yes** | Path to post-migration snapshot JSON |

**Example:**

```bash
aksmigrate conntest diff --pre ./pre-snapshot.json --post ./post-snapshot.json
```

### Probe Types

The connectivity prober generates four types of probes:

| Probe Type | Description | Target |
|------------|-------------|--------|
| `pod-to-pod` | Direct pod IP connectivity | Pod IP:80 |
| `pod-to-service` | ClusterIP service DNS resolution | `svc.ns.svc.cluster.local:port` |
| `pod-to-external` | External DNS reachability | 8.8.8.8:53 |
| `pod-to-node` | Node kubelet reachability | Node InternalIP:10250 |

Probes are executed by exec'ing into source pods and running `wget` (with `nc` fallback).

---

## aksmigrate discover

Scans an Azure subscription for AKS clusters using Azure NPM as their network policy engine and produces a prioritized migration plan.

### Prerequisites

- Azure CLI (`az`) installed and logged in
- `az` extension `resource-graph` installed (`az extension add --name resource-graph`)

### Usage

```
aksmigrate discover [flags]
```

### Flags

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--subscription` | string | | Azure subscription ID to scan (all subscriptions if omitted) |
| `--output` | string | `table` | Output format: `table` or `json` |

### Examples

**Discover across all subscriptions:**

```bash
aksmigrate discover
```

**Discover in a specific subscription:**

```bash
aksmigrate discover --subscription 12345678-1234-1234-1234-123456789abc
```

**JSON output for programmatic consumption:**

```bash
aksmigrate discover --subscription $SUB_ID --output json > fleet-plan.json
```

### Risk Assessment

Clusters are assigned a risk level based on:

| Factor | Risk Impact |
|--------|-------------|
| Windows node pools present | High (blocker) |
| Kubernetes version < 1.28 | High |
| Policy count > 50 | Medium |
| Node count > 100 | Medium |
| Otherwise | Low |

### Table Output

```
CLUSTER NAME                   RESOURCE GROUP            K8S VERSION  NODES      WINDOWS    RISK       ORDER
--------------------------------------------------------------------------------------------------------------
dev-cluster                    dev-rg                    1.30         3          No         low        1
staging-cluster                staging-rg                1.29         12         No         medium     2
prod-cluster                   prod-rg                   1.29         150        No         medium     3
legacy-cluster                 legacy-rg                 1.27         8          Yes        high       4

Fleet Summary
-------------
  Total clusters:       4
  Ready to migrate:     2
  Needs remediation:    1
  Blocked by Windows:   1
```

---

## aksmigrate migrate

Orchestrates the complete end-to-end migration workflow for a single AKS cluster.

### Prerequisites

- Azure CLI (`az`) 2.61+, logged in with permissions to update the cluster
- `kubectl` configured with cluster access
- `kubeconfig` for the target cluster

### Usage

```
aksmigrate migrate [flags]
```

### Flags

| Flag | Type | Default | Required | Description |
|------|------|---------|----------|-------------|
| `--cluster-name` | string | | **Yes** | Name of the AKS cluster |
| `--resource-group` | string | | **Yes** | Azure resource group containing the cluster |
| `--kubeconfig` | string | | No | Path to kubeconfig file |
| `--output-dir` | string | `./migration-output` | No | Directory to write migration artifacts |
| `--skip-snapshot` | bool | `false` | No | Skip pre-migration connectivity snapshot |
| `--skip-validation` | bool | `false` | No | Skip post-migration connectivity validation |
| `--dry-run` | bool | `false` | No | Preview migration plan without making changes |
| `--k8s-version` | string | `1.29` | No | Target Kubernetes version |

### Migration Steps

The orchestrator runs 7 sequential steps:

| Step | Phase | Skippable | Description |
|------|-------|-----------|-------------|
| 1/7 | Preflight | No | Checks Azure CLI version, Windows nodes, K8s version, PDBs |
| 2/7 | Snapshot | `--skip-snapshot` | Captures pre-migration connectivity baseline |
| 3/7 | Audit | No | Runs full policy analysis; blocks on FAIL findings |
| 4/7 | Translate | No | Generates patched policies and CiliumNetworkPolicies |
| 5/7 | Migrate | `--dry-run` | Runs `az aks update --network-dataplane cilium` |
| 6/7 | Patch | `--dry-run` | Applies translated policies via `kubectl apply` |
| 7/7 | Validate | `--skip-validation` / `--dry-run` | Compares pre/post connectivity snapshots |

### Examples

**Dry run (recommended first):**

```bash
aksmigrate migrate \
  --cluster-name my-cluster \
  --resource-group my-rg \
  --kubeconfig ~/.kube/config \
  --k8s-version 1.29 \
  --dry-run
```

**Full migration:**

```bash
aksmigrate migrate \
  --cluster-name my-cluster \
  --resource-group my-rg \
  --kubeconfig ~/.kube/config \
  --output-dir ./migration-artifacts/
```

**Migration without connectivity tests (faster, less safe):**

```bash
aksmigrate migrate \
  --cluster-name my-cluster \
  --resource-group my-rg \
  --skip-snapshot \
  --skip-validation
```

### Output Directory

```
migration-output/
├── pre-migration-snapshot.json     # Connectivity baseline
├── post-migration-snapshot.json    # Post-migration connectivity
└── patches/
    ├── patched-netpol-*.yaml       # Modified NetworkPolicies
    └── cilium-netpol-*.yaml        # Generated CiliumNetworkPolicies
```

### Preflight Checks

The orchestrator validates the following before proceeding:

- Azure CLI version >= 2.61.0
- No Windows node pools (Cilium does not support Windows)
- Kubernetes version >= 1.28
- StatefulSets have PodDisruptionBudgets (warns if missing)

---

## Common Workflows

### Workflow 1: Offline policy assessment (no cluster needed)

Export your cluster's policies to YAML, then analyze locally:

```bash
# Export from cluster (one-time)
kubectl get networkpolicies -A -o yaml > policies.yaml
kubectl get pods -A -o yaml > pods.yaml
kubectl get services -A -o yaml > services.yaml

# Place them in a directory
mkdir cluster-export/
mv policies.yaml pods.yaml services.yaml cluster-export/

# Audit
aksmigrate audit --input-dir ./cluster-export/ --k8s-version 1.29

# Translate
aksmigrate translate --input-dir ./cluster-export/ --output-dir ./patches/
```

### Workflow 2: Full fleet migration

```bash
# Step 1: Discover all NPM clusters
aksmigrate discover --subscription $SUB_ID --output json > fleet.json

# Step 2: For each cluster, start with a dry run
aksmigrate migrate --cluster-name dev-cluster --resource-group dev-rg --dry-run

# Step 3: Review audit findings and patches in ./migration-output/

# Step 4: Run the real migration on the lowest-risk cluster first
aksmigrate migrate --cluster-name dev-cluster --resource-group dev-rg

# Step 5: Validate connectivity
aksmigrate conntest diff \
  --pre ./migration-output/pre-migration-snapshot.json \
  --post ./migration-output/post-migration-snapshot.json

# Step 6: Repeat for remaining clusters in priority order
```

### Workflow 3: Pre/post connectivity validation only

If you are running the `az aks update` command yourself:

```bash
# Before migration
aksmigrate conntest snapshot --phase pre-migration --output ./pre.json

# <run your migration>

# After migration
aksmigrate conntest validate --pre-snapshot ./pre.json --output ./post.json
```

---

## Grafana Dashboard

The `dashboards/migration-monitor.json` file contains a Grafana dashboard for monitoring the migration in real-time.

### Import

1. Open Grafana
2. Go to **Dashboards > Import**
3. Upload `dashboards/migration-monitor.json`
4. Select your Prometheus data source

### Panels

| Panel | Metric | Purpose |
|-------|--------|---------|
| Cilium Agent Pod Status | `kube_pod_status_phase{pod=~"cilium-.*"}` | Verify Cilium agents are running |
| Cilium Endpoint Count | `cilium_endpoint_state` | Track endpoint registration |
| Policy Drops (Hubble) | `hubble_drop_total` | Detect traffic being dropped |
| DNS Query Latency | `hubble_dns_response_time_seconds_bucket` | Monitor DNS performance |
| TCP Connection Status | `hubble_tcp_flags_total` | Watch for RST spikes |
| Node Reimage Progress | `kube_node_status_condition` | Track node readiness |
| Migration Checklist | (text) | Step-by-step migration checklist |

---

## Audit Rules Reference

### CILIUM-001: ipBlock Catch-All

**Severity:** FAIL

**Trigger:** An ipBlock peer with a broad CIDR (0.0.0.0/0, /8, /12, /16) is the only peer in a rule, without a corresponding `namespaceSelector` or `podSelector` peer.

**Why:** In NPM, ipBlock CIDRs match all traffic including in-cluster pods. In Cilium, ipBlock only matches traffic from IPs that don't have a Cilium identity (i.e., external traffic). This means in-cluster pod traffic that previously matched the ipBlock will be denied.

**Fix:** Add a `namespaceSelector: {}` peer alongside the ipBlock to explicitly allow in-cluster traffic. The translator does this automatically.

### CILIUM-002: Named Ports

**Severity:** WARN (single mapping), FAIL (conflicting mappings)

**Trigger:** A NetworkPolicy references a port by name (e.g., `http-api`) instead of by number.

**Why:** Cilium does not support named ports in NetworkPolicies. The policy will not match any traffic.

**Fix:** Replace named ports with numeric values. The translator resolves these by looking up port names in pod container specs. Conflicting mappings (same name, different numbers across pods) require manual resolution.

### CILIUM-003: endPort Ranges

**Severity:** FAIL (Cilium < 1.17)

**Trigger:** A NetworkPolicy uses `endPort` to specify a port range, and the target Cilium version is before 1.17.

**Why:** Cilium added endPort support in version 1.17 (for K8s 1.32+). Older versions silently ignore the endPort field, enforcing only the start port.

**Fix:** Upgrade to K8s 1.32+ / Cilium 1.17+, or use the translator to expand the range into individual port entries.

### CILIUM-004: Implicit Local Node Egress

**Severity:** WARN

**Trigger:** Any egress policy exists (restrictive egress means pods can't reach node-local services).

**Why:** NPM implicitly allows pods to reach services on their local node (kubelet, node-local DNS, etc.). Cilium enforces strict egress and blocks this traffic unless explicitly allowed.

**Fix:** Create a CiliumNetworkPolicy with `toEntities: [host, remote-node]`. The translator generates this automatically.

### CILIUM-005: LB/NodePort Ingress Enforcement

**Severity:** FAIL

**Trigger:** A namespace has ingress deny-all policies AND has Services of type LoadBalancer or NodePort with `externalTrafficPolicy: Cluster`.

**Why:** NPM does not enforce NetworkPolicies on traffic entering through LoadBalancer/NodePort Services. Cilium enforces these policies, which means external traffic to LB-backed pods will be blocked.

**Fix:** Create CiliumNetworkPolicies allowing `fromEntities: [world]` on the appropriate ports. The translator generates these automatically.

### CILIUM-006: Host-Networked Pods

**Severity:** WARN

**Trigger:** A pod using `hostNetwork: true` is targeted by a NetworkPolicy (via label selector match).

**Why:** Cilium cannot enforce NetworkPolicies on host-networked pods because they share the node's network namespace. The policies will have no effect.

**Fix:** Consider migrating to non-host-networked alternatives, or use Cilium HostPolicies.

### CILIUM-007: kube-proxy Removal

**Severity:** INFO

**Trigger:** Always emitted.

**Why:** When migrating to Cilium on AKS, kube-proxy is removed and Cilium takes over service load balancing. This is informational but important to know for debugging.

### CILIUM-008: Identity Exhaustion

**Severity:** WARN

**Trigger:** More than 50,000 unique label combinations detected across pods.

**Why:** Cilium assigns identities based on pod label sets. The identity limit is 65,535. Clusters with high label cardinality (e.g., Spark jobs with unique job IDs as labels) may hit this limit.

**Fix:** Reduce label cardinality by removing high-churn labels from pods or using label aggregation.

### CILIUM-009: Service Mesh Detected

**Severity:** INFO

**Trigger:** Pods with Istio (`istio-proxy`) or Linkerd (`linkerd-proxy`) sidecar containers are detected.

**Why:** Service mesh sidecars may interact with Cilium's datapath. Cilium has its own L7 policy enforcement and can conflict with mesh-injected iptables rules.

**Fix:** Test thoroughly in a staging environment. Consider using Cilium's native L7 policies instead of the mesh for network policy enforcement.
