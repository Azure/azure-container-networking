# SwiftV2 Long-Running Pipeline

This pipeline tests SwiftV2 pod networking in a persistent environment with scheduled test runs.

## Architecture Overview

**Infrastructure (Persistent)**:
- **2 AKS Clusters**: aks-1, aks-2 (4 nodes each: 2 low-NIC default pool, 2 high-NIC nplinux pool)
- **4 VNets**: cx_vnet_a1, cx_vnet_a2, cx_vnet_a3 (Customer 1 with PE to storage), cx_vnet_b1 (Customer 2)
- **VNet Peerings**: vnet mesh.
- **Storage Account**: With private endpoint from cx_vnet_a1
- **NSGs**: Restricting traffic between subnets (s1, s2) in vnet cx_vnet_a1.
- **Node Labels**: All nodes labeled with `workload-type` and `nic-capacity` for targeted test execution

**Test Scenarios (8 total per workload type)**:
- Multiple pods across 2 clusters, 4 VNets, different subnets (s1, s2), and node types (low-NIC, high-NIC)
- Each test run: Create all resources → Wait 20 minutes → Delete all resources
- Tests run automatically every 1 hour via scheduled trigger

**Multi-Stage Workload Testing**:
- Tests are organized by workload type using node label `workload-type`
- Each workload type runs as a separate stage sequentially
- Current implementation: `swiftv2-linux` (Stage: ManagedNodeDataPathTests)
- Future stages can be added for different workload types (e.g., `swiftv2-l1vhaccelnet`, `swiftv2-linuxbyon`)
- Each stage uses the same infrastructure but targets different labeled nodes

## Pipeline Modes

### Resource Group Naming Conventions

The pipeline uses **strict naming conventions** for resource groups to ensure proper organization and lifecycle management:

**1. Production Scheduled Runs (Master/Main Branch)**:
```
Pattern: sv2-long-run-<region>
Examples: sv2-long-run-centraluseuap, sv2-long-run-eastus, sv2-long-run-westus2
```
- **When to use**: Creating infrastructure for scheduled automated tests on master/main branch
- **Purpose**: Long-running persistent infrastructure for continuous validation
- **Lifecycle**: Persistent (tagged with `SkipAutoDeleteTill=2032-12-31`)
- **Example**: If running scheduled tests in Central US EUAP region, use `sv2-long-run-centraluseuap`

**2. Test/Development/PR Validation Runs**:
```
Pattern: sv2-long-run-$(Build.BuildId)
Examples: sv2-long-run-12345, sv2-long-run-67890
```
- **When to use**: Temporary testing, one-time validation, or PR testing
- **Purpose**: Short-lived infrastructure for specific test runs
- **Lifecycle**: Can be cleaned up after testing completes
- **Example**: PR validation run with Build ID 12345 → `sv2-long-run-12345`

**Important Notes**:
-  Always follow the naming pattern for scheduled runs on master: `sv2-long-run-<region>`
-  Do not use build IDs for production scheduled infrastructure (it breaks continuity)
-  All resource names within the setup use the resource group name as BUILD_ID prefix


### Mode 2: Initial Setup or Rebuild
**Trigger**: Manual run with parameter change  
**Purpose**: Create new infrastructure or rebuild existing  
**Setup Stages**: Enabled via `runSetupStages: true`  
**Resource Group**: Must follow naming conventions (see below)

**To create new infrastructure for scheduled runs on master branch**:
1. Go to Pipeline → Run pipeline
2. Set `runSetupStages` = `true`
3. Set `resourceGroupName` = `sv2-long-run-<region>` (e.g., `sv2-long-run-centraluseuap`)
   - **Critical**: Use this exact naming pattern for production scheduled tests
   - Region should match the `location` parameter
4. Optionally adjust `location` to match your resource group name
5. Run pipeline

**To create new infrastructure for testing/development**:
1. Go to Pipeline → Run pipeline
2. Set `runSetupStages` = `true`
3. Set `resourceGroupName` = `sv2-long-run-$(Build.BuildId)` or custom name
   - For temporary testing: Use build ID pattern for auto-cleanup
   - For parallel environments: Use descriptive suffix (e.g., `sv2-long-run-centraluseuap-dev`)
4. Optionally adjust `location`
5. Run pipeline

## Pipeline Parameters

Parameters are organized by usage:

### Common Parameters (Always Relevant)
| Parameter | Default | Description |
|-----------|---------|-------------|
| `location` | `centraluseuap` | Azure region for resources. Auto-generates RG name: `sv2-long-run-<location>`. |
| `runSetupStages` | `false` | Set to `true` to create new infrastructure. `false` for scheduled test runs. |
| `subscriptionId` | `37deca37-...` | Azure subscription ID. |
| `serviceConnection` | `Azure Container Networking...` | Azure DevOps service connection. |

