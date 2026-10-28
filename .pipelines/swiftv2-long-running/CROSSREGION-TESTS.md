# SwiftV2 Cross-Region Connectivity Tests

This document covers the **SwiftV2 cross-region connectivity tests** that extend the existing long-running pipeline. The tests keep AKS and SwiftV2 pod provisioning in Region A and add persistent networking, test agents, and Storage destinations in Region B.

The cross-region tests use the existing always-on SwiftV2 pod as both the traffic source and destination. They validate customer-VNet and infrastructure-VNet paths through global VNet peering, Service Tunnel, and Private Link.

### Adding a New Region or Subscription

Edit the `scenarios` parameter in `pipeline.yaml`. Each scenario must define a peer region and a source zone for its cross-region tests:

```yaml
scenarios:
  - subscriptionId: "37deca37-..."
    location: centraluseuap
    label: "centraluseuap"
    peerLocation: eastus2euap
    crossRegionSourceZone: "0"
    longRunningPodZones: ["0"]
```

The `label` must be unique across scenarios. It is used in stage names and peer resource-group names, so scenarios in the same source region but different subscriptions do not collide.

The planned scenario matrix is:

| Scenario | Peer Region | Source Zone |
|----------|-------------|-------------|
| `centraluseuap` | `eastus2euap` | `0` |
| `eastus2euap` | `centraluseuap` | `1` |
| `eastus2euap_baseline` | `centraluseuap` | `1` |
| `westus2` | `westus3` | `0` |

Zone `0` must be present in `westus2.longRunningPodZones` before the `westus2` cross-region tests are activated.

### Prerequisites

- Region A infrastructure and the selected always-on pod must be healthy.
- The Region A customer subnet is `cx_vnet_v1/lr`.
- The Region A infrastructure pod subnet is `aks-1/podnet`.
- Region A and Region B address spaces must not overlap.
- The pipeline service principal must be able to manage VNets, peerings, Container Instances, Storage accounts, Private Endpoints, Private DNS, tags, and resource locks.
- Existing long-running pipeline Storage data-plane permissions remain required.

---

## Cross-Region Connectivity Tests

### Design

The selected always-on pod on Region A `aks-1` has two addresses:

- **Primary pod IP**: infrastructure path through `aks-1/podnet`.
- **Delegated pod IP**: customer path through `cx_vnet_v1/lr`.

Region B contains separate customer and infrastructure VNets. Each VNet is globally peered only with its matching Region A VNet.

```text
Region A                                                Region B

AKS cluster: aks-1
+----------------------------------+                    +----------------------------------+
| Always-on SwiftV2 pod            |                    | Customer peer VNet               |
|                                  |                    |  - private ACI test agent        |
| Delegated IP                     |<-- global peer --->|  - Private Endpoint subnet       |
| cx_vnet_v1/lr                    |                    +----------------------------------+
|                                  |
| Primary IP                       |                    +----------------------------------+
| aks-1/podnet                     |<-- global peer --->| Infrastructure peer VNet         |
+----------------------------------+                    |  - private ACI test agent        |
                                                      |  - Private Endpoint subnet       |
                                                      +----------------------------------+

                                                      Storage destinations:
                                                       - Service Tunnel account
                                                       - Customer Private Link account
                                                       - Infrastructure Private Link account
```

### Region B Networking

Region B receives two VNets:

| VNet | Region A Peer | Purpose |
|------|---------------|---------|
| Customer peer VNet | `cx_vnet_v1` | Customer-path agent and Private Endpoint |
| Infrastructure peer VNet | `aks-1` VNet | Infrastructure-path agent and Private Endpoint |

Each Region B VNet contains:

- A delegated subnet for a private VNet-injected Azure Container Instance.
- A separate subnet for Private Endpoints.
- A NAT gateway and Standard public IP for supported ACI outbound access.
- An NSG that permits TCP 8080 only from the matching Region A pod subnet.

Four peering objects provide bidirectional global VNet peering:

1. Region A customer VNet to Region B customer VNet.
2. Region B customer VNet to Region A customer VNet.
3. Region A infrastructure VNet to Region B infrastructure VNet.
4. Region B infrastructure VNet to Region A infrastructure VNet.

