# CNS Agent Brief (AKS/Azure CNI)

This brief is for code contributors working on Container Networking Service (CNS) in this repo.

## What CNS is
CNS is the container networking daemon running on every AKS node for Azure CNI. It bridges the AKS control plane and node dataplane by managing pod IP allocation state and coordinating with the Azure networking control plane for routable VNET IPs.

In Azure CNI modes where pods receive routable VNET IPs, CNS tracks the goal state from the NodeNetworkConfig (NNC) CRD and only exposes IPs to pods once the Network Container (NC) is programmed by NMAgent.

## Core architecture (high level)
- HTTP REST API used by CNI/azure-ipam for IP assignment and release.
- Controller-runtime reconcilers that watch NNCs (and SwiftV2 CRDs) and reconcile desired NC/IP state.
- IPAM pool monitor (dynamic IP scenarios) to resize IP pools on demand. Not used in fixed-prefix modes (e.g., overlay).
- IMDS/wireserver integration to query NMAgent NC status.

## Core flow (node → pod IP assignment)
1) DNC/RC creates a NodeNetworkConfig (NNC) for the node.
2) DNC allocates IPs as Network Containers (NCs) and publishes them to the fabric.
3) NMAgent programs the NC; CNS watches for NC version readiness via IMDS/wireserver.
4) CNS ingests NNC as goal state; once NC is ready, IPs become Available.
5) CNI/azure-ipam calls CNS to RequestIPConfigs/ReleaseIPConfigs during pod lifecycle.
6) CNS acts as the IPAM store for pod IP allocation and release.

## Primary CNS APIs used by AKS
Primary (current) IPAM APIs:
- POST /network/requestipconfigs — allocate one or more IPs for a pod.
- POST /network/releaseipconfigs — release IPs for a pod.

Legacy fallbacks (used when RequestIPConfigs/ReleaseIPConfigs are unsupported):
- POST /network/requestipconfig — allocate a single IP.
- POST /network/releaseipconfig — release a single IP.

Additional CNI-used queries:
- POST /network/getnetworkcontainerbyorchestratorcontext
- POST /network/getAllNetworkContainers

## Configuration inputs
- CNS is configured via JSON config file (cns_config.json) provided via ConfigMap in AKS.
- Config file path can be set by CNS_CONFIGURATION_PATH env var or command-line flag.
- Environment variables used by CNS (examples): NODENAME, NODE_IP, POD_CIDRs, SERVICE_CIDRs, INFRA_VNET_CIDRs.
- There are legacy flags and env vars in main; treat configuration as scenario-specific and static.

## State persistence
CNS writes persistent state to disk:
- Main CNS state: /var/lib/azure-network/azure-cns.json (Linux default). Stores NCs, networks, orchestrator data, timestamps, etc.
- Endpoint state (ManageEndpointState=true): /var/run/azure-cns/azure-endpoints.json on Linux, /k/azurecns/azure-endpoints.json on Windows. Stores containerID → endpoint IP mappings.
- CNI state file used for migration: /var/run/azure-vnet.json (Linux default).

## Code entry points (start here)
- Service entry: cns/service/main.go
- REST API handlers: cns/restserver/
- Reconcilers: cns/kubecontroller/
- IPAM pool monitor: cns/ipampool/

## Tests to run for CNS changes
- Tests in the packages above (restserver, kubecontroller, ipampool, service) are the first-line regression checks.

## Critical caution areas
CNS initialization and state management are high-risk code paths. Behavioral changes can cause large-scale impact. Keep changes minimal and validate thoroughly.

## SwiftV2 note
SwiftV2 adds multitenancy and multi-NIC behaviors. Treat SwiftV2-only APIs/CRDs as specialized paths; verify scenario-specific behavior when touching related code.