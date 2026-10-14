# SwiftV2 Cross-Region Connectivity

## Goal

Extend the SwiftV2 long-running pipeline with cross-region coverage while leaving AKS and SwiftV2 pod provisioning in Region A.

Each scenario always has a peer region:

| Scenario | Peer region | Source zone |
| --- | --- | --- |
| `centraluseuap` | `eastus2euap` | `0` |
| `eastus2euap` | `centraluseuap` | `1` |
| `eastus2euap_baseline` | `centraluseuap` | `1` |
| `westus2` | `westus3` | `0` |

`westus2` must add zone `0` to `longRunningPodZones` before its cross-region tests are activated.

## Design

The existing always-on pod on Region A `aks-1` is the test source and destination:

- Primary pod IP: infrastructure path through `aks-1/podnet`.
- Delegated pod IP: customer path through `cx_vnet_v1/lr`.

Region B receives two VNets:

1. Customer peer VNet, globally peered with Region A `cx_vnet_v1`.
2. Infrastructure peer VNet, globally peered with the Region A `aks-1` VNet.

Each Region B VNet contains:

- A private test agent listening on TCP 8080.
- A separate Private Endpoint subnet.

Region B also receives three Storage accounts:

1. Service Tunnel account:
   - Uses `Microsoft.Storage.Global` on the two Region A source subnets.
   - Storage firewall defaults to `Deny`.
   - Only the two Region A subnet IDs are allowed.
   - Has no Private Endpoint.
2. Customer Private Link account:
   - Public access disabled.
   - Private Endpoint in the Region B customer VNet.
3. Infrastructure Private Link account:
   - Public access disabled.
   - Private Endpoint in the Region B infrastructure VNet.

Separate Private Link accounts make DNS and path validation deterministic.

### Tests

1. Region A delegated IP to Region B customer agent.
2. Region B customer agent to Region A delegated IP.
3. Region A primary IP to Region B infrastructure agent.
4. Region B infrastructure agent to Region A primary IP.
5. Customer-to-infrastructure negative control.
6. Infrastructure-to-customer negative control.
7. Customer-path Service Tunnel access.
8. Infrastructure-path Service Tunnel access.
9. Customer-path Private Link access.
10. Infrastructure-path Private Link access.

Outbound pod probes bind explicitly to the intended primary or delegated source IP. Private Link tests assert the exact expected Private Endpoint IP before transferring data.

## Pipeline Model

Peer resources are provisioned by the existing `NetworkingAndStorage` job.

Cross-region traffic tests run in one new stage per scenario:

```text
AKSClusterAndNetworking
          |
LongRunningPodTests_<source-zone>
          |
AcquireLease
          |
CrossRegionConnectivity
          |
ReleaseLease
```

The existing regional lease is reused. No new lease is introduced.

## PR 1: Provision Cross-Region Infrastructure

### Purpose

Create and validate persistent peer resources without activating cross-region traffic tests.

### Changes

1. Add required `peerLocation` to every scenario in `pipeline.yaml`.
2. Add deterministic cross-region naming helpers.
3. Add CIDR preflight and Region B networking:
   - Peer resource group.
   - Customer and infrastructure VNets.
   - Agent and Private Endpoint subnets.
   - NAT gateways for VNet-injected agents.
   - NSGs.
   - Four global VNet peering objects.
4. Add the two Region B test agents.
5. Add the Service Tunnel Storage account and configure:
   - `Microsoft.Storage.Global`.
   - Region A subnet rules.
   - Default network action `Deny`.
6. Add the two Private Link Storage accounts, endpoints, and DNS registrations.
7. Invoke all provisioning from the existing infrastructure stage.
8. Add script syntax/static validation.

### Files

- `.pipelines/swiftv2-long-running/pipeline.yaml`
- `.pipelines/swiftv2-long-running/template/long-running-pipeline-template.yaml`
- `.pipelines/swiftv2-long-running/template/infrastructure-setup-stage.yaml`
- `.pipelines/swiftv2-long-running/scripts/cross_region_common.sh`
- `.pipelines/swiftv2-long-running/scripts/create_cross_region_networking.sh`
- `.pipelines/swiftv2-long-running/scripts/create_cross_region_agents.sh`
- `.pipelines/swiftv2-long-running/scripts/create_cross_region_service_tunnel.sh`
- `.pipelines/swiftv2-long-running/scripts/create_cross_region_private_link.sh`