All peerings must reach `Connected` before provisioning continues.

### Private Test Agents

Each Region B VNet contains one private ACI test agent:

| Property | Value |
|----------|-------|
| Image | Pinned `nicolaka/netshoot` digest |
| Addressing | Private IP only |
| Listener | TCP 8080 |
| Restart policy | `Always` |
| Customer agent source | Region A delegated pod IP |
| Infrastructure agent source | Region A primary pod IP |

The provisioning script verifies the image, subnet, command, provisioning state, runtime state, private IP, and TCP listener before declaring an agent ready.

### Service Tunnel Destination

The Service Tunnel scenario uses a dedicated Region B Storage account:

| Property | Value |
|----------|-------|
| Service endpoint | `Microsoft.Storage.Global` |
| Enabled on | Region A customer and infrastructure pod subnets |
| Storage firewall | Default action `Deny` |
| Allowed networks | Exactly the two Region A pod subnets |
| Private Endpoint | None |

The test targets the Storage service VIP. The Service Tunnel datapath reaches the Region B Storage backend while preserving Region A VNet identity for Storage network ACL evaluation.

### Private Link Destinations

Private Link uses separate Storage accounts for the customer and infrastructure paths:

| Storage Account | Private Endpoint Location | Public Network Access |
|-----------------|---------------------------|-----------------------|
| Customer Private Link | Region B customer VNet | Disabled |
| Infrastructure Private Link | Region B infrastructure VNet | Disabled |

Both Private Endpoints use the existing `privatelink.blob.core.windows.net` Private DNS zone in the Region A resource group. The zone is linked to the Region A customer and infrastructure VNets.

Separate Storage accounts make DNS resolution and path validation deterministic. Each test resolves the account hostname and verifies the exact expected Private Endpoint IP before transferring data.

### Test Matrix

The cross-region suite contains ten tests:

| # | Source | Destination | Path | Expected Result |
|---|--------|-------------|------|-----------------|
| 1 | Region A delegated IP | Region B customer agent | Customer peering | Success |
| 2 | Region B customer agent | Region A delegated IP | Customer peering | Success |
| 3 | Region A primary IP | Region B infrastructure agent | Infrastructure peering | Success |
| 4 | Region B infrastructure agent | Region A primary IP | Infrastructure peering | Success |
| 5 | Region A delegated IP | Region B infrastructure agent | Cross-path negative control | Failure |
| 6 | Region A primary IP | Region B customer agent | Cross-path negative control | Failure |
| 7 | Region A delegated IP | Region B Service Tunnel Storage | Customer Service Tunnel | Success |
| 8 | Region A primary IP | Region B Service Tunnel Storage | Infrastructure Service Tunnel | Success |
| 9 | Region A delegated IP | Region B customer Private Link Storage | Customer Private Link | Success |
| 10 | Region A primary IP | Region B infrastructure Private Link Storage | Infrastructure Private Link | Success |

Outbound pod probes bind explicitly to the intended primary or delegated source IP. Inbound tests execute from the matching Region B agent toward the corresponding Region A pod IP.

---

## Pipeline Structure

Peer resources are provisioned by the existing `NetworkingAndStorage` job. Cross-region traffic tests run in one new stage per scenario after the selected source-zone always-on pod is healthy and the existing regional lease is acquired.

```text
For each scenario:

AKSClusterAndNetworking_<label>
        |
        +--> LongRunningPodTests_<label>_Z<source-zone>
        |
        +--> AcquireLease_<label>
                  |
                  v
        CrossRegionConnectivity_<label>
                  |
                  v
        ReleaseLease_<label>
```

The cross-region stage depends on:

- `AKSClusterAndNetworking_<label>`.
- The selected `LongRunningPodTests_<label>_Z<source-zone>` stage.
- `AcquireLease_<label>`.

### Lease Mechanism

The existing regional ConfigMap lease is reused. No cross-region-specific lease is introduced.

`ReleaseLease_<label>` must include the cross-region stage in every dependency branch so the lease is released whether the tests pass, fail, or are skipped.

---

## Resource Naming

Each scenario receives its own Region B resource group:

```text
sv2-long-run-<stageLabel>-peer-<peerLocation>
```

