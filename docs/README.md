# Microsoft Azure Container Networking

## Table of Contents
* [CNI plugin](cni.md) - describes how to setup Azure CNI plugins.
* [Azure CNI Powered By Cilium](cilium.md) - describes the next generation of Azure CNI Plugin powered by Cilium dataplane.
* [Azure CNI Overlay Mode for AKS](overlay-for-aks.md) - describes a mode of the Azure CNI Plugin to provide a Pod network from an overlay address space with no encapsulation.
* [ACS](acs.md) - describes how to use the plugins with Azure Container Service.
* [Network](network.md) - describes container networks created by plugins.
* [IPAM](ipam.md) - describes how container IP address management is done by plugins.
* [NPM](npm.md) - describes how to setup Azure-NPM (Azure Network Policy Manager).
* [Scripts](scripts.md) - describes how to use the scripts in this repository.

## Docker Image Generation

This repository supports building Docker container images for networking components using standardized make recipes. The build system uses Docker or Podman to create multi-platform images.

### Prerequisites
- Docker or Podman installed and running
- For multi-platform builds: `make qemu-user-static` (Linux only)

### Available Components
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

### Windows Image Generation
The `GOOS` environment variable has specific impacts on image generation:

#### Operating System Targeting
- **Linux (default)**: `GOOS=linux` builds Linux container images using Ubuntu/Mariner base images
- **Windows**: `GOOS=windows` builds Windows container images using Windows Server Core base images

#### Cross-compilation Behavior
When `GOOS=windows` is set:
- Go binaries are cross-compiled for Windows with `.exe` extensions
- Docker builds use Windows-specific base images (Windows Server Core, Host Process Containers)
- Different Dockerfiles may be used (e.g., `npm/windows.Dockerfile` vs `npm/linux.Dockerfile`)
- The Docker `--target` parameter selects the appropriate build stage (e.g., `windows` vs `linux`)

#### Examples
```bash
# Build Linux image (default)
$ make npm-image

# Build Windows image
$ GOOS=windows make npm-image

# Build Windows image for AMD64
$ GOOS=windows PLATFORM=windows/amd64 make npm-image

# Cross-compile Windows binary in Linux container
$ GOOS=windows make cns-image
```

#### Component-Specific Considerations
- **NPM**: Uses separate Dockerfiles (`linux.Dockerfile`, `windows.Dockerfile`)
- **CNS**: Uses multi-stage Dockerfile with `linux` and `windows` targets
- **CNI**: Supports both Linux and Windows builds from the same Dockerfile
- **Azure-IPAM**: Primarily Linux-focused but supports Windows cross-compilation

## Code of Conduct
This project has adopted the [Microsoft Open Source Code of Conduct](https://opensource.microsoft.com/codeofconduct/). For more information see the [Code of Conduct FAQ](https://opensource.microsoft.com/codeofconduct/faq/) or contact [opencode@microsoft.com](mailto:opencode@microsoft.com) with any additional questions or comments.
