# Comprehensive Research Report: Migrating AKS from Azure CNI + NPM to Azure CNI Powered by Cilium

**Date:** February 23, 2026
**Team:** Azure Container Networking
**Purpose:** Research report and tooling plan for brownfield AKS customer migration from NPM/iptables to Cilium/eBPF

---

## Table of Contents

1. [Executive Summary](#1-executive-summary)
2. [Current AKS Networking Landscape](#2-current-aks-networking-landscape)
3. [The Upgrade Path — What Exists Today](#3-the-upgrade-path--what-exists-today)
4. [Critical Breaking Changes](#4-critical-breaking-changes)
5. [Technical Deep-Dive: NPM vs Cilium Internals](#5-technical-deep-dive-npm-vs-cilium-internals)
6. [Network Policy Translation: Full Compatibility Matrix](#6-network-policy-translation-full-compatibility-matrix)
7. [IP Address Management Differences](#7-ip-address-management-differences)
8. [Brownfield Challenges](#8-brownfield-challenges)
9. [Observability Gaps During Migration](#9-observability-gaps-during-migration)
10. [Community and Ecosystem](#10-community-and-ecosystem)
11. [Tooling Gap Analysis — What We Can Build](#11-tooling-gap-analysis--what-we-can-build)
12. [Proposed Project Structure](#12-proposed-project-structure)
13. [Implementation Priority and Roadmap](#13-implementation-priority-and-roadmap)
14. [Key Technical Risks](#14-key-technical-risks)
15. [Migration Playbook Summary](#15-migration-playbook-summary)
16. [Sources](#16-sources)

---

## 1. Executive Summary

Azure Network Policy Manager (NPM) — the iptables-based network policy engine for AKS — is being retired. **NPM for Windows reaches end-of-support on September 30, 2026**, and **NPM for Linux on September 30, 2028**. All brownfield AKS customers currently using Azure CNI + NPM must migrate to **Azure CNI Powered by Cilium**, which uses eBPF for packet forwarding, service routing, and network policy enforcement.

While Microsoft provides a single `az aks update --network-dataplane cilium` command for in-place migration, the process has **seven documented breaking changes** that can silently break production workloads. There is **no automated policy analysis, translation, or validation tooling** available today. This report identifies the gaps and proposes a suite of migration helper tools to enable safe brownfield migration at scale.

### Key Findings

- **In-place upgrade is supported** but reimages all node pools simultaneously (no per-pool rollout)
- **The upgrade is irreversible** — no rollback path exists
- **Policy enforcement gap** exists during migration (neither NPM nor Cilium enforces during the reimage window)
- **The #1 customer pain point** is the `ipBlock` behavioral change: `0.0.0.0/0` no longer matches pod/node IPs under Cilium
- **No automated tooling** exists to detect, remediate, or validate these breaking changes
- **Six tools** are proposed to fill the gap, with the Policy Analyzer and Policy Translator at P0 priority

---

## 2. Current AKS Networking Landscape

### 2.1 What is Azure CNI with iptables and NPM?

Azure CNI with NPM (Azure Network Policy Manager) is the original Microsoft-built network policy engine for AKS:

- **Network plugin:** `--network-plugin azure` with `--network-policy azure`
- **Data plane:** Traditional Linux kernel networking stack using **iptables** and **ipsets**
- **Policy enforcement:** NPM watches Kubernetes NetworkPolicy objects and translates them into iptables rules and ipset membership entries on each node
- **IPAM modes:** Azure CNI Node Subnet (pods get IPs from the VNet subnet) or Azure CNI with Dynamic Pod IP Assignment (pods get IPs from a dedicated pod subnet)
- **Service routing:** Uses **kube-proxy** in iptables mode for ClusterIP/NodePort/LoadBalancer service routing
- **Scale limits:** NPM officially supports up to **250 nodes** and **20,000 pods**. Beyond this, OOM errors are likely.

### 2.2 What is Azure CNI Powered by Cilium?

Azure CNI Powered by Cilium combines the Azure CNI control plane for IPAM with the Cilium data plane using **eBPF** programs loaded into the Linux kernel:

- **Network plugin:** `--network-plugin azure` with `--network-dataplane cilium`
- **Data plane:** Cilium eBPF programs for packet forwarding, service routing, and network policy enforcement
- **No kube-proxy:** AKS clusters with Cilium do **not** use kube-proxy; Cilium handles all service routing via eBPF
- **Policy enforcement:** Cilium enforces Kubernetes NetworkPolicy and also supports CiliumNetworkPolicy and CiliumClusterwideNetworkPolicy CRDs
- **IPAM modes supported:**
  - **Overlay mode** (`--network-plugin-mode overlay --pod-cidr 192.168.0.0/16`) — pods get IPs from a private overlay CIDR
  - **VNet mode** (dynamic pod IP assignment with `--pod-subnet-id`) — pods get IPs from an Azure VNet subnet
  - **Node Subnet mode** — pods get IPs from the node's subnet (requires Azure CLI 2.69.0+)

**Key benefits:**
- Improved service routing performance (eBPF vs iptables)
- More efficient network policy enforcement (O(1) BPF map lookups vs O(n) iptables chain traversal)
- Better observability of cluster traffic (with ACNS add-on)
- Support for larger clusters: 5,000+ nodes, 250 pods/node
- FQDN filtering and L7 policies (with ACNS)
- Dual-stack (IPv4/IPv6) support

### 2.3 Cilium Version Matrix on AKS

| Kubernetes Version | Minimum Cilium Version | endPort Support |
|---|---|---|
| 1.29 (LTS) | 1.14.19 | No |
| 1.30 | 1.14.19 | No |
| 1.31 | 1.16.6 | No |
| 1.32 | 1.17.0 | Yes |
| 1.33 | 1.17.0 | Yes |

This version matrix is critical because `endPort` (port ranges in NetworkPolicy) only works on Cilium 1.17+. Clusters on K8s 1.29-1.31 need port ranges expanded to individual port entries.

### 2.4 NPM Retirement Timeline

| Milestone | Date |
|---|---|
| NPM on Windows — no new onboarding | Already in effect |
| NPM on Windows — end of support | **September 30, 2026** |
| NPM on Linux — end of support | **September 30, 2028** |
| Kubenet — end of support | **March 31, 2028** |

### 2.5 ACNS Feature Matrix

Features requiring the **ACNS (Advanced Container Networking Services)** paid add-on:

| Feature | Without ACNS | With ACNS |
|---|---|---|
| K8s NetworkPolicy | Yes | Yes |
| Cilium L3/L4 Network Policies | Yes | Yes |
| CiliumClusterwideNetworkPolicy | Yes | Yes |
| Cilium Endpoint Slices | Yes | Yes |
| FQDN Filtering | **No** | Yes |
| L7 Network Policies (HTTP/gRPC/Kafka) | **No** | Yes |
| Container Network Observability (metrics + flow logs) | **No** | Yes |
| eBPF Host Routing | **No** | Yes |
| WireGuard Encryption | **No** | Yes (preview) |

Features **not available** with Cilium on AKS (regardless of ACNS):
- Windows node pools (Linux only)
- Custom Cilium configuration (AKS manages Cilium config; use BYO CNI for custom config)
- L7 policy on CiliumClusterwideNetworkPolicy (L7 only on namespace-scoped CiliumNetworkPolicy)
- Multiple services sharing the same host port with different protocols (Cilium <= 1.16, see Cilium issue #14287)
- AKS Local DNS is NOT compatible with ACNS FQDN Filtering

---

## 3. The Upgrade Path — What Exists Today

### 3.1 Official Migration Tooling from Microsoft

Microsoft provides:

1. **Azure CLI `az aks update` command** — the primary migration mechanism
2. **Azure Resource Graph query** — to discover impacted clusters
3. **A migration guide** — documenting breaking changes and workarounds
4. **No policy conversion tool** — no automated policy analysis, translation, or validation

### 3.2 In-Place Upgrade Command

```bash
# Step 1 (optional): Migrate IPAM to Overlay (irreversible, separate step)
az aks update \
  --name $CLUSTER_NAME \
  --resource-group $RESOURCE_GROUP \
  --network-plugin-mode overlay \
  --pod-cidr 192.168.0.0/16

# Step 2: Switch dataplane to Cilium
az aks update \
  --name $CLUSTER_NAME \
  --resource-group $RESOURCE_GROUP \
  --network-dataplane cilium
```

**Key constraints:**
- All node pools are reimaged **simultaneously** (no per-pool rollout)
- IPAM mode change and dataplane change **cannot** be done in a single operation (for Azure CNI Node Subnet)
- No Windows node pool support
- Network policies must be uninstalled before IPAM change to Overlay (but NOT before dataplane change to Cilium)
- Cilium begins enforcement only **after all nodes are reimaged** — there is a policy enforcement gap
- **No rollback path** — this is irreversible
- Requires Azure CLI version 2.52.0 or later

### 3.3 Node Pool Migration Strategy

The official Microsoft documentation states explicitly:

> "The update process triggers node pools to be reimaged **simultaneously**. Updating each node pool separately isn't supported."

This means:
- There is **no way** to do a rolling, node-pool-by-node-pool migration
- The disruption is comparable to a Kubernetes version upgrade
- The `--network-dataplane` is a **cluster-level** setting, not a node-pool-level setting
- You cannot have a mixed-mode cluster where some pools run NPM and others run Cilium

**Blue/Green Cluster Strategy (the only zero-downtime path):**

For customers requiring zero downtime, the only supported approach is:
1. Create a new AKS cluster with `--network-dataplane cilium` from day one
2. Deploy the same workloads and policies to the new cluster
3. Validate policy enforcement on the new cluster
4. Shift traffic (via Azure Traffic Manager, Front Door, or DNS)
5. Decommission the old cluster

### 3.4 Supported Upgrade Paths

| Source | Target | Supported? |
|---|---|---|
| Azure CNI (Node Subnet) + NPM | Azure CNI (Node Subnet) + Cilium | Yes (single step) |
| Azure CNI (Node Subnet) + NPM | Azure CNI Overlay + Cilium | Yes (two steps) |
| Azure CNI + Calico | Azure CNI + Cilium | Yes (single step) |
| Kubenet | Azure CNI Overlay + Cilium | Yes (two steps) |
| BYO CNI | Azure CNI + Cilium | **No** |
| Azure CNI + Dynamic IP Allocation | Azure CNI Overlay + Cilium | **No** (must stay on VNet IPAM) |

### 3.5 Azure Resource Graph Query for Discovery

```kusto
Resources
| where type == "microsoft.containerservice/managedclusters"
| mv-expand agentPool = properties.agentPoolProfiles
| where agentPool.osType != "Windows"
| extend netPol = tolower(tostring(properties.networkProfile.networkPolicy))
| where netPol == "azure"
| summarize by name, location, resourceGroup, netPol,
    k8sVersion = tostring(properties.kubernetesVersion),
    networkPlugin = tostring(properties.networkProfile.networkPlugin)
```

---

## 4. Critical Breaking Changes

These behavioral differences are documented by Microsoft but there is **no automated detection or remediation tooling**. They apply the moment Cilium begins enforcement after migration.

### 4.1 ipBlock Behavior Change (HIGHEST IMPACT)

**NPM behavior:** `ipBlock` with CIDR `0.0.0.0/0` allows egress to ALL destinations — pods, nodes, and external IPs. The CIDR is matched against raw packet source/destination IPs.

**Cilium behavior:** `ipBlock` with CIDR `0.0.0.0/0` will **block egress to pod and node IPs** even though they fall within the CIDR range. Cilium treats pod and node identities separately from raw IP-based rules. `ipBlock` only matches the "world" identity (external/non-cluster IPs).

**Impact:** Any egress policy using `0.0.0.0/0` as a catch-all silently breaks pod-to-pod and pod-to-node traffic after migration. This is the **#1 migration blocker** reported by customers.

**Workaround:** Add explicit `namespaceSelector: {}` and `podSelector: {}` rules alongside the ipBlock to allow pod-to-pod traffic. For node IPs, create a CiliumNetworkPolicy:

```yaml
# Patch to existing NetworkPolicy (add these peers alongside ipBlock)
egress:
  - to:
    - ipBlock:
        cidr: 0.0.0.0/0
    - namespaceSelector: {}  # allows traffic to all pods in all namespaces
    - podSelector: {}        # (within the policy's namespace for podSelector)

---
# Supplementary CiliumNetworkPolicy for node access
apiVersion: "cilium.io/v2"
kind: CiliumNetworkPolicy
metadata:
  name: allow-node-egress
  namespace: <namespace>
spec:
  endpointSelector: {}
  egress:
    - toEntities:
        - host         # local node
        - remote-node  # other nodes in the cluster
```

### 4.2 Implicit Local Node Egress Allow Removed

**NPM behavior:** Egress traffic from a pod to its own node's IP is **implicitly allowed**, even when egress policies restrict other traffic.

**Cilium behavior:** Egress to the local node is **blocked** unless explicitly allowed. This is a fundamental behavioral difference.

**Impact:** Workloads that communicate with node-level services (kubelet health checks, node-local caches, host-networked DaemonSets) may silently lose connectivity.

**Workaround:** Add a CiliumNetworkPolicy after migration that explicitly allows egress to the `host` entity.

### 4.3 Ingress via LoadBalancer/NodePort with externalTrafficPolicy=Cluster

**NPM behavior:** With `externalTrafficPolicy=Cluster` (the default), ingress traffic arriving via LoadBalancer or NodePort services **bypasses ingress network policy enforcement**. Even a deny-all ingress policy does not block LoadBalancer traffic.

**Cilium behavior:** Cilium **enforces ingress policies on ALL traffic**, including traffic routed internally due to `externalTrafficPolicy=Cluster`. A deny-all ingress policy **WILL** block LoadBalancer and NodePort traffic.

**Impact:** Workloads behind LoadBalancers that have restrictive ingress policies (especially deny-all defaults) will lose external connectivity after migration.

**Workaround:** Review all ingress policies for workloads behind LoadBalancer or NodePort services. Add explicit ingress allow rules for expected traffic sources.

### 4.4 Named Ports Inconsistency

**NPM behavior:** Named ports in NetworkPolicy work consistently by resolving to the container port number.

**Cilium behavior:** Named ports may fail to enforce correctly when the same port name maps to different port numbers across different pods. See [Cilium GitHub issue #30003](https://github.com/cilium/cilium/issues/30003).

**Workaround:** Replace ALL named ports with their numeric values before migration.

```yaml
# BEFORE (risky under Cilium)
ports:
  - port: http-api
    protocol: TCP

# AFTER (safe under both NPM and Cilium)
ports:
  - port: 8080
    protocol: TCP
```

### 4.5 endPort Support (Port Ranges)

**NPM behavior:** The `endPort` field for port ranges is fully supported.

**Cilium behavior:** `endPort` is only supported starting with Cilium version 1.17+. On earlier versions, policies with endPort are silently ignored — the policy compiles without error but does not enforce the range.

**Workaround:** Verify your Cilium version (based on K8s version — see Section 2.3). If < 1.17, replace endPort ranges with individual port entries.

### 4.6 Host-Networked Pods

**NPM behavior:** Network policies are applied to pods using host networking.

**Cilium behavior:** Network policies are **NOT applied** to pods using host networking (`spec.hostNetwork: true`). These pods use the host identity instead of individual identities.

**Impact:** Any security controls enforced via NetworkPolicy on host-networked pods (monitoring agents, log collectors, CNI plugins) will stop being enforced.

### 4.7 kube-proxy Removal

**NPM behavior:** kube-proxy runs as a DaemonSet and manages iptables rules for service routing.

**Cilium behavior:** kube-proxy is **completely removed**. Cilium takes over all service routing via eBPF-based service maps.

**Impact:**
- Any tooling or monitoring that parses kube-proxy's iptables chains will break
- `iptables -t nat -L` will show empty chains for service routing
- Custom kube-proxy configurations are lost
- Service mesh interactions (Istio, Linkerd) may be affected (see Section 8.2)

---

## 5. Technical Deep-Dive: NPM vs Cilium Internals

### 5.1 How NPM Implements Network Policies (iptables + ipsets)

NPM operates as a DaemonSet on each node with the following architecture:

1. **Policy Watcher:** NPM watches the Kubernetes API server for NetworkPolicy objects
2. **Translation Layer:** Each NetworkPolicy is decomposed into:
   - **ipset entries:** Sets of IP addresses or CIDR ranges that represent pod selectors, namespace selectors, and ipBlock rules (named `azure-npm-{hash}`)
   - **iptables rules:** Chain rules in the FILTER table that reference the ipsets
3. **Rule Application:** For each policy:
   - An ipset is created for each selector containing the matching pod IPs
   - iptables rules are inserted in the FORWARD chain to ACCEPT or DROP packets matching the ipset + port + protocol combinations
4. **Rule Updates:** When pods are created/deleted or labels change, NPM updates the ipset membership. When policies change, iptables rules are rewritten.

**Performance characteristics:**
- iptables rule evaluation is **O(n)** — every packet traverses the full chain
- Large numbers of rules (many policies or large ipBlocks) degrade forwarding performance
- ipset lookups are O(1) for hash-based sets, but the number of iptables rules referencing them is still linear
- NPM is known to have **race conditions** when editing large policies, causing temporary connectivity issues
- Scale ceiling: ~250 nodes, ~20K pods before OOM

### 5.2 How Cilium Implements Network Policies (eBPF)

Cilium uses a fundamentally different architecture:

1. **Identity-Based Enforcement:** Instead of IP addresses, Cilium assigns a **security identity** (a numeric ID from a pool of 65,535) to each group of pods sharing the same set of labels. Policies are evaluated against identities, not IPs. This is why `ipBlock` cannot select pod/node IPs — Cilium resolves those to identities, not raw IPs.

2. **eBPF Programs:** Cilium compiles network policies into eBPF programs that are attached to the Linux kernel's TC (Traffic Control) hook on each pod's virtual ethernet interface (veth pair). These programs run in kernel space with near-native performance.

3. **Policy Map:** Each endpoint (pod) has a **BPF policy map** (hash table in kernel memory) that contains:
   - Allowed identities for ingress
   - Allowed identities for egress
   - Port/protocol restrictions
   - L7 policy references (if ACNS is enabled)

4. **Conntrack (Connection Tracking):** Cilium maintains its own eBPF-based connection tracking table, replacing the kernel's netfilter conntrack. This provides more efficient stateful policy enforcement.

5. **DNS Proxy:** For FQDN-based policies (ACNS), Cilium runs a transparent DNS proxy that intercepts DNS queries and maps resolved IPs to FQDN-based policy rules.

6. **No iptables dependency:** Cilium does not use iptables for policy enforcement or service routing. It replaces kube-proxy entirely with eBPF-based service maps.

**Performance characteristics:**
- eBPF policy evaluation is **O(1)** using BPF maps (hash tables in kernel memory)
- Policy updates are incremental (only the affected endpoint's BPF map is updated)
- No linear chain traversal — packets are matched against the BPF map in constant time
- Scales to thousands of nodes and hundreds of thousands of pods
- Identity limit: 65,535 — high-churn workloads can exhaust this

### 5.3 Evaluation Model Comparison

**NPM (iptables) evaluation model:**
1. Packets enter the iptables FORWARD chain
2. Rules are evaluated in order (top to bottom)
3. First match wins (ACCEPT or DROP)
4. Default policy (if no NetworkPolicy selects the pod): ACCEPT all
5. If any NetworkPolicy selects the pod: default DENY for the policy type, then evaluate rules
6. `ipBlock` CIDR ranges match against **raw packet source/destination IPs**
7. Pod selector rules match against pod IPs resolved at policy application time via ipsets

**Cilium (eBPF) evaluation model:**
1. Packets enter the eBPF program at the TC hook on the pod's veth interface
2. Source/destination **identity** is determined (not IP address)
3. The BPF policy map is consulted for the endpoint
4. **Identity-based matching:** Pod and node IPs are matched via their Cilium identity, NOT via raw IP
5. `ipBlock` CIDR ranges **ONLY match external (non-pod, non-node) IPs** — this is the root cause of the ipBlock behavioral difference
6. Default behavior: same as K8s spec (if a NetworkPolicy selects a pod, default deny applies)
7. Rules within a policy are OR'd (any matching rule allows traffic)
8. Multiple policies selecting the same pod are additive per direction (union of all allowing rules)
9. **Implicit allows differ from NPM:** No implicit local-node egress allow; full enforcement on LB/NodePort ingress

---

## 6. Network Policy Translation: Full Compatibility Matrix

### 6.1 Feature-by-Feature Comparison

| Feature | NPM (iptables) | Cilium (eBPF) | Action Required |
|---|---|---|---|
| Kubernetes NetworkPolicy v1 | Full support | Full support (with caveats) | Audit ipBlock, named ports |
| CiliumNetworkPolicy CRD | N/A | Supported (L3/L4) | New capability |
| CiliumClusterwideNetworkPolicy | N/A | Supported (L3/L4 only) | New capability |
| ipBlock selecting pod/node IPs | Works as expected | Does NOT work (identity-based) | **CRITICAL: Add selectors + CNP** |
| ipBlock `except` field | Works on Linux | Works on Linux | None |
| Named ports | Works consistently | Buggy (issue #30003) | Replace with numeric ports |
| endPort (port ranges) | Works | Cilium 1.17+ only | Check Cilium version |
| Namespace selectors | Works | Works | None |
| Pod selectors (matchExpressions) | Works | Works | None |
| Default deny ingress | Leaky for LB/NodePort traffic | Strict enforcement on ALL traffic | **Review LB-exposed services** |
| Default deny egress | Allows local node implicitly | Strict enforcement | **Add host entity allow** |
| Egress to local node | Implicitly allowed | Blocked unless allowed | **Add CiliumNetworkPolicy** |
| FQDN filtering | Not supported | Requires ACNS add-on | New capability |
| L7 policies (HTTP/gRPC/Kafka) | Not supported | Requires ACNS add-on | New capability |
| Host-networked pod policies | Applied | NOT applied | Review hostNetwork pods |
| SCTP protocol | Not supported on Windows | Supported | N/A (Cilium is Linux-only) |

### 6.2 Calico vs NPM vs Cilium Policy Semantics

For customers also evaluating Calico:

| Aspect | NPM (Azure) | Calico | Cilium |
|---|---|---|---|
| Enforcement mechanism | iptables + ipsets | iptables + ipsets (Linux), HNS (Windows) | eBPF |
| CRD policies | None | CalicoNetworkPolicy, GlobalNetworkPolicy | CiliumNetworkPolicy, CiliumClusterwideNetworkPolicy |
| ipBlock for pod/node IPs | Works | Works | Does NOT work (identity-based) |
| Host-networked pod policies | Enforced | Enforced (with host endpoint policies) | NOT enforced |
| FQDN policies | Not available | Available (Calico Enterprise) | Available (with ACNS) |
| L7 policies | Not available | Available (Calico Enterprise) | Available (with ACNS) |
| Global/cluster-wide policies | Not available | GlobalNetworkPolicy | CiliumClusterwideNetworkPolicy |
| Windows support | Yes | Yes (2019, 2022) | **No** |
| Scale | 250 nodes / 20K pods | Higher (depends on config) | 5,000+ nodes |

**Note:** Migration from Calico to Cilium is also supported via `az aks update --network-dataplane cilium`. Calico CRDs and CRs are NOT automatically deleted — they must be manually cleaned up.

---

## 7. IP Address Management Differences

### 7.1 If Migrating IPAM Mode (Azure CNI Node Subnet to Overlay)

| Behavior | Azure CNI Node Subnet | Azure CNI Overlay |
|---|---|---|
| Pod CIDR | VNet subnet (e.g., 10.240.0.0/16) | Overlay CIDR (e.g., 192.168.0.0/16) |
| Pod IP visibility | VNet-routable | NOT VNet-routable |
| Egress SNAT | No SNAT (pod IP is VNet IP) | SNAT to node IP for traffic leaving cluster |
| Ingress | Direct to pod IP possible | Must go through Service (LB, Ingress) |
| Peered VNet access | Direct pod-to-VM | SNAT'd; VM sees node IP |
| On-premises access | Direct pod IP reachable via ExpressRoute/VPN | Pod IP NOT reachable; must use Service |
| IP consumption | 1 VNet IP per pod | Only node IPs from VNet; pods use overlay CIDR |
| Max nodes | 250 (NPM limit) | 5,000 nodes, 250 pods per node |
| Pod CIDR requirement | N/A (uses VNet IPs) | Must be specified (e.g., `192.168.0.0/16`) |

**Critical impact:** If any external systems (monitoring, firewalls, audit logs, on-prem routing) rely on pod IP addresses, switching to Overlay will break those integrations because all pod traffic will be SNAT'd to the node IP.

### 7.2 If Keeping Azure CNI Node Subnet (Flat) with Cilium

Pod CIDR behavior remains the same. No SNAT changes. Pod IPs remain VNet-routable. Only the dataplane (iptables to eBPF) and policy engine (NPM to Cilium) change. This is the **lower-risk** migration path for customers who depend on direct pod IP reachability.

### 7.3 IPAM Migration Constraints

- The `--pod-cidr` must NOT overlap with the VNet address space or the existing service CIDR
- Updating to Azure CNI Overlay is **irreversible**
- Custom `azure-ip-masq-agent` configurations may break — pod IPs from the overlay space are unreachable from outside the cluster
- Old `azure-ip-masq-agent-config` ConfigMaps should be deleted before upgrading (unless intentionally in place)
- Only `azure-ip-masq-agent-config-reconciled` ConfigMap should exist; it's updated automatically

---

## 8. Brownfield Challenges

### 8.1 Stateful Workloads During Migration

The migration triggers a **full node pool reimage**, which means all pods on a node are evicted and rescheduled:

| Workload Type | Impact | Mitigation |
|---|---|---|
| **StatefulSets with Azure Disks (RWO)** | Pod must reschedule to a node where the disk can be attached; may cause delays | Ensure PDBs allow at least one pod to be evicted; verify replica health |
| **StatefulSets with Azure Files (RWM)** | Less impactful; multiple nodes can mount the same share | Standard PDB protections |
| **Databases / Message Queues** | Replicas may lose quorum temporarily | Run pre-migration health checks; ensure sufficient replicas |
| **Long-running jobs** | Jobs are interrupted and must be restarted | Use checkpointing; implement graceful termination |
| **Active TCP connections** | All existing connections are terminated | Applications must handle reconnection gracefully |

**Recommendations:**
- Set PodDisruptionBudgets (PDBs) on all critical workloads
- Ensure replica counts are sufficient to survive rolling eviction
- Validate that stateful workloads can tolerate rolling restarts
- Consider draining/cordoning nodes manually for critical stateful workloads

### 8.2 Service Mesh Interactions

#### Istio
- Istio uses its own iptables rules for traffic interception (via `istio-init` init container or Istio CNI plugin)
- The removal of kube-proxy and the change to eBPF-based service routing can affect Istio's traffic interception
- **Cilium's kube-proxy replacement:** Istio expects kube-proxy for ClusterIP resolution. Cilium replaces this, which is generally compatible but may require Istio version >= 1.16 for full compatibility
- AKS managed Cilium does not allow custom configuration, so you cannot enable Cilium's own service mesh features to replace Istio
- **Recommendation:** Test Istio connectivity thoroughly in a non-production cluster before migrating. Verify mTLS, traffic routing, and authorization policies work correctly.

#### Linkerd
- Linkerd uses a similar iptables-based traffic interception model
- Generally less affected than Istio due to simpler iptables requirements
- Same recommendation: verify in staging before production migration

#### General Service Mesh Considerations
- After migration, iptables rules injected by service mesh sidecars continue to work for pod-level traffic interception
- The absence of kube-proxy means that `iptables -t nat` rules for service resolution may behave differently
- eBPF TC hooks execute **before** iptables FORWARD chain processing — this can affect custom iptables rules

### 8.3 Existing Custom iptables Rules

| Rule Type | Post-Migration Behavior |
|---|---|
| **Node-level iptables rules** (DaemonSets) | Survive migration, but interaction with Cilium eBPF programs can be unpredictable (eBPF TC hooks execute before iptables FORWARD chain) |
| **Pod-level iptables rules** (init containers) | Continue to work within the pod's network namespace; Cilium enforcement is at the veth interface (outside the pod namespace) |
| **kube-proxy iptables rules** | **Completely removed.** Any tooling parsing these chains breaks. |
| **ip-masq-agent configuration** | May need adjustment; `azure-ip-masq-agent-config-reconciled` ConfigMap is auto-updated during migration |

### 8.4 Load Balancer and Service IP Preservation

| Component | Preserved? | Notes |
|---|---|---|
| Service ClusterIPs | Yes | Cilium takes over routing from kube-proxy; same Service objects |
| LoadBalancer external IPs | Yes | Managed by cloud-controller-manager, not CNI |
| NodePort allocations | Yes | Same Service objects, same NodePort numbers |
| `loadBalancerSourceRanges` | Yes (different enforcement point) | NPM: iptables on node; Cilium: eBPF — test explicitly |
| `externalTrafficPolicy` | **Behavioral change** | With Cilium, ingress policies are enforced on LB traffic (see Section 4.3) |

### 8.5 Cilium Identity Exhaustion

Cilium's identity limit is **65,535**. High-churn workloads (Spark jobs, batch processing, CI/CD) can exhaust this limit because each unique label set creates a new identity.

**Symptoms:** New pods fail to get an identity; policy enforcement breaks.

**Mitigation:** Exclude high-churn labels (e.g., `spark-app-name`, `spark-app-selector`, `job-name`) from identity computation. On managed AKS Cilium, this requires a support request or label exclusion annotations since customers cannot modify the Cilium configmap directly.

---

## 9. Observability Gaps During Migration

### 9.1 NPM Metrics vs Cilium/Hubble Metrics

| Metric Category | NPM | Cilium (no ACNS) | Cilium (with ACNS) |
|---|---|---|---|
| Policy rule counts | `npm_num_iptables_rules`, `npm_num_ipsets` | `cilium endpoint list` | Full Prometheus metrics |
| Flow visibility | None native | Basic `cilium monitor` | Full Hubble flow logs |
| DNS observability | None | None | `hubble_dns_queries_total`, `hubble_dns_responses_total` |
| Drop monitoring | iptables LOG rules | `cilium monitor --type drop` | `hubble_drop_total` metric with labels |
| Prometheus metrics | `npm_num_policies`, `npm_add_policy_exec_time` | Cilium agent metrics | Full Hubble + node metrics |
| Grafana dashboards | Custom/manual | Manual setup | Azure Managed Grafana integration |
| L7 visibility | None | None | HTTP/gRPC/Kafka flow data |

### 9.2 The Policy Enforcement Gap During Cutover

The migration documentation warns:

> "Cilium begins enforcing network policies only after **all nodes** are reimaged."

This creates a dangerous window:

1. **Phase 1:** `az aks update --network-dataplane cilium` is initiated
2. **Phase 2:** NPM is removed from nodes as they begin reimaging
3. **Phase 3:** Nodes are being reimaged (rolling within each pool, but all pools simultaneously)
4. **Phase 4:** Cilium starts enforcing only after ALL nodes complete reimaging

**During Phases 2-3, there is a window where NEITHER NPM NOR Cilium is enforcing policies.** This is a security exposure with no existing mitigation.

### 9.3 Post-Migration Validation Commands

```bash
# Check Cilium agent status on each node
kubectl -n kube-system exec -it <cilium-pod> -- cilium status

# List all Cilium endpoints and their policy enforcement status
kubectl -n kube-system exec -it <cilium-pod> -- cilium endpoint list

# Check if a specific endpoint has the correct policy
kubectl -n kube-system exec -it <cilium-pod> -- cilium endpoint get <endpoint-id>

# View the policy map for an endpoint (what is actually enforced in eBPF)
kubectl -n kube-system exec -it <cilium-pod> -- cilium bpf policy get <endpoint-id>

# Monitor policy verdicts in real time
kubectl -n kube-system exec -it <cilium-pod> -- cilium monitor --type policy-verdict

# Monitor drops in real-time
kubectl -n kube-system exec -it <cilium-pod> -- cilium monitor --type drop
```

### 9.4 Grafana Dashboard Migration

NPM-era custom dashboards based on iptables metrics become immediately irrelevant after migration. There is **no automated migration path for dashboards**. With ACNS enabled, Azure provides:

- Pre-built Container Network Observability dashboards
- Integration with Azure Monitor managed Prometheus
- Integration with Azure Managed Grafana
- Hubble metrics dashboards: DNS latency, flow rates, drop rates, TCP connection states

---

## 10. Community and Ecosystem

### 10.1 Cilium Ecosystem Tools

1. **Cilium Policy Audit Mode:** Cilium supports a "policy audit" mode where it logs policy decisions without enforcing. However, on AKS managed Cilium, custom configuration is not allowed — you cannot toggle audit mode unless using BYO CNI.

2. **Hubble (Observability):** Cilium's built-in observability platform. With ACNS enabled, provides flow visibility, DNS monitoring, and network metrics. Critical for post-migration validation.

3. **Cilium Network Policy Editor** (editor.cilium.io): Web-based visual tool for creating and validating CiliumNetworkPolicy resources.

4. **`cilium connectivity test`:** CLI command that validates end-to-end connectivity and policy enforcement. Can be run post-migration.

5. **Inspektor Gadget:** eBPF-based tool for Kubernetes that can trace network connections and policy decisions. Works well with Cilium.

6. **Netpol Analyzer** (IBM Research): Analyzes Kubernetes NetworkPolicy resources and computes effective connectivity between pods. Works on API objects.

### 10.2 Third-Party Migration Tools

As of February 2026, there are **no widely-adopted third-party tools** specifically designed to automate NPM-to-Cilium policy migration. The migration is primarily a manual process. Some community-contributed scripts exist in blog posts and GitHub gists, but none have reached production-grade maturity.

---

## 11. Tooling Gap Analysis — What We Can Build

### Tool 1: NPM Policy Analyzer / Pre-Migration Audit CLI (P0)

A CLI tool (or `kubectl` plugin) that scans all existing NetworkPolicy objects and produces a migration readiness report.

**What it checks:**
- `ipBlock` rules using CIDRs that overlap with pod/node/service CIDRs (the `0.0.0.0/0` problem)
- Named ports — flags any policy using named ports; cross-references with target pods to detect divergent port numbers
- `endPort` usage — checks against target Cilium version
- Egress policies that rely on implicit local-node allow
- Ingress policies on pods backing LoadBalancer/NodePort services with `externalTrafficPolicy=Cluster`
- Host-networked pods that have NetworkPolicies applied (enforcement will be lost)
- Policies referencing pod/node IPs via ipBlock instead of selectors
- Custom iptables rules on nodes (via DaemonSets, init containers)
- Service mesh sidecar detection (Istio, Linkerd) for kube-proxy dependency warnings
- High-churn labels that could cause identity exhaustion

**Output:** Per-policy risk assessment (PASS / WARN / FAIL) with specific remediation steps and links to documentation.

### Tool 2: Policy Translator / Patch Generator (P0)

Automatically generates the supplementary CiliumNetworkPolicy objects and Kubernetes NetworkPolicy patches needed to maintain behavioral equivalence.

**What it generates:**
- For each `ipBlock: cidr: 0.0.0.0/0` egress rule: adds `namespaceSelector: {}` + `podSelector: {}` peers, plus a CiliumNetworkPolicy with `toEntities: [host, remote-node]`
- For each egress-restricted pod: CiliumNetworkPolicy allowing egress to `host` entity (local node)
- For each ingress-restricted pod behind a LoadBalancer: explicit ingress allow rules
- Replaces all named ports with numeric values (by querying live pod specs)
- For `endPort` ranges on older Cilium: expands to individual port entries

**Output format:** Directory of YAML files, ready to `kubectl apply`. Can also output a Kustomize overlay or Helm values patch.

### Tool 3: Pre/Post Migration Connectivity Validator (P1)

A test harness that captures the "ground truth" of connectivity before migration and validates it after.

**Pre-migration (snapshot phase):**
- Deploys ephemeral test pods in each namespace
- Tests pod-to-pod, pod-to-service, pod-to-external, external-to-service, pod-to-node connectivity
- Records which connections succeed/fail under current NPM enforcement
- Captures DNS resolution behavior
- Saves results as a connectivity matrix JSON

**Post-migration (validation phase):**
- Re-runs the same connectivity tests against the same targets
- Diffs results against the pre-migration snapshot
- Flags any regressions (connection that worked before but fails now, or vice versa)
- Integrates with Hubble flow logs (if ACNS enabled) to show exactly why a connection was dropped

### Tool 4: Fleet-Scale Discovery and Prioritization (P1)

For customers with many clusters, a tool to scan across subscriptions and prioritize migration order.

**Capabilities:**
- Uses Azure Resource Graph to discover all NPM clusters
- Analyzes each cluster: K8s version, node count, Windows pool presence, policy count, service mesh detection
- Prioritizes: clusters without Windows pools, on newer K8s versions, with fewer/simpler policies go first
- Clusters with complex policies, Windows pools, or service meshes flagged for manual review
- Outputs a prioritized migration plan with estimated risk levels

### Tool 5: Migration Orchestrator (P2)

A higher-level workflow tool that wraps the raw `az aks update` commands with safety rails.

**Capabilities:**
- Pre-flight checks: validates all prerequisites (K8s version, no Windows pools, CLI version, PDBs in place)
- Runs Tool 1 (audit) and blocks migration if FAIL-level issues exist
- Runs Tool 3 (connectivity snapshot) before migration
- Executes the `az aks update --network-dataplane cilium` command
- Monitors node reimage progress (polls node status)
- Applies Tool 2 (generated CiliumNetworkPolicy patches) after all nodes are reimaged
- Runs Tool 3 (connectivity validation) after migration
- Generates a migration report (before/after diff, policy changes applied, any regressions)
- Optionally enables ACNS for observability

### Tool 6: Live Policy Diff Dashboard (P2)

A Grafana dashboard (or CLI tool) for real-time migration monitoring:

- NPM iptables rule count (decreasing as nodes are reimaged)
- Cilium endpoint count (increasing as nodes come up with Cilium)
- Policy enforcement status per node (NPM vs Cilium vs none)
- Hubble flow logs showing drops with policy verdict
- Side-by-side comparison of NPM metrics vs Cilium/Hubble metrics

---

## 12. Proposed Project Structure

```
npmmigration/
├── cmd/
│   ├── npm-audit/              # Tool 1: Policy Analyzer CLI
│   │   └── main.go
│   ├── npm-translate/          # Tool 2: Policy Translator
│   │   └── main.go
│   ├── npm-conntest/           # Tool 3: Connectivity Validator
│   │   └── main.go
│   └── npm-migrate/            # Tool 4: Migration Orchestrator
│       └── main.go
├── pkg/
│   ├── policy/
│   │   ├── analyzer.go         # Core policy analysis logic
│   │   ├── translator.go       # NPM-to-Cilium policy translation
│   │   ├── rules.go            # Breaking change detection rules
│   │   └── types.go            # Shared types
│   ├── connectivity/
│   │   ├── prober.go           # Connectivity test probe deployment
│   │   ├── matrix.go           # Connectivity matrix capture/diff
│   │   └── reporter.go         # Results reporting
│   ├── cluster/
│   │   ├── discovery.go        # Azure Resource Graph queries
│   │   ├── prereqs.go          # Pre-flight checks
│   │   └── monitor.go          # Migration progress monitoring
│   └── cilium/
│       ├── policy.go           # CiliumNetworkPolicy generation
│       ├── hubble.go           # Hubble integration
│       └── health.go           # Cilium health checks
├── dashboards/
│   └── migration-monitor.json  # Grafana dashboard definition
├── test/
│   ├── policies/               # Sample NetworkPolicies for testing
│   │   ├── ipblock-catch-all.yaml
│   │   ├── named-ports.yaml
│   │   ├── endport-range.yaml
│   │   ├── deny-all-ingress.yaml
│   │   ├── deny-all-egress.yaml
│   │   └── complex-selectors.yaml
│   └── e2e/                    # End-to-end migration tests
│       ├── setup_cluster.sh
│       ├── migrate_test.go
│       └── validate_test.go
├── docs/
│   └── migration-research-report.md  # This document
├── go.mod
└── go.sum
```

**Language:** Go — aligns with Kubernetes ecosystem, Azure SDK, and Cilium codebase.

**Key dependencies:**
- `k8s.io/client-go` — Kubernetes API access
- `github.com/Azure/azure-sdk-for-go` — Azure Resource Graph, AKS management
- `github.com/cilium/cilium/pkg/policy` — Cilium policy types
- `sigs.k8s.io/network-policy-api` — NetworkPolicy types
- `github.com/spf13/cobra` — CLI framework

---

## 13. Implementation Priority and Roadmap

| Priority | Tool | Effort Estimate | Customer Impact | Justification |
|---|---|---|---|---|
| **P0** | Policy Analyzer (audit) | 2-3 weeks | Prevents silent breakage | #1 customer complaint; no existing tooling |
| **P0** | Policy Translator (patch gen) | 2 weeks | Directly remediates issues | Automates the fixes the analyzer finds |
| **P1** | Connectivity Validator | 2-3 weeks | Gives migration confidence | Fills the gap left by unavailable Cilium audit mode |
| **P1** | Fleet Discovery | 1 week | Helps enterprise planning | Simple ARG queries + prioritization logic |
| **P2** | Migration Orchestrator | 3-4 weeks | Automates full workflow | Reduces human error; wraps all other tools |
| **P2** | Live Dashboard | 1-2 weeks | Operational visibility | Grafana JSON + Prometheus queries |

### Suggested Milestone Plan

**Milestone 1 (Weeks 1-5): Core Safety Net**
- Policy Analyzer CLI (P0)
- Policy Translator (P0)
- Basic test policy suite

**Milestone 2 (Weeks 6-9): Validation Layer**
- Connectivity Validator (P1)
- Fleet Discovery tool (P1)
- E2E test infrastructure

**Milestone 3 (Weeks 10-14): Full Automation**
- Migration Orchestrator (P2)
- Live Dashboard (P2)
- Documentation and customer-facing guides

---

## 14. Key Technical Risks

### Risk 1: No Audit Mode on Managed Cilium
AKS doesn't expose Cilium's `policy-audit-mode` config flag. We can't run Cilium in "log-only" mode before enforcing. **Mitigation:** Our connectivity validator (Tool 3) must fill this gap by capturing ground truth before migration.

### Risk 2: Policy Enforcement Gap During Reimage
Between first and last node reimaging, enforcement is inconsistent. Neither NPM nor Cilium enforces during the transition. **Mitigation:** Our orchestrator should warn customers about this window, recommend maintenance windows, and verify no security-sensitive workloads are exposed.

### Risk 3: No Rollback
If migration breaks things, the only option is a new cluster. **Mitigation:** Our pre-migration validation (Tools 1 + 3) must be extremely thorough to prevent this scenario. The analyzer should block migration if FAIL-level issues exist.

### Risk 4: ACNS Cost Dependency
Hubble observability (critical for post-migration debugging) requires the paid ACNS add-on. Customers without ACNS have limited debugging capability. **Mitigation:** Our tools should work both with and without ACNS, using basic `cilium monitor` commands as fallback.

### Risk 5: Identity Churn Exhaustion
High-churn workloads (Spark, batch jobs) can exhaust Cilium's 65K identity limit. **Mitigation:** The analyzer should flag labels that cause identity explosion and recommend exclusions. Include this in the pre-flight check.

### Risk 6: Service Mesh Compatibility
Istio and Linkerd depend on kube-proxy iptables rules that are removed by Cilium. **Mitigation:** The analyzer should detect service mesh sidecars and warn about potential compatibility issues. Recommend staging cluster validation.

---

## 15. Migration Playbook Summary

### Pre-Migration Checklist

- [ ] Run Policy Analyzer (Tool 1) — resolve all FAIL-level issues
- [ ] Run Policy Translator (Tool 2) — generate all remediation patches
- [ ] Apply NetworkPolicy patches (ipBlock fixes, named port replacements)
- [ ] Verify prerequisites: Azure CLI >= 2.52.0, no Windows pools, K8s >= 1.29
- [ ] Set PodDisruptionBudgets on all critical workloads
- [ ] Run Connectivity Validator snapshot (Tool 3)
- [ ] Test in staging cluster first
- [ ] Schedule maintenance window for the enforcement gap
- [ ] Notify dependent teams (security, monitoring, on-call)

### Migration Execution

```bash
# 1. (Optional) Migrate IPAM to Overlay
az aks update --name $CLUSTER --resource-group $RG \
  --network-plugin-mode overlay --pod-cidr 192.168.0.0/16

# 2. Switch dataplane to Cilium
az aks update --name $CLUSTER --resource-group $RG \
  --network-dataplane cilium

# 3. Monitor node reimaging
kubectl get nodes -w

# 4. Apply post-migration CiliumNetworkPolicy objects
kubectl apply -f ./generated-cilium-policies/

# 5. Validate connectivity
npm-conntest validate --snapshot ./pre-migration-snapshot.json

# 6. (Optional) Enable ACNS
az aks update --name $CLUSTER --resource-group $RG \
  --enable-acns
```

### Post-Migration Verification

- [ ] Verify Cilium health: `kubectl get pods -n kube-system -l k8s-app=cilium`
- [ ] Run Connectivity Validator post-migration diff (Tool 3)
- [ ] Check for dropped traffic: `cilium monitor --type drop`
- [ ] Validate policy enforcement: `cilium endpoint list`
- [ ] Update monitoring dashboards to Cilium/Hubble metrics
- [ ] Clean up NPM-specific configuration and monitoring
- [ ] Update runbooks and documentation

---

## 16. Sources

| # | Source | URL | Updated |
|---|---|---|---|
| 1 | Configure Azure CNI Powered by Cilium in AKS | https://learn.microsoft.com/en-us/azure/aks/azure-cni-powered-by-cilium | Jan 5, 2026 |
| 2 | Migrate from NPM to Cilium Network Policy | https://learn.microsoft.com/en-us/azure/aks/migrate-from-npm-to-cilium-network-policy | Feb 17, 2026 |
| 3 | Secure Pod Traffic with Network Policies in AKS | https://learn.microsoft.com/en-us/azure/aks/use-network-policies | Feb 17, 2026 |
| 4 | Update Azure CNI IPAM Mode and Data Plane Technology | https://learn.microsoft.com/en-us/azure/aks/update-azure-cni | Dec 4, 2025 |
| 5 | CNI Networking Concepts in AKS | https://learn.microsoft.com/en-us/azure/aks/concepts-network-cni-overview | Dec 3, 2025 |
| 6 | Azure CNI Overlay Networking Overview | https://learn.microsoft.com/en-us/azure/aks/concepts-network-azure-cni-overlay | Jan 15, 2026 |
| 7 | Advanced Container Networking Services Overview | https://learn.microsoft.com/en-us/azure/aks/advanced-container-networking-services-overview | Jan 6, 2026 |
| 8 | Configure Azure CNI Networking | https://learn.microsoft.com/en-us/azure/aks/configure-azure-cni | Jan 29, 2026 |
| 9 | NPM Windows Retirement Announcement | https://azure.microsoft.com/updates?id=500273 | — |
| 10 | NPM Linux Retirement Announcement | https://azure.microsoft.com/updates?id=500268 | — |
| 11 | Cilium Issue #14287 — Multiple services sharing host port | https://github.com/cilium/cilium/issues/14287 | — |
| 12 | Cilium Issue #30003 — Named ports inconsistency | https://github.com/cilium/cilium/issues/30003 | — |
| 13 | Cilium Documentation — Identity-Relevant Labels | https://docs.cilium.io/en/stable/operations/performance/scalability/identity-relevant-labels/ | — |

---

*End of Report*