### Setup-Only Parameters (Only Used When runSetupStages=true)

| Parameter | Default | Description |
|-----------|---------|-------------|
| `resourceGroupName` | `""` (empty) | **Leave empty** to auto-generate based on usage pattern. See Resource Group Naming Conventions below. |

**Note**: VM SKUs are hardcoded as constants in the pipeline template:
- Default nodepool: `Standard_D4s_v3` (low-nic capacity, 1 NIC)
- NPLinux nodepool: `Standard_D16s_v3` (high-nic capacity, 7 NICs)

Setup-only parameters are ignored when `runSetupStages=false` (scheduled runs).

## Pipeline Stage Organization

The pipeline is organized into stages based on workload type, allowing sequential testing of different node configurations using the same infrastructure.

### Stage 1: AKS Cluster and Networking Setup (Conditional)
**Runs when**: `runSetupStages=true`  
**Purpose**: One-time infrastructure creation  
**Creates**: AKS clusters, VNets, peerings, storage accounts, NSGs, private endpoints, node labels

### Stage 2: ManagedNodeDataPathTests (Current)
**Workload Type**: `swiftv2-linux`  
**Node Label Filter**: `workload-type=swiftv2-linux`  
**Jobs**:
1. Create Test Resources (8 pod scenarios)
2. Connectivity Tests (9 test cases)
3. Private Endpoint Tests (5 test cases)
4. Delete Test Resources (cleanup)

**Node Selection**:
- Tests automatically filter nodes by `workload-type=swiftv2-linux` AND `nic-capacity` labels
- Environment variable `WORKLOAD_TYPE=swiftv2-linux` is set for this stage
- Ensures tests only run on nodes designated for this workload type

### Future Stages (Planned Architecture)
Additional stages can be added to test different workload types sequentially:

**Example: Stage 3 - LinuxBYONodeDataPathTests**
```yaml
- stage: LinuxBYONodeDataPathTests
  displayName: "SwiftV2 Data Path Tests - BYO Node ID"
  dependsOn: ManagedNodeDataPathTests
  variables:
    WORKLOAD_TYPE: "swiftv2-linuxbyon"
  # Same job structure as ManagedNodeDataPathTests
  # Tests run on nodes labeled: workload-type=swiftv2-byonodeid
```

**Example: Stage 4 - L1vhAccelnetNodeDataPathTests**
```yaml
- stage: L1vhAccelnetNodeDataPathTests
  displayName: "SwiftV2 Data Path Tests - Windows Nodes Accelnet"
  dependsOn: BYONodeDataPathTests
  variables:
    WORKLOAD_TYPE: "swiftv2-windows"
  # Same job structure
  # Tests run on nodes labeled: workload-type=swiftv2-windows
```

**Node Labeling for Multiple Workload Types**:
Each node pool gets labeled with its designated workload type during setup:
```bash
# During cluster creation or node pool addition:
kubectl label nodes -l  workload-type=swiftv2-linux
kubectl label nodes -l  workload-type=swiftv2-linuxbyon
kubectl label nodes -l  workload-type=swiftv2-l1vhaccelnet
kubectl label nodes -l  workload-type=swiftv2-l1vhib
```

## How It Works

### Scheduled Test Flow
Every 3 hour, the pipeline:
1. Skips setup stages (infrastructure already exists)
2. **Job 1 - Create Resources**: Creates 8 test scenarios (PodNetwork, PNI, Pods with HTTP servers on port 8080)
3. **Job 2 - Connectivity Tests**: Tests HTTP connectivity between pods (9 test cases), then waits 20 minutes
4. **Job 3 - Private Endpoint Tests**: Tests private endpoint access and tenant isolation (5 test cases)
5. **Job 4 - Delete Resources**: Deletes all test resources (Phase 1: Pods, Phase 2: PNI/PN/Namespaces)
6. Reports results

**Connectivity Tests (9 scenarios)**:

