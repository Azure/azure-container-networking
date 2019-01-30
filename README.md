# Microsoft Azure Container Networking

[![CircleCI](https://circleci.com/gh/Azure/azure-container-networking/tree/master.svg?style=svg)](https://circleci.com/gh/Azure/azure-container-networking/tree/master) [![Go Report Card](https://goreportcard.com/badge/github.com/Azure/azure-container-networking)](https://goreportcard.com/report/github.com/Azure/azure-container-networking) ![GitHub release](https://img.shields.io/github/release/Azure/azure-container-networking.svg)
[![codecov](https://codecov.io/gh/Azure/azure-container-networking/branch/master/graph/badge.svg)](https://codecov.io/gh/Azure/azure-container-networking)

## Overview
This repository contains container networking plugins for Linux and Windows containers running on Azure:

* [CNM (libnetwork) network and IPAM plugins](docs/cnm.md) for Docker Engine.
* [CNI network and IPAM plugins](docs/cni.md) for Kubernetes and DC/OS.

The `azure-vnet` network plugins connect containers to your [Azure VNET](https://docs.microsoft.com/en-us/azure/virtual-network/virtual-networks-overview), to take advantage of Azure SDN capabilities. The `azure-vnet-ipam` IPAM plugins provide address management functionality for container IP addresses allocated from Azure VNET address space.

> Azure VNET plugins are currently available as a **public preview**.

The following environments are supported:
* [Microsoft Azure](https://azure.microsoft.com): Available in all Azure regions.
* [Microsoft Azure Stack](https://azure.microsoft.com/en-us/overview/azure-stack/): The hybrid cloud platform that enables you to deliver Azure services from your own datacenter.

Plugins are offered as part of [Azure Container Service (ACS)](https://azure.microsoft.com/en-us/services/container-service), as well as for individual Azure IaaS VMs. For ACS clusters created by [acs-engine](https://github.com/Azure/acs-engine), the deployment and configuration of both plugins on both Linux and Windows nodes is automatic.

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

## Changelog
See [CHANGELOG](CHANGELOG.md).

## License
See [LICENSE](LICENSE).

## Code of Conduct
This project has adopted the [Microsoft Open Source Code of Conduct](https://opensource.microsoft.com/codeofconduct/). For more information see the [Code of Conduct FAQ](https://opensource.microsoft.com/codeofconduct/faq/) or contact [opencode@microsoft.com](mailto:opencode@microsoft.com) with any additional questions or comments.
