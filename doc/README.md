# Docker Image Generation

This repository supports building Docker container images for networking components using standardized make recipes. The build system uses Docker or Podman to create multi-platform images.

## Prerequisites
- Docker or Podman installed and running
- For multi-platform builds: `make qemu-user-static` (Linux only)

## Available Components
- **acncli** - CNI manager
- **azure-ipam** - Azure IP Address Management
- **cni** - Container Network Interface
- **cns** - Container Network Service
- **npm** - Network Policy Manager
- **ipv6-hp-bpf** - IPv6 Host Policy BPF

## Generic Build Pattern
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

## Example Usage
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

## Customization
Environment variables for customizing builds:
- `PLATFORM` - Target platform/architecture (default: linux/amd64)
- `IMAGE_REGISTRY` - Custom registry (default: acnpublic.azurecr.io)
- `CONTAINER_BUILDER` - Container builder (default: docker, alternative: podman)

Images are tagged with platform and version information and published to the configured registry.