| Test | Source → Destination | Expected Result | Purpose |
|------|---------------------|-----------------|---------|
| SameVNetSameSubnet | pod-c1-aks1-a1s2-low → pod-c1-aks1-a1s2-high | ✓ Success | Basic connectivity in same subnet |
| NSGBlocked_S1toS2 | pod-c1-aks1-a1s1-low → pod-c1-aks1-a1s2-high | ✗ Blocked | NSG rule blocks s1→s2 in cx_vnet_a1 |
| NSGBlocked_S2toS1 | pod-c1-aks1-a1s2-low → pod-c1-aks1-a1s1-low | ✗ Blocked | NSG rule blocks s2→s1 (bidirectional) |
| DifferentVNetSameCustomer | pod-c1-aks1-a2s1-high → pod-c1-aks2-a2s1-low | ✓ Success | Cross-cluster, same customer VNet |
| PeeredVNets | pod-c1-aks1-a1s2-low → pod-c1-aks1-a2s1-high | ✓ Success | Peered VNets (a1 ↔ a2) |
| PeeredVNets_A2toA3 | pod-c1-aks1-a2s1-high → pod-c1-aks2-a3s1-high | ✓ Success | Peered VNets across clusters |
| DifferentCustomers_A1toB1 | pod-c1-aks1-a1s2-low → pod-c2-aks2-b1s1-low | ✗ Blocked | Customer isolation (C1 → C2) |
| DifferentCustomers_A2toB1 | pod-c1-aks1-a2s1-high → pod-c2-aks2-b1s1-high | ✗ Blocked | Customer isolation (C1 → C2) |

**Test Results**: 4 should succeed, 5 should be blocked (3 NSG rules + 2 customer isolation)

**Private Endpoint Tests (5 scenarios)**:

| Test | Source → Destination | Expected Result | Purpose |
|------|---------------------|-----------------|---------|
| TenantA_VNetA1_S1_to_StorageA | pod-c1-aks1-a1s1-low → Storage-A | ✓ Success | Tenant A pod can access Storage-A via private endpoint |
| TenantA_VNetA1_S2_to_StorageA | pod-c1-aks1-a1s2-low → Storage-A | ✓ Success | Tenant A pod can access Storage-A via private endpoint |
| TenantA_VNetA2_to_StorageA | pod-c1-aks1-a2s1-high → Storage-A | ✓ Success | Tenant A pod from peered VNet can access Storage-A |
| TenantA_VNetA3_to_StorageA | pod-c1-aks2-a3s1-high → Storage-A | ✓ Success | Tenant A pod from different cluster can access Storage-A |
| TenantB_to_StorageA_Isolation | pod-c2-aks2-b1s1-low → Storage-A | ✗ Blocked | Tenant B pod CANNOT access Storage-A (tenant isolation) |

**Test Results**: 4 should succeed, 1 should be blocked (tenant isolation)

## Test Case Details

### 8 Pod Scenarios (Created in Job 1)

All test scenarios create the following resources:
- **PodNetwork**: Defines the network configuration for a VNet/subnet combination
- **PodNetworkInstance**: Instance-level configuration with IP allocation
- **Pod**: Test pod running nicolaka/netshoot with HTTP server on port 8080

| # | Scenario | Cluster | VNet | Subnet | Node Type | Pod Name | Purpose |
|---|----------|---------|------|--------|-----------|----------|---------|
| 1 | Customer2-AKS2-VnetB1-S1-LowNic | aks-2 | cx_vnet_b1 | s1 | low-nic | pod-c2-aks2-b1s1-low | Tenant B pod for isolation testing |
| 2 | Customer2-AKS2-VnetB1-S1-HighNic | aks-2 | cx_vnet_b1 | s1 | high-nic | pod-c2-aks2-b1s1-high | Tenant B pod on high-NIC node |
| 3 | Customer1-AKS1-VnetA1-S1-LowNic | aks-1 | cx_vnet_a1 | s1 | low-nic | pod-c1-aks1-a1s1-low | Tenant A pod in NSG-protected subnet |
| 4 | Customer1-AKS1-VnetA1-S2-LowNic | aks-1 | cx_vnet_a1 | s2 | low-nic | pod-c1-aks1-a1s2-low | Tenant A pod for NSG isolation test |
| 5 | Customer1-AKS1-VnetA1-S2-HighNic | aks-1 | cx_vnet_a1 | s2 | high-nic | pod-c1-aks1-a1s2-high | Tenant A pod on high-NIC node |
| 6 | Customer1-AKS1-VnetA2-S1-HighNic | aks-1 | cx_vnet_a2 | s1 | high-nic | pod-c1-aks1-a2s1-high | Tenant A pod in peered VNet |
| 7 | Customer1-AKS2-VnetA2-S1-LowNic | aks-2 | cx_vnet_a2 | s1 | low-nic | pod-c1-aks2-a2s1-low | Cross-cluster same VNet test |
| 8 | Customer1-AKS2-VnetA3-S1-HighNic | aks-2 | cx_vnet_a3 | s1 | high-nic | pod-c1-aks2-a3s1-high | Private endpoint access test |