Using `stageLabel` prevents the primary and baseline `eastus2euap` scenarios from sharing peer resources.

Fixed peer address spaces:

| Resource | Address Space |
|----------|---------------|
| Customer peer VNet | `192.168.240.0/22` |
| Customer agent subnet | `192.168.240.0/24` |
| Customer Private Endpoint subnet | `192.168.241.0/24` |
| Infrastructure peer VNet | `192.168.244.0/22` |
| Infrastructure agent subnet | `192.168.244.0/24` |
| Infrastructure Private Endpoint subnet | `192.168.245.0/24` |

Storage account names are deterministic and globally unique. They use a SHA-256-derived prefix based on subscription, scenario label, peer region, and role:

| Role | Suffix |
|------|--------|
| Service Tunnel | `st` |
| Customer Private Link | `pc` |
| Infrastructure Private Link | `pi` |

---

## Pipeline Parameters

| Parameter | Description | Set By |
|-----------|-------------|--------|
| `subscriptionId` | Subscription containing the Region A and Region B resources | Scenario matrix |
| `location` | Region A source region | Scenario matrix |
| `label` | Unique scenario identifier used in stages and resource names | Scenario matrix |
| `peerLocation` | Region B peer region | Scenario matrix |
| `crossRegionSourceZone` | Region A always-on pod zone used by the cross-region stage | Scenario matrix |
| `resourceGroupName` | Shared resource-group name prefix | Pipeline parameter |

---

## Idempotency

Cross-region infrastructure provisioning runs outside the lease and must be safe to repeat.

| Operation | Idempotency |
|-----------|-------------|
| Peer resource group | Reused; required tags are merged without replacing unrelated tags |
| CIDR validation | All Region A and Region B address spaces are preflighted before peering mutation |
| VNets and subnets | Existing location, address space, delegation, NSG, and NAT configuration are verified |
| NAT gateways and public IPs | Existing location, SKU, allocation, timeout, and association are verified |
| NSGs | Required TCP 8080 allow and deny rules are reconciled |
| VNet peerings | Existing remote VNet and VNet-access settings are verified; state must reach `Connected` |
| ACI agents | Existing image, subnet, command, readiness, private IP, and listener are verified |
| Storage accounts | Deterministic names and role tags identify reusable accounts |
| Test blobs | Initialization tag prevents data-plane initialization after network lockdown |
| Service Tunnel ACLs | Exactly two Region A subnet rules are retained; unintended IP rules are removed |
| Private Endpoints | Existing target resource, subnet, approval state, private IP, and DNS group are verified |
| Resource lock | Applied after all peer resources are provisioned successfully |

Normal pipeline runs never delete cross-region resources. Disabling or removing a scenario requires an explicit teardown operation.

---

## File Structure

```text
.pipelines/swiftv2-long-running/
|-- pipeline.yaml
|-- CROSSREGION-TESTS.md
|-- template/
|   |-- long-running-pipeline-template.yaml
|   |-- infrastructure-setup-stage.yaml
|   `-- cross-region-connectivity-stage.yaml
`-- scripts/
    |-- cross_region_common.sh
    |-- create_cross_region_networking.sh
    |-- create_cross_region_agents.sh
    |-- create_cross_region_service_tunnel.sh
    `-- create_cross_region_private_link.sh

test/integration/swiftv2/longRunningCluster/
|-- cross_region.go
|-- cross_region_test.go
`-- datapath_cross_region_test.go
```

---

## Build Tags

The integration suite uses a dedicated Go build tag:

| Build Tag | File | Pipeline Job |
|-----------|------|--------------|
| `cross_region_connectivity_test` | `datapath_cross_region_test.go` | `CrossRegionConnectivity_<label>` |

Pure helper tests in `cross_region_test.go` run without the integration build tag.

---

## Implementation Checkpoints

### Checkpoint 1: Provision Cross-Region Infrastructure

**Purpose**: Create and validate persistent Region B resources without activating cross-region traffic tests.

Changes:

1. Add required `peerLocation` values to every scenario.
2. Add deterministic naming and CIDR preflight helpers.
3. Provision the Region B resource group, VNets, subnets, NAT gateways, public IPs, NSGs, and four peerings.
4. Provision the two private Region B test agents.
5. Provision the Service Tunnel Storage destination and Region A subnet rules.
6. Provision the customer and infrastructure Private Link Storage destinations, Private Endpoints, and DNS integration.
7. Invoke provisioning from the existing infrastructure stage.
8. Add syntax, static, and rerun validation.

Files:

- `pipeline.yaml`
- `template/long-running-pipeline-template.yaml`
- `template/infrastructure-setup-stage.yaml`
- `scripts/cross_region_common.sh`
- `scripts/create_cross_region_networking.sh`
- `scripts/create_cross_region_agents.sh`
- `scripts/create_cross_region_service_tunnel.sh`
- `scripts/create_cross_region_private_link.sh`

Merge gate:

1. Run the infrastructure pipeline manually.
2. Verify all four scenario resource sets.
3. Verify all peerings report `Connected`.
4. Verify agents and Private Endpoints are healthy.
5. Verify Storage ACLs and `Microsoft.Storage.Global`.
6. Run the pipeline a second time and confirm idempotent reuse.
7. Allow at least one scheduled run to complete without regressions.

Checkpoint 1 does not add a test stage, modify the lease, or change existing AKS and pod provisioning.

### Checkpoint 2: Activate Cross-Region Tests

**Purpose**: Add test implementation and pipeline orchestration after Region B infrastructure is proven stable.

Changes:

1. Add `crossRegionSourceZone` to every scenario.
2. Add zone `0` to `westus2.longRunningPodZones`.
3. Add focused Go helpers for environment parsing, pod discovery, source-address selection, command execution, bound probes, DNS validation, and failed-step reporting.
4. Add pure unit tests for the helpers.
5. Add the build-tagged ten-test integration suite.
6. Add `cross-region-connectivity-stage.yaml`.
7. Expand one cross-region stage per scenario.
8. Add infrastructure, source-zone, and lease dependencies.
9. Add the cross-region stage to every release-lease dependency branch.
10. Publish per-path metrics.
11. Update the long-running pipeline documentation.

Files:

- `pipeline.yaml`
- `template/long-running-pipeline-template.yaml`
- `template/cross-region-connectivity-stage.yaml`
- `test/integration/swiftv2/longRunningCluster/cross_region.go`
- `test/integration/swiftv2/longRunningCluster/cross_region_test.go`
- `test/integration/swiftv2/longRunningCluster/datapath_cross_region_test.go`
- `README.md`
- `LONGRUNNING-TESTS.md`
- `CROSSREGION-TESTS.md`

Merge gate:

1. Go unit tests pass.
2. The tagged integration suite compiles.
3. Pipeline template expansion contains exactly one cross-region stage per scenario.
4. Lease release remains correct for all workload flag combinations.
5. Manually validate `centraluseuap` to `eastus2euap`.
6. Confirm all ten tests and lease release.
7. Repeat the canary to exclude one-time provisioning effects.
8. Validate `eastus2euap` to `centraluseuap`.
9. Validate `westus2` to `westus3`.
10. Validate the baseline subscription separately.

---

## Validation

```bash
bash -n .pipelines/swiftv2-long-running/scripts/*.sh
shellcheck .pipelines/swiftv2-long-running/scripts/cross_region_common.sh \
  .pipelines/swiftv2-long-running/scripts/create_cross_region_*.sh

go test ./test/integration/swiftv2/longRunningCluster -run TestCrossRegion
go test ./test/integration/swiftv2/longRunningCluster \
  -run '^$' -tags=cross_region_connectivity_test

git diff --check
```

Pipeline template expansion must also be validated. Go and shell checks do not verify the Azure Pipelines stage graph.

---

## Operational Constraints

- Region A AKS, PodNetwork, PodNetworkInstance, and existing pod creation remain unchanged.
- Provisioning must be idempotent because it runs outside the lease.
- CIDR conflicts must fail before any peering mutation.
- Resources are discovered by deterministic names and role tags.
- Secrets, account keys, and SAS tokens must not be logged.
- Outbound pod probes must bind to the intended source IP.
- Private Link tests must validate the expected Private Endpoint IP before transferring data.
- Normal pipeline runs must not delete peer resources.