### Merge gate

Before PR 2:

1. Run the infrastructure pipeline manually.
2. Verify all four scenario resource sets.
3. Verify all peerings report `Connected`.
4. Verify agents and Private Endpoints are healthy.
5. Verify Storage ACLs and `Microsoft.Storage.Global`.
6. Run the pipeline a second time and confirm idempotent reuse.
7. Allow at least one scheduled run to complete without regressions.

PR 1 does not add a test stage, modify the lease, or change existing pod provisioning.

## PR 2: Activate Cross-Region Tests

### Purpose

Add the test implementation and pipeline orchestration after Region B infrastructure is proven stable.

### Changes

1. Add `crossRegionSourceZone` to every scenario.
2. Add zone `0` to `westus2.longRunningPodZones`.
3. Add focused Go helpers for:
   - Environment/config parsing.
   - Always-on pod discovery.
   - Primary and delegated source addresses.
   - Pod and Region B agent command execution.
   - Bound TCP and Storage probes.
   - DNS/Private Endpoint address validation.
   - Secret redaction and failed-step reporting.
4. Add pure unit tests for the helpers.
5. Add the build-tagged ten-test integration suite.
6. Add `cross-region-connectivity-stage.yaml`.
7. Expand one cross-region stage per scenario.
8. Make the stage depend on:
   - Regional infrastructure.
   - Selected long-running source-zone stage.
   - Existing lease acquisition.
9. Add the cross-region stage to every release-lease dependency branch.
10. Publish per-path metrics.
11. Update pipeline documentation after documentation PR #4987 is merged.

### Files

- `.pipelines/swiftv2-long-running/pipeline.yaml`
- `.pipelines/swiftv2-long-running/template/long-running-pipeline-template.yaml`
- `.pipelines/swiftv2-long-running/template/cross-region-connectivity-stage.yaml`
- `test/integration/swiftv2/longRunningCluster/cross_region.go`
- `test/integration/swiftv2/longRunningCluster/cross_region_test.go`
- `test/integration/swiftv2/longRunningCluster/datapath_cross_region_test.go`
- `.pipelines/swiftv2-long-running/README.md`
- `.pipelines/swiftv2-long-running/LONGRUNNING-TESTS.md`

### Merge gate

1. Go unit tests pass.
2. Tagged integration suite compiles.
3. Pipeline template expansion contains exactly one cross-region stage per scenario.
4. Lease release remains correct for all workload flag combinations.
5. Manually validate `centraluseuap` to `eastus2euap`.
6. Confirm all ten tests and lease release.
7. Repeat the canary to exclude one-time provisioning effects.
8. Validate `eastus2euap` to `centraluseuap`.
9. Validate `westus2` to `westus3`.
10. Validate the baseline subscription separately.

## Validation Commands

```bash
bash -n .pipelines/swiftv2-long-running/scripts/*.sh
shellcheck .pipelines/swiftv2-long-running/scripts/cross_region_common.sh \
  .pipelines/swiftv2-long-running/scripts/create_cross_region_*.sh

go test ./test/integration/swiftv2/longRunningCluster -run TestCrossRegion
go test ./test/integration/swiftv2/longRunningCluster \
  -run '^$' -tags=cross_region_connectivity_test

git diff --check
```

Pipeline template expansion must also be validated; Go and shell checks are not substitutes for verifying the Azure Pipelines stage graph.

## Operational Constraints

- Region A AKS, PodNetwork, PNI, and existing pod creation remain unchanged.
- Provisioning must be idempotent because it runs outside the lease.
- CIDR conflicts must fail before any peering mutation.
- Resources are discovered by deterministic names and role tags.
- Secrets, account keys, and SAS tokens must not be logged.
- Disabling or deleting peer resources requires an explicit teardown operation; normal pipeline runs never delete them.
