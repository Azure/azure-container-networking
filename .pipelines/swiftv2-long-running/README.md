# SwiftV2 Long-Running Pipeline

This pipeline tests SwiftV2 pod networking in a persistent environment with scheduled test runs.

## Architecture Overview

**Infrastructure (Persistent)**:
- **2 AKS Clusters**: aks-1, aks-2 (4 nodes each: 2 low-NIC default pool, 2 high-NIC nplinux pool)
- **4 VNets**: cx_vnet_a1, cx_vnet_a2, cx_vnet_a3 (Customer 1 with PE to storage), cx_vnet_b1 (Customer 2)
- **VNet Peerings**: vnet mesh.
- **Storage Account**: With private endpoint from cx_vnet_a1
- **NSGs**: Restricting traffic between subnets (s1, s2) in vnet cx_vnet_a1.

**Test Scenarios (8 total)**:
- Multiple pods across 2 clusters, 4 VNets, different subnets (s1, s2), and node types (low-NIC, high-NIC)
- Each test run: Create all resources → Wait 20 minutes → Delete all resources
- Tests run automatically every 1 hour via scheduled trigger

## Pipeline Modes

### Mode 1: Scheduled Test Runs (Default)
**Trigger**: Automated cron schedule every 1 hour  
**Purpose**: Continuous validation of long-running infrastructure  
**Setup Stages**: Disabled  
**Test Duration**: ~30-40 minutes per run  
**Resource Group**: Static (default: `sv2-long-run-<region>`, e.g., `sv2-long-run-centraluseuap`)

```yaml
# Runs automatically every 1 hour
# No manual/external triggers allowed
```

### Mode 2: Initial Setup or Rebuild
**Trigger**: Manual run with parameter change  
**Purpose**: Create new infrastructure or rebuild existing  
**Setup Stages**: Enabled via `runSetupStages: true`  
**Resource Group**: Auto-generated or custom

**To create new infrastructure**:
1. Go to Pipeline → Run pipeline
2. Set `runSetupStages` = `true`
3. **Optional**: Leave `resourceGroupName` empty to auto-generate `sv2-long-run-<location>`
   - Or provide custom name for parallel setups (e.g., `sv2-long-run-eastus-dev`)
4. Optionally adjust `location`, `vmSkuDefault`, `vmSkuHighNIC`
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
| `resourceGroupName` | `""` (empty) | **Leave empty** to auto-generate `sv2-long-run-<location>`. Provide custom name only for parallel setups (e.g., `sv2-long-run-eastus-dev`). |
| `vmSkuDefault` | `Standard_D4s_v3` | VM SKU for low-NIC node pool (1 NIC). |
| `vmSkuHighNIC` | `Standard_D16s_v3` | VM SKU for high-NIC node pool (7 NICs). |

**Note**: Setup-only parameters are ignored when `runSetupStages=false` (scheduled runs).

## How It Works

### Scheduled Test Flow
Every 1 hour, the pipeline:
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

### Setup Flow (When runSetupStages = true)
1. Create resource group with `SkipAutoDeleteTill=2032-12-31` tag
2. Create 2 AKS clusters with 2 node pools each (tagged for persistence)
3. Create 4 customer VNets with subnets and delegations (tagged for persistence)
4. Create VNet peerings 
5. Create storage accounts with persistence tags
6. Create NSGs for subnet isolation
7. Run initial test (create → wait → delete)

**All infrastructure resources are tagged with `SkipAutoDeleteTill=2032-12-31`** to prevent automatic cleanup by Azure subscription policies.

## Resource Naming

All test resources use the pattern: `<type>-static-setup-<vnet>-<subnet>`

**Examples**:
- PodNetwork: `pn-static-setup-a1-s1`
- PodNetworkInstance: `pni-static-setup-a1-s1`  
- Pod: `pod-c1-aks1-a1s1-low`
- Namespace: `pn-static-setup-a1-s1`

VNet names are simplified:
- `cx_vnet_a1` → `a1`
- `cx_vnet_b1` → `b1`

## Switching to a New Setup

**Scenario**: You created a new setup in RG `sv2-long-run-eastus` and want scheduled runs to use it.

**Steps**:
1. Go to Pipeline → Edit
2. Update location parameter default value:
   ```yaml
   - name: location
     default: "centraluseuap"  # Change this
   ```
3. Save and commit
4. RG name will automatically become `sv2-long-run-centraluseuap`

Alternatively, manually trigger with the new location or override `resourceGroupName` directly.

## Creating Multiple Test Setups

**Use Case**: You want to create a new test environment without affecting the existing one (e.g., for testing different configurations, regions, or versions).

**Steps**:
1. Go to Pipeline → Run pipeline
2. Set `runSetupStages` = `true`
3. **Set `resourceGroupName`** to a unique value:
   - For different region: `sv2-long-run-eastus`
   - For parallel test: `sv2-long-run-centraluseuap-dev`
   - For experimental: `sv2-long-run-centraluseuap-v2`
   - Or leave empty to use auto-generated `sv2-long-run-<location>`
4. Optionally adjust `location`, `vmSkuDefault`, `vmSkuHighNIC`
5. Run pipeline

**After setup completes**:
- The new infrastructure will be tagged with `SkipAutoDeleteTill=2032-12-31`
- Resources are isolated by the unique resource group name
- To run tests against the new setup, the scheduled pipeline would need to be updated with the new RG name

