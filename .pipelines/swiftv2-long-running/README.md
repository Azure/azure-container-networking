# SwiftV2 Long-Running Pipeline

This pipeline tests SwiftV2 pod networking in a persistent environment with scheduled test runs.

## Architecture Overview

**Infrastructure (Persistent)**:
- **2 AKS Clusters**: aks-1, aks-2 (4 nodes each: 2 low-NIC default pool, 2 high-NIC nplinux pool)
- **4 VNets**: cx_vnet_v1, cx_vnet_v2, cx_vnet_v3 (Customer 1 with PE to storage), cx_vnet_v4 (Customer 2)
- **VNet Peerings**: Full mesh between the three Customer 1 VNets; Customer 2 remains isolated
- **2 Storage Accounts**: One with a private endpoint in cx_vnet_v1 and one reserved for future scenarios
- **NSGs**: Restricting traffic between subnets (s1, s2) in vnet cx_vnet_v1.
- **Node Labels**: All nodes labeled with `workload-type` and `nic-capacity` for targeted test execution


**Node Labeling for Multiple Workload Types**:
Each managed or BYON node pool gets its designated workload label during setup:

| Node type | `workload-type` |
|-----------|-----------------|
| Managed nodepool1 and nplinux | `swiftv2-linux` |
| Linux BYON | `swiftv2-linux-byon` |
| Accelerated-networking BYON | `swiftv2-l1vh-accelnet-byon` |

## How It Works

### Scheduled Test Flow
Every scheduled run, the pipeline:
1. Runs the idempotent infrastructure stage for every subscription/region scenario. Existing resources are verified and reused by the setup scripts.
2. Runs persistent rotating-pod, always-on DaemonSet, and connectivity jobs for every configured long-running zone.
3. Acquires a per-scenario ConfigMap lease for the ephemeral datapath stages.
4. For each enabled workload type:
   - Creates 8 PodNetwork/PNI/pod scenarios.
   - Runs 8 pod-to-pod connectivity tests.
   - Runs 4 Private Endpoint tests.
   - Runs the scale create/delete test.
   - Deletes the ephemeral test resources.
5. Releases the datapath lease even when a datapath stage fails.


## Test Case Details

### 8 Pod Scenarios (Created in Job 1)

All test scenarios create the following resources:
- **PodNetwork**: Defines the network configuration for a VNet/subnet combination
- **PodNetworkInstance**: Instance-level configuration with IP allocation
- **Pod**: Test pod running nicolaka/netshoot with TCP netcat listener on port 8080

| # | Scenario | Cluster | VNet | Subnet | Node Type | Pod Name | Purpose |
|---|----------|---------|------|--------|-----------|----------|---------|
| 1 | Customer2-AKS2-VnetV4-S1-LowNic | aks-2 | cx_vnet_v4 | s1 | low-nic | pod-c2-aks2-v4s1-low | Tenant B pod for isolation testing |
| 2 | Customer2-AKS2-VnetV4-S1-HighNic | aks-2 | cx_vnet_v4 | s1 | high-nic | pod-c2-aks2-v4s1-high | Tenant B pod on high-NIC node |
| 3 | Customer1-AKS1-VnetV1-S1-LowNic | aks-1 | cx_vnet_v1 | s1 | low-nic | pod-c1-aks1-v1s1-low | Tenant A pod in NSG-protected subnet |
| 4 | Customer1-AKS1-VnetV1-S2-LowNic | aks-1 | cx_vnet_v1 | s2 | low-nic | pod-c1-aks1-v1s2-low | Tenant A pod for NSG isolation test |
| 5 | Customer1-AKS1-VnetV1-S2-HighNic | aks-1 | cx_vnet_v1 | s2 | high-nic | pod-c1-aks1-v1s2-high | Tenant A pod on high-NIC node |
| 6 | Customer1-AKS1-VnetV2-S1-HighNic | aks-1 | cx_vnet_v2 | s1 | high-nic | pod-c1-aks1-v2s1-high | Tenant A pod in peered VNet |
| 7 | Customer1-AKS2-VnetV2-S1-LowNic | aks-2 | cx_vnet_v2 | s1 | low-nic | pod-c1-aks2-v2s1-low | Cross-cluster same VNet test |
| 8 | Customer1-AKS2-VnetV3-S1-HighNic | aks-2 | cx_vnet_v3 | s1 | high-nic | pod-c1-aks2-v3s1-high | Private endpoint access test |

### Connectivity Tests (8 Test Cases)

Tests TCP connectivity between pods using netcat with 3-second timeout:

**Expected to SUCCEED (4 tests)**:

| Test | Source → Destination | Validation | Purpose |
|------|---------------------|------------|---------|
| SameVNetSameSubnet | pod-c1-aks1-v1s2-low → pod-c1-aks1-v1s2-high | TCP Connected | Basic same-subnet connectivity |
| DifferentVNetSameCustomer | pod-c1-aks1-v2s1-high → pod-c1-aks2-v2s1-low | TCP Connected | Cross-cluster, same VNet (v2) |
| PeeredVNets | pod-c1-aks1-v1s2-low → pod-c1-aks1-v2s1-high | TCP Connected | VNet peering (v1 ↔ v2) |
| PeeredVNets_v2tov3 | pod-c1-aks1-v2s1-high → pod-c1-aks2-v3s1-high | TCP Connected | VNet peering across clusters |

**Expected to FAIL (4 tests)**:

| Test | Source → Destination | Expected Error | Purpose |
|------|---------------------|----------------|---------|
| NSGBlocked_S1toS2 | pod-c1-aks1-v1s1-low → pod-c1-aks1-v1s2-high | Connection timeout | NSG blocks s1→s2 in cx_vnet_v1 |
| NSGBlocked_S2toS1 | pod-c1-aks1-v1s2-low → pod-c1-aks1-v1s1-low | Connection timeout | NSG blocks s2→s1 (bidirectional) |
| DifferentCustomers_V1toV4 | pod-c1-aks1-v1s2-low → pod-c2-aks2-v4s1-low | Connection timeout | Customer isolation (no peering) |
| DifferentCustomers_V2toV4 | pod-c1-aks1-v2s1-high → pod-c2-aks2-v4s1-high | Connection timeout | Customer isolation (no peering) |

**NSG Rules Configuration**:
- cx_vnet_v1 has NSG rules blocking traffic between s1 and s2 subnets:
  - Deny outbound from s1 to s2 (priority 100)
  - Deny inbound from s1 to s2 (priority 110)
  - Deny outbound from s2 to s1 (priority 100)
  - Deny inbound from s2 to s1 (priority 110)

### Private Endpoint Tests (4 Test Cases)

Tests access to an Azure Storage account through its Private Endpoint:

**Expected to SUCCEED (4 tests)**:

| Test | Source → Storage | Validation | Purpose |
|------|-----------------|------------|---------|
| TenantA_VNetV1_S1_to_StorageA | pod-c1-aks1-v1s1-low → Storage-A | Blob download via SAS | Access via private endpoint from VNet V1 |
| TenantA_VNetV1_S2_to_StorageA | pod-c1-aks1-v1s2-low → Storage-A | Blob download via SAS | Access via private endpoint from VNet V1 |
| TenantA_VNetV2_to_StorageA | pod-c1-aks1-v2s1-high → Storage-A | Blob download via SAS | Access via peered VNet (V2 peered with V1) |
| TenantA_VNetV3_to_StorageA | pod-c1-aks2-v3s1-high → Storage-A | Blob download via SAS | Access via peered VNet from different cluster |

**Private Endpoint Configuration**:
- Private endpoint created in cx_vnet_v1 subnet 'pe'
- Private DNS zone `privatelink.blob.core.windows.net` linked to:
  - cx_vnet_v1, cx_vnet_v2, cx_vnet_v3 (Tenant A VNets)
  - aks-1 and aks-2 cluster VNets
- Storage Account 1 (Tenant A):
  - Shared key access: Disabled (Azure AD only)
  - Blob public access: Disabled
- Storage Account 2: Provisioned with the same account-level security settings for future tests; it has no Private Endpoint test

**Test Flow**:
1. DNS resolution: Storage FQDN resolves through the Private DNS zone
2. Generate SAS token: Azure AD authentication via management plane
3. Download blob: Using wget with SAS token via data plane
4. Validation: Verify blob content matches expected value

### Resource Creation Patterns

**Naming Convention**:
```
BUILD_ID = <resourceGroupName>

PodNetwork:         pn-<BUILD_ID>-<vnet>-<subnet>
PodNetworkInstance: pni-<BUILD_ID>-<vnet>-<subnet>
Namespace:          pn-<BUILD_ID>-<vnet>-<subnet>
Pod:                pod-<scenario-suffix>
```

**Example** (for `resourceGroupName=sv2-long-run-centraluseuap`):
```
pn-sv2-long-run-centraluseuap-v1-s1
pni-sv2-long-run-centraluseuap-v1-s1
pn-sv2-long-run-centraluseuap-v1-s1 (namespace)
pod-c1-aks1-v1s1-low
```

**VNet Name Simplification**:
- `cx_vnet_v1` → `v1`
- `cx_vnet_v2` → `v2`
- `cx_vnet_v3` → `v3`
- `cx_vnet_v4` → `v4`


## Node Pool Configuration

### Node Labels and Architecture

All nodes in the clusters are labeled with two key labels for workload identification and NIC capacity. These labels are applied during cluster creation by the `create_aks.sh` script.

**1. Workload Type Label** (`workload-type`):
- Purpose: Identifies which test scenario group the node belongs to
- Managed node value: `swiftv2-linux`
- Applied during: Infrastructure and BYON setup
- Applied by: `infrastructure-setup-stage.yaml`, `deploy_linuxbyon.sh`, and `deploy_accelnetbyon.sh`
- Future use: Supports multiple workload types running as separate stages (e.g., `swiftv2-windows`, `swiftv2-byonodeid`)
- Stage isolation: Each test stage uses `WORKLOAD_TYPE` environment variable to filter nodes

