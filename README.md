# Microsoft Azure Container Networking

[![Build Status](https://msazure.visualstudio.com/One/_apis/build/status/Custom/Networking/ContainerNetworking/Azure.azure-container-networking?branchName=master)](https://msazure.visualstudio.com/One/_build/latest?definitionId=95007&branchName=master) [![Go Report Card](https://goreportcard.com/badge/github.com/Azure/azure-container-networking)](https://goreportcard.com/report/github.com/Azure/azure-container-networking)  ![GitHub release](https://img.shields.io/github/release/Azure/azure-container-networking.svg)

| Azure Network Policy Manager Conformance      |  |
| ----------- | ----------- |
| Cyclonus Network Policy Suite      | [![Cyclonus Network Policy Test](https://github.com/Azure/azure-container-networking/actions/workflows/cyclonus-netpol-test.yaml/badge.svg?branch=master)](https://github.com/Azure/azure-container-networking/actions/workflows/cyclonus-netpol-test.yaml)       |
| Kubernetes Network Policy E2E  | [![Build Status](https://dev.azure.com/msazure/One/_apis/build/status/Custom/Networking/ContainerNetworking/NPM%20Conformance%20Tests?branchName=master)](https://dev.azure.com/msazure/One/_build/latest?definitionId=195725&branchName=master)  |



## Overview
This repository contains container networking services and plugins for Linux and Windows containers running on Azure:

* [Azure CNI network and IPAM plugins](docs/cni.md) for Kubernetes.
* [Azure NPM - Kubernetes Network Policy Manager](docs/npm.md) (Linux and (preview) Windows Server 2022)

The `azure-vnet` network plugins connect containers to your [Azure VNET](https://docs.microsoft.com/en-us/azure/virtual-network/virtual-networks-overview), to take advantage of Azure SDN capabilities. The `azure-vnet-ipam` IPAM plugins provide address management functionality for container IP addresses allocated from Azure VNET address space.

The following environments are supported:
* [Microsoft Azure](https://azure.microsoft.com): Available in all Azure regions.

Plugins are offered as part of [Azure Kubernetes Service (AKS)](https://docs.microsoft.com/en-us/azure/aks/), as well as for individual Azure IaaS VMs. For Kubernetes clusters created by [aks-engine](https://github.com/Azure/aks-engine), the deployment and configuration of both plugins on both Linux and Windows nodes is automatic and default.

The next generation of Azure CNI Plugin is powered by [Cilium](https://cilium.io/). Learn more at [Azure CNI Powered By Cilium](docs/cilium.md)

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

## Docker Image Generation
This repository supports building Docker container images for networking components using standardized make recipes. The build system uses Docker or Podman to create multi-platform images.

### Prerequisites
- Docker or Podman installed and running
- For multi-platform builds: `make qemu-user-static` (Linux only)

### Available Components
- **acncli** - CNI manager
- **azure-ipam** - Azure IP Address Management
- **cni** - Container Network Interface
- **cns** - Container Network Service
- **npm** - Network Policy Manager
- **ipv6-hp-bpf** - IPv6 Host Policy BPF

### Generic Build Pattern
All components follow the same make recipe pattern:

```bash
# Build container image
$ make <component>-image

# View image name and tag
$ make <component>-image-name-and-tag

# Push image to registry
$ make <component>-image-push

# Pull image from registry
$ make <component>-image-pull

# Build multi-platform manifest
$ make <component>-manifest-build
$ make <component>-manifest-push
```

### Example Usage
```bash
# Build CNS image
$ make cns-image

# Build NPM image with custom platform
$ PLATFORM=linux/arm64 make npm-image

# Build Azure-IPAM image with custom registry
$ IMAGE_REGISTRY=myregistry.azurecr.io make azure-ipam-image

# Use Podman instead of Docker
$ CONTAINER_BUILDER=podman make cni-image
```

### Customization
Environment variables for customizing builds:
- `PLATFORM` - Target platform/architecture (default: linux/amd64)
- `IMAGE_REGISTRY` - Custom registry (default: acnpublic.azurecr.io)
- `CONTAINER_BUILDER` - Container builder (default: docker, alternative: podman)

Images are tagged with platform and version information and published to the configured registry.

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
