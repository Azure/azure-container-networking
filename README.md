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
This repository supports building Docker container images for the core networking components. The build system uses Docker or Podman to create multi-platform images.

### Prerequisites
- Docker or Podman installed and running
- For multi-platform builds: `make qemu-user-static` (Linux only)

### Available Components

#### Container Network Service (CNS)
```bash
# Build CNS container image
$ make cns-image

# View image name and tag
$ make cns-image-name-and-tag
```

#### Container Network Interface (CNI)
```bash
# Build CNI container image
$ make cni-image

# View image name and tag
$ make cni-image-name-and-tag
```

#### Network Policy Manager (NPM)
```bash
# Build NPM container image
$ make npm-image

# View image name and tag
$ make npm-image-name-and-tag
```

#### Azure IP Address Management (Azure-IPAM)
```bash
# Build Azure-IPAM container image
$ make azure-ipam-image

# View image name and tag
$ make azure-ipam-image-name-and-tag
```

### Customization Options
You can customize the build by setting environment variables:

```bash
# Build for different platform/architecture
$ PLATFORM=linux/arm64 make cns-image

# Use custom image registry
$ IMAGE_REGISTRY=myregistry.azurecr.io make cns-image

# Use Podman instead of Docker
$ CONTAINER_BUILDER=podman make cns-image
```

### Image Operations
Each component supports additional operations:

```bash
# Push image to registry
$ make cns-image-push

# Pull image from registry
$ make cns-image-pull

# Build multi-platform manifest
$ make cns-manifest-build
$ make cns-manifest-push
```

Images are tagged with platform and version information (e.g., `linux-amd64-v1.2.3`) and published to the `acnpublic.azurecr.io` registry by default.

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