### Connectivity Tests (9 Test Cases in Job 2)

Tests HTTP connectivity between pods using curl with 5-second timeout:

**Expected to SUCCEED (4 tests)**:

| Test | Source → Destination | Validation | Purpose |
|------|---------------------|------------|---------|
| SameVNetSameSubnet | pod-c1-aks1-a1s2-low → pod-c1-aks1-a1s2-high | HTTP 200 | Basic same-subnet connectivity |
| DifferentVNetSameCustomer | pod-c1-aks1-a2s1-high → pod-c1-aks2-a2s1-low | HTTP 200 | Cross-cluster, same VNet (a2) |
| PeeredVNets | pod-c1-aks1-a1s2-low → pod-c1-aks1-a2s1-high | HTTP 200 | VNet peering (a1 ↔ a2) |
| PeeredVNets_A2toA3 | pod-c1-aks1-a2s1-high → pod-c1-aks2-a3s1-high | HTTP 200 | VNet peering across clusters |

**Expected to FAIL (5 tests)**:

| Test | Source → Destination | Expected Error | Purpose |
|------|---------------------|----------------|---------|
| NSGBlocked_S1toS2 | pod-c1-aks1-a1s1-low → pod-c1-aks1-a1s2-high | Connection timeout | NSG blocks s1→s2 in cx_vnet_a1 |
| NSGBlocked_S2toS1 | pod-c1-aks1-a1s2-low → pod-c1-aks1-a1s1-low | Connection timeout | NSG blocks s2→s1 (bidirectional) |
| DifferentCustomers_A1toB1 | pod-c1-aks1-a1s2-low → pod-c2-aks2-b1s1-low | Connection timeout | Customer isolation (no peering) |
| DifferentCustomers_A2toB1 | pod-c1-aks1-a2s1-high → pod-c2-aks2-b1s1-high | Connection timeout | Customer isolation (no peering) |
| UnpeeredVNets_A3toB1 | pod-c1-aks2-a3s1-high → pod-c2-aks2-b1s1-low | Connection timeout | No peering between a3 and b1 |

**NSG Rules Configuration**:
- cx_vnet_a1 has NSG rules blocking traffic between s1 and s2 subnets:
  - Deny outbound from s1 to s2 (priority 100)
  - Deny inbound from s1 to s2 (priority 110)
  - Deny outbound from s2 to s1 (priority 100)
  - Deny inbound from s2 to s1 (priority 110)

### Private Endpoint Tests (5 Test Cases in Job 3)

Tests access to Azure Storage Account via Private Endpoint with public network access disabled:

**Expected to SUCCEED (4 tests)**:

| Test | Source → Storage | Validation | Purpose |
|------|-----------------|------------|---------|
| TenantA_VNetA1_S1_to_StorageA | pod-c1-aks1-a1s1-low → Storage-A | Blob download via SAS | Access via private endpoint from VNet A1 |
| TenantA_VNetA1_S2_to_StorageA | pod-c1-aks1-a1s2-low → Storage-A | Blob download via SAS | Access via private endpoint from VNet A1 |
| TenantA_VNetA2_to_StorageA | pod-c1-aks1-a2s1-high → Storage-A | Blob download via SAS | Access via peered VNet (A2 peered with A1) |
| TenantA_VNetA3_to_StorageA | pod-c1-aks2-a3s1-high → Storage-A | Blob download via SAS | Access via peered VNet from different cluster |

**Expected to FAIL (1 test)**:

| Test | Source → Storage | Expected Error | Purpose |
|------|-----------------|----------------|---------|
| TenantB_to_StorageA_Isolation | pod-c2-aks2-b1s1-low → Storage-A | Connection timeout/failed | Tenant isolation - no private endpoint access, public blocked |

**Private Endpoint Configuration**:
- Private endpoint created in cx_vnet_a1 subnet 'pe'
- Private DNS zone `privatelink.blob.core.windows.net` linked to:
  - cx_vnet_a1, cx_vnet_a2, cx_vnet_a3 (Tenant A VNets)
  - aks-1 and aks-2 cluster VNets
- Storage Account 1 (Tenant A):
  - Public network access: **Disabled**
  - Shared key access: Disabled (Azure AD only)
  - Blob public access: Disabled
- Storage Account 2 (Tenant B): Public access enabled (for future tests)