**2. NIC Capacity Label** (`nic-capacity`):
- Purpose: Identifies the NIC capacity tier of the node
- Applied during: The `EnsureNodeLabels` infrastructure job
- Applied by: `.pipelines/swiftv2-long-running/template/infrastructure-setup-stage.yaml`
- Values:
  - `low-nic`: Default nodepool (nodepool1) with `Standard_D4s_v3` (1 NIC)
  - `high-nic`: NPLinux nodepool (nplinux) with `Standard_D16s_v3` (7 NICs)

**Managed Node Label Application**:
```bash
# Step 1: All nodes get workload-type label
kubectl label nodes --all workload-type=swiftv2-linux --overwrite

# Step 2: Default nodepool gets low-nic capacity label
kubectl label nodes -l agentpool=nodepool1 nic-capacity=low-nic --overwrite

# Step 3: NPLinux nodepool gets high-nic capacity label  
kubectl label nodes -l agentpool=nplinux nic-capacity=high-nic --overwrite
```

### Node Selection in Tests

Tests use these labels to select appropriate nodes dynamically:
- **Function**: `GetNodesByNicCount()` in `test/integration/swiftv2/longRunningCluster/datapath.go`
- **Filtering**: Nodes filtered by BOTH `workload-type` AND `nic-capacity` labels
- **Environment Variable**: `WORKLOAD_TYPE` (set by each test stage) determines which nodes are used
  - Managed datapath and long-running stages use `WORKLOAD_TYPE=swiftv2-linux`
  - Future: Different values for each stage (e.g., `swiftv2-byonodeid`, `swiftv2-windows`)
- **Selection Logic**:
  ```go
  // Get low-nic nodes with matching workload type
  kubectl get nodes -l "nic-capacity=low-nic,workload-type=$WORKLOAD_TYPE"
  
  // Get high-nic nodes with matching workload type
  kubectl get nodes -l "nic-capacity=high-nic,workload-type=$WORKLOAD_TYPE"
  ```
- **Pod Assignment**: 
  - Low-NIC nodes: Limited to 1 pod per node
  - High-NIC nodes: Currently limited to 1 pod per node in test logic

**Node Pool Configuration**:

| Node Pool | VM SKU | NICs | Label | Pods per Node |
|-----------|--------|------|-------|---------------|
| nodepool1 (default) | `Standard_D4s_v3` | 1 | `nic-capacity=low-nic` | 1 |
| nplinux | `Standard_D16s_v3` | 7 | `nic-capacity=high-nic` | 1 (current test logic) |

**Note**: VM SKUs are hardcoded as constants in the pipeline template and cannot be changed by users.

## File Structure

```
.pipelines/swiftv2-long-running/
├── pipeline.yaml                              # Main pipeline with schedule and scenario matrix
├── README.md                                # Datapath pipeline documentation
├── LONGRUNNING-TESTS.md                     # Persistent zonal test documentation
├── template/
│   ├── long-running-pipeline-template.yaml  # Top-level stage composition
│   ├── infrastructure-setup-stage.yaml      # Regional infrastructure and BYON setup
│   ├── datapath-tests-stage.yaml            # Ephemeral datapath test jobs
│   ├── long-running-pod-tests-stage.yaml    # Persistent per-zone test jobs
│   └── metrics-setup-steps.yaml             # Shared metrics artifact setup
└── scripts/
    ├── create_aks.sh                         # AKS cluster creation
    ├── create_vnets.sh                       # VNet and subnet creation
    ├── create_peerings.sh                    # VNet peering setup
    ├── create_storage.sh                     # Storage account creation
    ├── create_nsg.sh                         # Network security groups
    ├── create_pe.sh                          # Private endpoint setup
    ├── ensure_zone_nodepools.sh              # Persistent zonal node pools
    ├── acquire_pipeline_lease.sh             # Datapath lease acquisition
    ├── release_pipeline_lease.sh             # Datapath lease release
    └── lock_resource_groups.sh               # Resource-group locks and lifetime tags

test/integration/swiftv2/longRunningCluster/
├── datapath_create_test.go                   # Create the 8 ephemeral pod scenarios
├── datapath_connectivity_test.go             # Run the 8 TCP connectivity tests
├── datapath_private_endpoint_test.go         # Run the 4 Private Endpoint tests
├── datapath_scale_test.go                    # Create and delete pods at scale
├── datapath_delete_test.go                   # Delete ephemeral test resources
├── datapath_longrunning_rotating_test.go     # Rotate persistent deployments
├── datapath_longrunning_alwayson_test.go     # Ensure the persistent DaemonSet
├── datapath_longrunning_connectivity_test.go # Test persistent pod connectivity
├── datapath_longrunning_shared.go            # Shared persistent-test helpers
└── datapath.go                               # Resource orchestration and common probes
```