**Example Scenarios**:
| Scenario | Resource Group Name | Purpose |
|----------|-------------------|---------|
| Default production | `sv2-long-run-centraluseuap` | Daily scheduled tests |
| East US environment | `sv2-long-run-eastus` | Regional testing |
| Test new features | `sv2-long-run-centraluseuap-dev` | Development/testing |
| Version upgrade | `sv2-long-run-centraluseuap-v2` | Parallel environment for upgrades |

## Resource Naming
 instead of ping use 
The pipeline uses the **resource group name as the BUILD_ID** to ensure unique resource names per test setup. This allows multiple parallel test environments without naming collisions.

**Generated Resource Names**:
```
BUILD_ID = <resourceGroupName>

PodNetwork:         pn-<BUILD_ID>-<vnet>-<subnet>
PodNetworkInstance: pni-<BUILD_ID>-<vnet>-<subnet>
Namespace:          pn-<BUILD_ID>-<vnet>-<subnet>
Pod:                pod-<scenario-suffix>
```

**Example for `resourceGroupName=sv2-long-run-centraluseuap`**:
```
pn-sv2-long-run-centraluseuap-b1-s1       (PodNetwork for cx_vnet_b1, subnet s1)
pni-sv2-long-run-centraluseuap-b1-s1      (PodNetworkInstance)
pn-sv2-long-run-centraluseuap-a1-s1       (PodNetwork for cx_vnet_a1, subnet s1)
pni-sv2-long-run-centraluseuap-a1-s2      (PodNetworkInstance for cx_vnet_a1, subnet s2)
```

**Example for different setup `resourceGroupName=sv2-long-run-eastus`**:
```
pn-sv2-long-run-eastus-b1-s1       (Different from centraluseuap setup)
pni-sv2-long-run-eastus-b1-s1
pn-sv2-long-run-eastus-a1-s1
```

This ensures **no collision** between different test setups running in parallel.

## Deletion Strategy
### Phase 1: Delete All Pods
Deletes all pods across all scenarios first. This ensures IP reservations are released.

```
Deleting pod pod-c2-aks2-b1s1-low...
Deleting pod pod-c2-aks2-b1s1-high...
...
```

### Phase 2: Delete Shared Resources
Groups resources by vnet/subnet/cluster and deletes PNI/PN/Namespace once per group.

```
Deleting PodNetworkInstance pni-static-setup-b1-s1...
Deleting PodNetwork pn-static-setup-b1-s1...
Deleting namespace pn-static-setup-b1-s1...
```

**Why**: Multiple pods can share the same PNI. Deleting PNI while pods exist causes "ReservationInUse" errors.

## Troubleshooting

### Tests are running on wrong cluster
- Check `resourceGroupName` parameter points to correct RG
- Verify RG contains aks-1 and aks-2 clusters
- Check kubeconfig retrieval in logs

### Setup stages not running
- Verify `runSetupStages` parameter is set to `true`
- Check condition: `condition: eq(${{ parameters.runSetupStages }}, true)`

### Schedule not triggering
- Verify cron expression: `"0 */1 * * *"` (every 1 hour)
- Check branch in schedule matches your working branch
- Ensure `always: true` is set (runs even without code changes)

### PNI stuck with "ReservationInUse"
- Check if pods were deleted first (Phase 1 logs)
- Manual fix: Delete pod → Wait 10s → Patch PNI to remove finalizers

### Pipeline timeout after 6 hours
- This is expected behavior (timeoutInMinutes: 360)
- Tests should complete in ~30-40 minutes
- If tests hang, check deletion logs for stuck resources

## Manual Testing

Run locally against existing infrastructure:

```bash
export RG="sv2-long-run-centraluseuap"  # Match your resource group
export BUILD_ID="$RG"  # Use same RG name as BUILD_ID for unique resource names

cd test/integration/swiftv2/longRunningCluster
ginkgo -v -trace --timeout=6h .
```

## Node Pool Configuration

- **Low-NIC nodes** (`Standard_D4s_v3`): 1 NIC, label `agentpool!=nplinux`
  - Can only run 1 pod at a time
  
- **High-NIC nodes** (`Standard_D16s_v3`): 7 NICs, label `agentpool=nplinux`
  - Currently limited to 1 pod per node in test logic

## Schedule Modification

To change test frequency, edit the cron schedule:

```yaml
schedules:
  - cron: "0 */1 * * *"  # Every 1 hour (current)
  # Examples:
  # - cron: "0 */2 * * *"  # Every 2 hours
  # - cron: "0 */6 * * *"  # Every 6 hours
  # - cron: "0 0,8,16 * * *"  # At 12am, 8am, 4pm
  # - cron: "0 0 * * *"  # Daily at midnight
```

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

## Resource Tags

All infrastructure resources are automatically tagged during creation:

```bash
SkipAutoDeleteTill=2032-12-31
```

This prevents automatic cleanup by Azure subscription policies that delete resources after a certain period. The tag is applied to:
- Resource group (via create_resource_group job)
- AKS clusters (aks-1, aks-2)
- AKS cluster VNets
- Customer VNets (cx_vnet_a1, cx_vnet_a2, cx_vnet_a3, cx_vnet_b1)
- Storage accounts (sa1xxxx, sa2xxxx)

To manually update the tag date:
```bash
az resource update --ids <resource-id> --set tags.SkipAutoDeleteTill=2033-12-31
```