**Test Flow**:
1. DNS resolution: Storage FQDN resolves to private IP for Tenant A, fails/public IP for Tenant B
2. Generate SAS token: Azure AD authentication via management plane
3. Download blob: Using curl with SAS token via data plane
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
pn-sv2-long-run-centraluseuap-a1-s1
pni-sv2-long-run-centraluseuap-a1-s1
pn-sv2-long-run-centraluseuap-a1-s1 (namespace)
pod-c1-aks1-a1s1-low
```

**VNet Name Simplification**:
- `cx_vnet_a1` → `a1`
- `cx_vnet_a2` → `a2`
- `cx_vnet_a3` → `a3`
- `cx_vnet_b1` → `b1`

### Setup Flow (When runSetupStages = true)
1. Create resource group with `SkipAutoDeleteTill=2032-12-31` tag
2. Create 2 AKS clusters with 2 node pools each (tagged for persistence)
3. Create 4 customer VNets with subnets and delegations (tagged for persistence)
4. Create VNet peerings 
5. Create storage accounts with persistence tags
6. Create NSGs for subnet isolation
7. Run initial test (create → wait → delete)

**All infrastructure resources are tagged with `SkipAutoDeleteTill=2032-12-31`** to prevent automatic cleanup by Azure subscription policies.


## Manual Testing

Run locally against existing infrastructure:

```bash
export RG="sv2-long-run-centraluseuap"  # Match your resource group
export BUILD_ID="$RG"  # Use same RG name as BUILD_ID for unique resource names

cd test/integration/swiftv2/longRunningCluster
ginkgo -v -trace --timeout=6h .
```

## Node Pool Configuration

### Node Labels and Architecture

All nodes in the clusters are labeled with two key labels for workload identification and NIC capacity. These labels are applied during cluster creation by the `create_aks.sh` script.

**1. Workload Type Label** (`workload-type`):
- Purpose: Identifies which test scenario group the node belongs to
- Current value: `swiftv2-linux` (applied to all nodes in current setup)
- Applied during: Cluster creation in Stage 1 (AKSClusterAndNetworking)
- Applied by: `.pipelines/swiftv2-long-running/scripts/create_aks.sh`
- Future use: Supports multiple workload types running as separate stages (e.g., `swiftv2-windows`, `swiftv2-byonodeid`)
- Stage isolation: Each test stage uses `WORKLOAD_TYPE` environment variable to filter nodes

**2. NIC Capacity Label** (`nic-capacity`):
- Purpose: Identifies the NIC capacity tier of the node
- Applied during: Cluster creation in Stage 1 (AKSClusterAndNetworking)
- Applied by: `.pipelines/swiftv2-long-running/scripts/create_aks.sh`
- Values:
  - `low-nic`: Default nodepool (nodepool1) with `Standard_D4s_v3` (1 NIC)
  - `high-nic`: NPLinux nodepool (nplinux) with `Standard_D16s_v3` (7 NICs)

**Label Application in create_aks.sh**:
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
  - Current: `WORKLOAD_TYPE=swiftv2-linux` in ManagedNodeDataPathTests stage
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
├── pipeline.yaml                    # Main pipeline with schedule
├── README.md                        # This file
├── template/
│   └── long-running-pipeline-template.yaml  # Stage definitions (2 jobs)
└── scripts/
    ├── create_aks.sh               # AKS cluster creation
    ├── create_vnets.sh             # VNet and subnet creation
    ├── create_peerings.sh          # VNet peering setup
    ├── create_storage.sh           # Storage account creation
    ├── create_nsg.sh               # Network security groups
    └── create_pe.sh                # Private endpoint setup

test/integration/swiftv2/longRunningCluster/
├── datapath_test.go                # Original combined test (deprecated)
├── datapath_create_test.go         # Create test scenarios (Job 1)
├── datapath_delete_test.go         # Delete test scenarios (Job 2)
├── datapath.go                     # Resource orchestration
└── helpers/
    └── az_helpers.go               # Azure/kubectl helper functions
```

## Best Practices

1. **Keep infrastructure persistent**: Only recreate when necessary (cluster upgrades, config changes)
2. **Monitor scheduled runs**: Set up alerts for test failures
3. **Resource naming**: BUILD_ID is automatically set to the resource group name, ensuring unique resource names per setup
4. **Tag resources appropriately**: All setup resources automatically tagged with `SkipAutoDeleteTill=2032-12-31`
   - AKS clusters
   - AKS VNets
   - Customer VNets (cx_vnet_a1, cx_vnet_a2, cx_vnet_a3, cx_vnet_b1)
   - Storage accounts
5. **Avoid resource group collisions**: Always use unique `resourceGroupName` when creating new setups
6. **Document changes**: Update this README when modifying test scenarios or infrastructure
