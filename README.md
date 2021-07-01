# Microsoft Azure Container Networking

[![Build Status](https://msazure.visualstudio.com/One/_apis/build/status/Custom/Networking/ContainerNetworking/Azure.azure-container-networking?branchName=master)](https://msazure.visualstudio.com/One/_build/latest?definitionId=95007&branchName=master) [![Go Report Card](https://goreportcard.com/badge/github.com/Azure/azure-container-networking)](https://goreportcard.com/report/github.com/Azure/azure-container-networking) [![golangci-lint](https://github.com/Azure/azure-container-networking/actions/workflows/repo-hygiene.yaml/badge.svg?event=schedule)](https://github.com/Azure/azure-container-networking/actions/workflows/repo-hygiene.yaml) ![GitHub release](https://img.shields.io/github/release/Azure/azure-container-networking.svg)

| Azure Network Policy Manager Conformance      |  |
| ----------- | ----------- |
| Cyclonus Network Policy Suite      | [![Cyclonus Network Policy Test](https://github.com/Azure/azure-container-networking/actions/workflows/cyclonus-netpol-test.yaml/badge.svg?branch=master)](https://github.com/Azure/azure-container-networking/actions/workflows/cyclonus-netpol-test.yaml)       |
| Kubernetes Network Policy E2E  | [![Build Status](https://dev.azure.com/msazure/One/_apis/build/status/Custom/Networking/ContainerNetworking/NPM%20Conformance%20Tests?branchName=master)](https://dev.azure.com/msazure/One/_build/latest?definitionId=195725&branchName=master)  |



## Overview
This repository contains container networking services and plugins for Linux and Windows containers running on Azure:

* [Azure CNI network and IPAM plugins](docs/cni.md) for Kubernetes.
* [Azure CNM (libnetwork) network and IPAM plugins](docs/cnm.md) for Docker Engine. **(MAINTENANCE MODE)**
* [Azure NPM - Kubernetes Network Policy Manager](docs/npm.md) (Supports only linux for now).

The `azure-vnet` network plugins connect containers to your [Azure VNET](https://docs.microsoft.com/en-us/azure/virtual-network/virtual-networks-overview), to take advantage of Azure SDN capabilities. The `azure-vnet-ipam` IPAM plugins provide address management functionality for container IP addresses allocated from Azure VNET address space.

The following environments are supported:
* [Microsoft Azure](https://azure.microsoft.com): Available in all Azure regions.

Plugins are offered as part of [Azure Kubernetes Service (AKS)](https://docs.microsoft.com/en-us/azure/aks/), as well as for individual Azure IaaS VMs. For Kubernetes clusters created by [aks-engine](https://github.com/Azure/aks-engine), the deployment and configuration of both plugins on both Linux and Windows nodes is automatic and default.

## Documentation
See [Documentation](docs/) for more information and examples.

## Build
This repository builds on Windows and Linux. Build plugins directly from the source code for the latest version.

```bash
$ git clone https://github.com/Azure/azure-container-networking
$ cd azure-container-networking
$ make all-binaries
```

Then follow the instructions for the plugin in [Documentation](docs/).

## Contributions
Contributions in the form of bug reports, feature requests and PRs are always welcome.

Please follow these steps before submitting a PR:
* Create an issue describing the bug or feature request.
* Clone the repository and create a topic branch.
* Make changes, adding new tests for new functionality.
* Submit a PR.

## License
See [LICENSE](LICENSE).

## Code of Conduct
This project has adopted the [Microsoft Open Source Code of Conduct](https://opensource.microsoft.com/codeofconduct/). For more information see the [Code of Conduct FAQ](https://opensource.microsoft.com/codeofconduct/faq/) or contact [opencode@microsoft.com](mailto:opencode@microsoft.com) with any additional questions or comments.
