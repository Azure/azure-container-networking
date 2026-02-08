include ./cilium-log-collector/Makefile

.DEFAULT_GOAL := help

# Default platform commands
SHELL		= /bin/bash
MKDIR 	   := mkdir -p
RMDIR 	   := rm -rf
ARCHIVE_CMD = tar -czvf

# Default platform extensions
ARCHIVE_EXT = tgz

# Windows specific commands
ifeq ($(OS),Windows_NT)
MKDIR := powershell.exe -NoProfile -Command New-Item -ItemType Directory -Force
RMDIR := powershell.exe -NoProfile -Command Remove-Item -Recurse -Force
endif

# Build defaults.
GOOS 	 		?= $(shell go env GOOS)
GOARCH   		?= $(shell go env GOARCH)
GOOSES   		?= "linux windows" # To override at the cli do: GOOSES="\"darwin bsd\""
GOARCHES 		?= "amd64 arm64" # To override at the cli do: GOARCHES="\"ppc64 mips\""
LD_BUILD_FLAGS 	?= ""

# Windows specific extensions
# set these based on the GOOS, not the OS
ifeq ($(GOOS),windows)
ARCHIVE_CMD = zip -9lq
ARCHIVE_EXT = zip
EXE_EXT 	= .exe
endif

# Interrogate the git repo and set some variables
REPO_ROOT							?= $(shell git rev-parse --show-toplevel)
REVISION							?= $(shell git rev-parse --short HEAD)
ACN_VERSION							?= $(shell git describe --exclude "cilium-log-collector*" --exclude "azure-iptables-monitor*" --exclude "azure-ip-masq-merger*" --exclude "azure-ipam*" --exclude "dropgz*" --exclude "zapai*" --exclude "ipv6-hp-bpf*" --exclude "azure-block-iptables*" --tags --always)
IPV6_HP_BPF_VERSION					?= $(notdir $(shell git describe --match "ipv6-hp-bpf*" --tags --always))
AZURE_BLOCK_IPTABLES_VERSION		?= $(notdir $(shell git describe --match "azure-block-iptables*" --tags --always))
AZURE_IPAM_VERSION					?= $(notdir $(shell git describe --match "azure-ipam*" --tags --always))
AZURE_IP_MASQ_MERGER_VERSION		?= $(notdir $(shell git describe --match "azure-ip-masq-merger*" --tags --always))
AZURE_IPTABLES_MONITOR_VERSION		?= $(notdir $(shell git describe --match "azure-block-iptables*" --match "azure-iptables-monitor*" --tags --always))
CNI_VERSION							?= $(ACN_VERSION)
CNS_VERSION							?= $(ACN_VERSION)
NPM_VERSION							?= $(ACN_VERSION)
ZAPAI_VERSION						?= $(notdir $(shell git describe --match "zapai*" --tags --always))

# Build directories.
AZURE_IPAM_DIR = $(REPO_ROOT)/azure-ipam
AZURE_IP_MASQ_MERGER_DIR = $(REPO_ROOT)/azure-ip-masq-merger
AZURE_IPTABLES_MONITOR_DIR = $(REPO_ROOT)/azure-iptables-monitor
IPV6_HP_BPF_DIR = $(REPO_ROOT)/bpf-prog/ipv6-hp-bpf
AZURE_BLOCK_IPTABLES_DIR = $(REPO_ROOT)/bpf-prog/azure-block-iptables

CNI_NET_DIR = $(REPO_ROOT)/cni/network/plugin
CNI_IPAM_DIR = $(REPO_ROOT)/cni/ipam/plugin
STATELESS_CNI_NET_DIR = $(REPO_ROOT)/cni/network/stateless
CNI_IPAMV6_DIR = $(REPO_ROOT)/cni/ipam/pluginv6
CNI_TELEMETRY_DIR = $(REPO_ROOT)/cni/telemetry/service
ACNCLI_DIR = $(REPO_ROOT)/tools/acncli
CNS_DIR = $(REPO_ROOT)/cns/service
NPM_DIR = $(REPO_ROOT)/npm/cmd
OUTPUT_DIR = $(REPO_ROOT)/output
BUILD_DIR = $(OUTPUT_DIR)/$(GOOS)_$(GOARCH)
AZURE_IPAM_BUILD_DIR = $(BUILD_DIR)/azure-ipam
AZURE_IP_MASQ_MERGER_BUILD_DIR = $(BUILD_DIR)/azure-ip-masq-merger
AZURE_IPTABLES_MONITOR_BUILD_DIR = $(BUILD_DIR)/azure-iptables-monitor
IPV6_HP_BPF_BUILD_DIR = $(BUILD_DIR)/bpf-prog/ipv6-hp-bpf
AZURE_BLOCK_IPTABLES_BUILD_DIR = $(BUILD_DIR)/bpf-prog/azure-block-iptables
IMAGE_DIR  = $(OUTPUT_DIR)/images

CNI_BUILD_DIR = $(BUILD_DIR)/cni
ACNCLI_BUILD_DIR = $(BUILD_DIR)/acncli
STATELESS_CNI_BUILD_DIR = $(CNI_BUILD_DIR)/stateless
CNI_MULTITENANCY_BUILD_DIR = $(BUILD_DIR)/cni-multitenancy
CNI_MULTITENANCY_TRANSPARENT_VLAN_BUILD_DIR = $(BUILD_DIR)/cni-multitenancy-transparent-vlan
CNI_SWIFT_BUILD_DIR = $(BUILD_DIR)/cni-swift
CNI_OVERLAY_BUILD_DIR = $(BUILD_DIR)/cni-overlay
CNI_BAREMETAL_BUILD_DIR = $(BUILD_DIR)/cni-baremetal
CNI_DUALSTACK_BUILD_DIR = $(BUILD_DIR)/cni-dualstack
CNS_BUILD_DIR = $(BUILD_DIR)/cns
NPM_BUILD_DIR = $(BUILD_DIR)/npm
CNI_AI_ID = 5515a1eb-b2bc-406a-98eb-ba462e6f0411
CNS_AI_ID = ce672799-8f08-4235-8c12-08563dc2acef
NPM_AI_ID = 014c22bd-4107-459e-8475-67909e96edcb
ACN_PACKAGE_PATH = github.com/Azure/azure-container-networking
CNI_AI_PATH=$(ACN_PACKAGE_PATH)/telemetry.aiMetadata
CNS_AI_PATH=$(ACN_PACKAGE_PATH)/cns/logger.aiMetadata
NPM_AI_PATH=$(ACN_PACKAGE_PATH)/npm.aiMetadata


# Archive file names.
ACNCLI_ARCHIVE_NAME = acncli-$(GOOS)-$(GOARCH)-$(ACN_VERSION).$(ARCHIVE_EXT)
CNI_ARCHIVE_NAME = azure-vnet-cni-$(GOOS)-$(GOARCH)-$(CNI_VERSION).$(ARCHIVE_EXT)
CNI_MULTITENANCY_ARCHIVE_NAME = azure-vnet-cni-multitenancy-$(GOOS)-$(GOARCH)-$(CNI_VERSION).$(ARCHIVE_EXT)
CNI_MULTITENANCY_TRANSPARENT_VLAN_ARCHIVE_NAME = azure-vnet-cni-multitenancy-transparent-vlan-$(GOOS)-$(GOARCH)-$(CNI_VERSION).$(ARCHIVE_EXT)
CNI_SWIFT_ARCHIVE_NAME = azure-vnet-cni-swift-$(GOOS)-$(GOARCH)-$(CNI_VERSION).$(ARCHIVE_EXT)
CNI_OVERLAY_ARCHIVE_NAME = azure-vnet-cni-overlay-$(GOOS)-$(GOARCH)-$(CNI_VERSION).$(ARCHIVE_EXT)
CNI_BAREMETAL_ARCHIVE_NAME = azure-vnet-cni-baremetal-$(GOOS)-$(GOARCH)-$(CNI_VERSION).$(ARCHIVE_EXT)
CNI_DUALSTACK_ARCHIVE_NAME = azure-vnet-cni-overlay-dualstack-$(GOOS)-$(GOARCH)-$(CNI_VERSION).$(ARCHIVE_EXT)

CNS_ARCHIVE_NAME = azure-cns-$(GOOS)-$(GOARCH)-$(CNS_VERSION).$(ARCHIVE_EXT)
NPM_ARCHIVE_NAME = azure-npm-$(GOOS)-$(GOARCH)-$(NPM_VERSION).$(ARCHIVE_EXT)
AZURE_IPAM_ARCHIVE_NAME = azure-ipam-$(GOOS)-$(GOARCH)-$(AZURE_IPAM_VERSION).$(ARCHIVE_EXT)
AZURE_IP_MASQ_MERGER_ARCHIVE_NAME = azure-ip-masq-merger-$(GOOS)-$(GOARCH)-$(AZURE_IP_MASQ_MERGER_VERSION).$(ARCHIVE_EXT)
AZURE_IPTABLES_MONITOR_ARCHIVE_NAME = azure-iptables-monitor-$(GOOS)-$(GOARCH)-$(AZURE_IPTABLES_MONITOR_VERSION).$(ARCHIVE_EXT)
IPV6_HP_BPF_ARCHIVE_NAME = ipv6-hp-bpf-$(GOOS)-$(GOARCH)-$(IPV6_HP_BPF_VERSION).$(ARCHIVE_EXT)
AZURE_BLOCK_IPTABLES_ARCHIVE_NAME = azure-block-iptables-$(GOOS)-$(GOARCH)-$(AZURE_BLOCK_IPTABLES_VERSION).$(ARCHIVE_EXT)

# Image info file names.
CNI_IMAGE_INFO_FILE			= azure-cni-$(CNI_VERSION).txt
CNS_IMAGE_INFO_FILE			= azure-cns-$(CNS_VERSION).txt
NPM_IMAGE_INFO_FILE			= azure-npm-$(NPM_VERSION).txt

#Tools paths
TOOLS_GO_MOD = $(REPO_ROOT)/tools.go.mod

# Default target
all-binaries-platforms: ## Make all platform binaries
	@for goos in "$(GOOSES)"; do \
		for goarch in "$(GOARCHES)"; do \
			make all-binaries GOOS=$$goos GOARCH=$$goarch; \
		done \
	done

# OS specific binaries/images
ifeq ($(GOOS),linux)
all-binaries: acncli azure-cni-plugin azure-cns azure-npm azure-ipam azure-ip-masq-merger azure-iptables-monitor ipv6-hp-bpf azure-block-iptables cilium-log-collector
all-images: npm-image cns-image cni-manager-image azure-ip-masq-merger-image azure-iptables-monitor-image ipv6-hp-bpf-image cilium-log-collector
else
all-binaries: azure-cni-plugin azure-cns azure-npm
all-images:
	@echo "Nothing to build. Skip."
endif

# Shorthand target names for convenience.
azure-cni-plugin: azure-vnet-binary azure-vnet-stateless-binary azure-vnet-ipam-binary azure-vnet-ipamv6-binary azure-vnet-telemetry-binary cni-archive
azure-cns: azure-cns-binary cns-archive
acncli: acncli-binary acncli-archive
azure-npm: azure-npm-binary npm-archive
azure-ipam: azure-ipam-binary azure-ipam-archive
ipv6-hp-bpf: ipv6-hp-bpf-binary ipv6-hp-bpf-archive
azure-block-iptables: azure-block-iptables-binary azure-block-iptables-archive
azure-ip-masq-merger: azure-ip-masq-merger-binary azure-ip-masq-merger-archive
azure-iptables-monitor: azure-iptables-monitor-binary azure-iptables-monitor-archive


##@ Versioning

revision: ## print the current git revision
	@echo $(REVISION)

version: ## prints the root version
	@echo $(ACN_VERSION)

acncli-version: version

azure-ipam-version: ## prints the azure-ipam version
	@echo $(AZURE_IPAM_VERSION)

azure-ip-masq-merger-version: ## prints the azure-ip-masq-merger version
	@echo $(AZURE_IP_MASQ_MERGER_VERSION)

azure-iptables-monitor-version: ## prints the azure-iptables-monitor version
	@echo $(AZURE_IPTABLES_MONITOR_VERSION)

ipv6-hp-bpf-version: ## prints the ipv6-hp-bpf version
	@echo $(IPV6_HP_BPF_VERSION)

azure-block-iptables-version: ## prints the azure-block-iptables version
	@echo $(AZURE_BLOCK_IPTABLES_VERSION)

cni-version: ## prints the cni version
	@echo $(CNI_VERSION)

cns-version:
	@echo $(CNS_VERSION)

npm-version:
	@echo $(NPM_VERSION)

zapai-version: ## prints the zapai version
	@echo $(ZAPAI_VERSION)

##@ Binaries

# Build the delegated IPAM plugin binary.
azure-ipam-binary:
	cd $(AZURE_IPAM_DIR) && CGO_ENABLED=0 go build -v -o $(AZURE_IPAM_BUILD_DIR)/azure-ipam$(EXE_EXT) -ldflags "-X github.com/Azure/azure-container-networking/azure-ipam/internal/buildinfo.Version=$(AZURE_IPAM_VERSION) $(LD_BUILD_FLAGS)" -gcflags="-dwarflocationlists=true"

# Build the ipv6-hp-bpf binary.
ipv6-hp-bpf-binary: bpf-lib
	cd $(IPV6_HP_BPF_DIR) && CGO_ENABLED=0 go generate ./...
	cd $(IPV6_HP_BPF_DIR)/cmd/ipv6-hp-bpf && CGO_ENABLED=0 go build -v -o $(IPV6_HP_BPF_BUILD_DIR)/ipv6-hp-bpf$(EXE_EXT) -ldflags "-X main.version=$(IPV6_HP_BPF_VERSION) $(LD_BUILD_FLAGS)" -gcflags="-dwarflocationlists=true"

# Libraries for bpf
bpf-lib:
ifeq ($(GOARCH),amd64)
	sudo apt-get update && sudo apt-get install -y llvm clang linux-libc-dev libbpf-dev libc6-dev nftables iproute2 gcc-multilib
	for dir in /usr/include/x86_64-linux-gnu/*; do sudo ln -sfn "$$dir" /usr/include/$$(basename "$$dir"); done
else ifeq ($(GOARCH),arm64)
	sudo apt-get update && sudo apt-get install -y llvm clang linux-libc-dev libbpf-dev libc6-dev nftables iproute2 gcc-aarch64-linux-gnu
	for dir in /usr/include/aarch64-linux-gnu/*; do sudo ln -sfn "$$dir" /usr/include/$$(basename "$$dir"); done
endif

# Build the azure-block-iptables binary.
azure-block-iptables-binary: bpf-lib
	cd $(AZURE_BLOCK_IPTABLES_DIR) && CGO_ENABLED=0 go generate ./...
	cd $(AZURE_BLOCK_IPTABLES_DIR)/cmd/azure-block-iptables && CGO_ENABLED=0 go build -v -o $(AZURE_BLOCK_IPTABLES_BUILD_DIR)/azure-block-iptables$(EXE_EXT) -ldflags "-X main.version=$(AZURE_BLOCK_IPTABLES_VERSION)" -gcflags="-dwarflocationlists=true"

# Build the Azure CNI network binary.
azure-vnet-binary:
	cd $(CNI_NET_DIR) && CGO_ENABLED=0 go build -v -o $(CNI_BUILD_DIR)/azure-vnet$(EXE_EXT) -ldflags "-X main.version=$(CNI_VERSION) $(LD_BUILD_FLAGS)" -gcflags="-dwarflocationlists=true"

# Build the Azure CNI stateless network binary
azure-vnet-stateless-binary:
	cd $(STATELESS_CNI_NET_DIR) && CGO_ENABLED=0 go build -v -o $(STATELESS_CNI_BUILD_DIR)/azure-vnet$(EXE_EXT) -ldflags "-X main.version=$(CNI_VERSION) $(LD_BUILD_FLAGS)" -gcflags="-dwarflocationlists=true"

# Build the Azure CNI IPAM binary.
azure-vnet-ipam-binary:
	cd $(CNI_IPAM_DIR) && CGO_ENABLED=0 go build -v -o $(CNI_BUILD_DIR)/azure-vnet-ipam$(EXE_EXT) -ldflags "-X main.version=$(CNI_VERSION) $(LD_BUILD_FLAGS)" -gcflags="-dwarflocationlists=true"

# Build the Azure CNI IPAMV6 binary.
azure-vnet-ipamv6-binary:
	cd $(CNI_IPAMV6_DIR) && CGO_ENABLED=0 go build -v -o $(CNI_BUILD_DIR)/azure-vnet-ipamv6$(EXE_EXT) -ldflags "-X main.version=$(CNI_VERSION) $(LD_BUILD_FLAGS)" -gcflags="-dwarflocationlists=true"

# Build the Azure CNI telemetry binary.
azure-vnet-telemetry-binary:
	cd $(CNI_TELEMETRY_DIR) && CGO_ENABLED=0 go build -v -o $(CNI_BUILD_DIR)/azure-vnet-telemetry$(EXE_EXT) -ldflags "-X main.version=$(CNI_VERSION) -X $(CNI_AI_PATH)=$(CNI_AI_ID) $(LD_BUILD_FLAGS)" -gcflags="-dwarflocationlists=true"

# Build the Azure CLI network binary.
acncli-binary:
	cd $(ACNCLI_DIR) && CGO_ENABLED=0 go build -v -o $(ACNCLI_BUILD_DIR)/acn$(EXE_EXT) -ldflags "-X main.version=$(ACN_VERSION) $(LD_BUILD_FLAGS)" -gcflags="-dwarflocationlists=true"

# Build the Azure CNS binary.
azure-cns-binary:
	cd $(CNS_DIR) && CGO_ENABLED=0 go build -v -o $(CNS_BUILD_DIR)/azure-cns$(EXE_EXT) -ldflags "-X main.version=$(CNS_VERSION) -X $(CNS_AI_PATH)=$(CNS_AI_ID) -X $(CNI_AI_PATH)=$(CNI_AI_ID) $(LD_BUILD_FLAGS)" -gcflags="-dwarflocationlists=true"

# Build the Azure NPM binary.
azure-npm-binary:
	cd $(CNI_TELEMETRY_DIR) && CGO_ENABLED=0 go build -v -o $(NPM_BUILD_DIR)/azure-vnet-telemetry$(EXE_EXT) -ldflags "-X main.version=$(NPM_VERSION) $(LD_BUILD_FLAGS)" -gcflags="-dwarflocationlists=true"
	cd $(NPM_DIR) && CGO_ENABLED=0 go build -v -o $(NPM_BUILD_DIR)/azure-npm$(EXE_EXT) -ldflags "-X main.version=$(NPM_VERSION) -X $(NPM_AI_PATH)=$(NPM_AI_ID) $(LD_BUILD_FLAGS)" -gcflags="-dwarflocationlists=true"

# Build the azure-ip-masq-merger binary.
azure-ip-masq-merger-binary:
	cd $(AZURE_IP_MASQ_MERGER_DIR) && CGO_ENABLED=0 go build -v -o $(AZURE_IP_MASQ_MERGER_BUILD_DIR)/azure-ip-masq-merger$(EXE_EXT) -ldflags "-X main.version=$(AZURE_IP_MASQ_MERGER_VERSION) $(LD_BUILD_FLAGS)" -gcflags="-dwarflocationlists=true"

# Build the azure-iptables-monitor binary.
azure-iptables-monitor-binary:
	cd $(AZURE_IPTABLES_MONITOR_DIR) && CGO_ENABLED=0 go build -v -o $(AZURE_IPTABLES_MONITOR_BUILD_DIR)/azure-iptables-monitor$(EXE_EXT) -ldflags "-X main.version=$(AZURE_IPTABLES_MONITOR_VERSION)" -gcflags="-dwarflocationlists=true"

##@ Containers

## Common variables for all containers.
IMAGE_REGISTRY      ?= acnpublic.azurecr.io
OS                  ?= $(GOOS)
ARCH                ?= $(GOARCH)
PLATFORM            ?= $(OS)/$(ARCH)
BUILDX_ACTION  		?= --load
CONTAINER_BUILDER   ?= buildah
CONTAINER_RUNTIME   ?= podman
CONTAINER_TRANSPORT ?= skopeo


# prefer buildah, if available, but fall back to docker if that binary is not in the path or on Windows.
ifeq (, $(shell which $(CONTAINER_BUILDER)))
CONTAINER_BUILDER = docker
endif
ifeq ($(OS), windows)
CONTAINER_BUILDER = docker
endif

# prefer podman, if available, but fall back to docker if that binary is not in the path or on Windows.
ifeq (, $(shell which $(CONTAINER_RUNTIME)))
CONTAINER_RUNTIME = docker
endif
ifeq ($(OS), windows)
CONTAINER_RUNTIME = docker
endif

# prefer skopeo, if available, but fall back to docker if that binary is not in the path. or on Windows
ifeq (, $(shell which $(CONTAINER_TRANSPORT)))
CONTAINER_TRANSPORT = docker
endif
ifeq ($(OS), windows)
CONTAINER_TRANSPORT = docker
endif

## Image name definitions.
ACNCLI_IMAGE					= acncli
AZURE_IPAM_IMAGE				= azure-ipam
IPV6_HP_BPF_IMAGE				= ipv6-hp-bpf
CNI_IMAGE						= azure-cni
CNS_IMAGE						= azure-cns
NPM_IMAGE						= azure-npm
AZURE_IP_MASQ_MERGER_IMAGE		= azure-ip-masq-merger
AZURE_IPTABLES_MONITOR_IMAGE	= azure-iptables-monitor

## Image platform tags.
ACNCLI_PLATFORM_TAG					?= $(subst /,-,$(PLATFORM))-$(ACN_VERSION)
AZURE_IPAM_PLATFORM_TAG				?= $(subst /,-,$(PLATFORM))-$(AZURE_IPAM_VERSION)
AZURE_IPAM_WINDOWS_PLATFORM_TAG		?= $(subst /,-,$(PLATFORM))-$(AZURE_IPAM_VERSION)-$(OS_SKU_WIN)
IPV6_HP_BPF_IMAGE_PLATFORM_TAG		?= $(subst /,-,$(PLATFORM))-$(IPV6_HP_BPF_VERSION)
CNI_PLATFORM_TAG					?= $(subst /,-,$(PLATFORM))-$(CNI_VERSION)
CNI_WINDOWS_PLATFORM_TAG			?= $(subst /,-,$(PLATFORM))-$(CNI_VERSION)-$(OS_SKU_WIN)
CNS_PLATFORM_TAG					?= $(subst /,-,$(PLATFORM))-$(CNS_VERSION)
CNS_WINDOWS_PLATFORM_TAG			?= $(subst /,-,$(PLATFORM))-$(CNS_VERSION)-$(OS_SKU_WIN)
NPM_PLATFORM_TAG					?= $(subst /,-,$(PLATFORM))-$(NPM_VERSION)
AZURE_IP_MASQ_MERGER_PLATFORM_TAG	?= $(subst /,-,$(PLATFORM))-$(AZURE_IP_MASQ_MERGER_VERSION)
AZURE_IPTABLES_MONITOR_PLATFORM_TAG	?= $(subst /,-,$(PLATFORM))-$(AZURE_IPTABLES_MONITOR_VERSION)


qemu-user-static: ## Set up the host to run qemu multiplatform container builds.
	sudo $(CONTAINER_RUNTIME) run --rm --privileged multiarch/qemu-user-static --reset -p yes


## Reusable build targets for building individual container images.

container-buildah: # util target to build container images using buildah. do not invoke directly.
	buildah bud \
		--build-arg ARCH=$(ARCH) \
		--build-arg OS=$(OS) \
		--build-arg PLATFORM=$(PLATFORM) \
		--build-arg VERSION=$(TAG) \
		$(EXTRA_BUILD_ARGS) \
		--jobs 16 \
		--platform $(PLATFORM) \
		--target $(TARGET) \
		-f $(DOCKERFILE) \
		-t $(IMAGE_REGISTRY)/$(IMAGE):$(TAG) \
		.
	buildah push $(IMAGE_REGISTRY)/$(IMAGE):$(TAG)

container-docker: # util target to build container images using docker buildx. do not invoke directly.
	docker buildx create --use --driver-opt image=mcr.microsoft.com/oss/v2/moby/buildkit:v0.16.0-2 --platform $(PLATFORM)
	docker buildx build \
		$(BUILDX_ACTION) \
		--build-arg ARCH=$(ARCH) \
		--build-arg OS=$(OS) \
		--build-arg PLATFORM=$(PLATFORM) \
		--build-arg VERSION=$(TAG) \
		$(EXTRA_BUILD_ARGS) \
		--platform $(PLATFORM) \
		--target $(TARGET) \
		-f $(DOCKERFILE) \
		-t $(IMAGE_REGISTRY)/$(IMAGE):$(TAG) \
		.

container: # util target to build container images. do not invoke directly.
	$(MAKE) container-$(CONTAINER_BUILDER) \
		ARCH=$(ARCH) \
		OS=$(OS) \
		PLATFORM=$(PLATFORM) \
		TAG=$(TAG) \
		TARGET=$(TARGET)

container-push: # util target to publish container image. do not invoke directly.
	$(CONTAINER_BUILDER) push \
		$(IMAGE_REGISTRY)/$(IMAGE):$(TAG)

container-pull: # util target to pull container image. do not invoke directly.
	$(CONTAINER_BUILDER) pull \
		$(IMAGE_REGISTRY)/$(IMAGE):$(TAG)


## Build specific container images.

# acncli

acncli-image-name: # util target to print the CNI manager image name.
	@echo $(ACNCLI_IMAGE)

acncli-image-name-and-tag: # util target to print the CNI manager image name and tag.
	@echo $(IMAGE_REGISTRY)/$(ACNCLI_IMAGE):$(ACNCLI_PLATFORM_TAG)

acncli-image: ## build cni-manager container image.
	$(MAKE) container \
		DOCKERFILE=tools/acncli/Dockerfile \
		IMAGE=$(ACNCLI_IMAGE) \
		TAG=$(ACNCLI_PLATFORM_TAG)

acncli-image-push: ## push cni-manager container image.
	$(MAKE) container-push \
		IMAGE=$(ACNCLI_IMAGE) \
		TAG=$(ACNCLI_PLATFORM_TAG)

acncli-image-pull: ## pull cni-manager container image.
	$(MAKE) container-pull \
		IMAGE=$(ACNCLI_IMAGE) \
		TAG=$(ACNCLI_PLATFORM_TAG)


# azure-ipam

azure-ipam-image-name: # util target to print the azure-ipam  image name.
	@echo $(AZURE_IPAM_IMAGE)

azure-ipam-image-name-and-tag: # util target to print the azure-ipam image name and tag.
	@echo $(IMAGE_REGISTRY)/$(AZURE_IPAM_IMAGE):$(AZURE_IPAM_PLATFORM_TAG)

azure-ipam-image: ## build azure-ipam container image.
	$(MAKE) container \
		DOCKERFILE=azure-ipam/Dockerfile \
		IMAGE=$(AZURE_IPAM_IMAGE) \
		PLATFORM=$(PLATFORM) \
		TAG=$(AZURE_IPAM_PLATFORM_TAG) \
		TARGET=$(OS) \
		OS=$(OS) \
		ARCH=$(ARCH)

azure-ipam-image-push: ## push azure-ipam container image.
	$(MAKE) container-push \
		IMAGE=$(AZURE_IPAM_IMAGE) \
		TAG=$(AZURE_IPAM_PLATFORM_TAG)

azure-ipam-image-pull: ## pull azure-ipam container image.
	$(MAKE) container-pull \
		IMAGE=$(AZURE_IPAM_IMAGE) \
		TAG=$(AZURE_IPAM_PLATFORM_TAG)

# azure-ip-masq-merger
azure-ip-masq-merger-image-name: # util target to print the azure-ip-masq-merger image name.
	@echo $(AZURE_IP_MASQ_MERGER_IMAGE)

azure-ip-masq-merger-image-name-and-tag: # util target to print the azure-ip-masq-merger image name and tag.
	@echo $(IMAGE_REGISTRY)/$(AZURE_IP_MASQ_MERGER_IMAGE):$(AZURE_IP_MASQ_MERGER_PLATFORM_TAG)

azure-ip-masq-merger-image: ## build azure-ip-masq-merger container image.
	$(MAKE) container \
		DOCKERFILE=azure-ip-masq-merger/Dockerfile \
		IMAGE=$(AZURE_IP_MASQ_MERGER_IMAGE) \
		PLATFORM=$(PLATFORM) \
		TAG=$(AZURE_IP_MASQ_MERGER_PLATFORM_TAG) \
		TARGET=$(OS) \
		OS=$(OS) \
		ARCH=$(ARCH)

azure-ip-masq-merger-image-push: ## push azure-ip-masq-merger container image.
	$(MAKE) container-push \
		IMAGE=$(AZURE_IP_MASQ_MERGER_IMAGE) \
		TAG=$(AZURE_IP_MASQ_MERGER_PLATFORM_TAG)

azure-ip-masq-merger-image-pull: ## pull azure-ip-masq-merger container image.
	$(MAKE) container-pull \
		IMAGE=$(AZURE_IP_MASQ_MERGER_IMAGE) \
		TAG=$(AZURE_IP_MASQ_MERGER_PLATFORM_TAG)

# azure-iptables-monitor
azure-iptables-monitor-image-name: # util target to print the azure-iptables-monitor image name.
	@echo $(AZURE_IPTABLES_MONITOR_IMAGE)

azure-iptables-monitor-image-name-and-tag: # util target to print the azure-iptables-monitor image name and tag.
	@echo $(IMAGE_REGISTRY)/$(AZURE_IPTABLES_MONITOR_IMAGE):$(AZURE_IPTABLES_MONITOR_PLATFORM_TAG)

azure-iptables-monitor-image: ## build azure-iptables-monitor container image.
	$(MAKE) container \
		DOCKERFILE=azure-iptables-monitor/Dockerfile \
		IMAGE=$(AZURE_IPTABLES_MONITOR_IMAGE) \
		PLATFORM=$(PLATFORM) \
		TAG=$(AZURE_IPTABLES_MONITOR_PLATFORM_TAG) \
		TARGET=$(OS) \
		OS=$(OS) \
		ARCH=$(ARCH) \
		EXTRA_BUILD_ARGS="--build-arg AZURE_BLOCK_IPTABLES_VERSION=$(AZURE_BLOCK_IPTABLES_VERSION)"

azure-iptables-monitor-image-push: ## push azure-iptables-monitor container image.
	$(MAKE) container-push \
		IMAGE=$(AZURE_IPTABLES_MONITOR_IMAGE) \
		TAG=$(AZURE_IPTABLES_MONITOR_PLATFORM_TAG)

azure-iptables-monitor-image-pull: ## pull azure-iptables-monitor container image.
	$(MAKE) container-pull \
		IMAGE=$(AZURE_IPTABLES_MONITOR_IMAGE) \
		TAG=$(AZURE_IPTABLES_MONITOR_PLATFORM_TAG)

# ipv6-hp-bpf

ipv6-hp-bpf-image-name: # util target to print the ipv6-hp-bpf image name.
	@echo $(IPV6_HP_BPF_IMAGE)

ipv6-hp-bpf-image-name-and-tag: # util target to print the ipv6-hp-bpf image name and tag.
	@echo $(IMAGE_REGISTRY)/$(IPV6_HP_BPF_IMAGE):$(IPV6_HP_BPF_IMAGE_PLATFORM_TAG)

ipv6-hp-bpf-image: ## build ipv6-hp-bpf container image.
	$(MAKE) container \
		DOCKERFILE=bpf-prog/ipv6-hp-bpf/$(OS).Dockerfile \
		IMAGE=$(IPV6_HP_BPF_IMAGE) \
		EXTRA_BUILD_ARGS='--build-arg OS=$(OS) --build-arg ARCH=$(ARCH) --build-arg DEBUG=$(DEBUG)'\
		PLATFORM=$(PLATFORM) \
		TAG=$(IPV6_HP_BPF_IMAGE_PLATFORM_TAG) \
		TARGET=$(OS) \
		OS=$(OS) \
		ARCH=$(ARCH)

ipv6-hp-bpf-image-push: ## push ipv6-hp-bpf container image.
	$(MAKE) container-push \
		IMAGE=$(IPV6_HP_BPF_IMAGE) \
		TAG=$(IPV6_HP_BPF_IMAGE_PLATFORM_TAG)

ipv6-hp-bpf-image-pull: ## pull ipv6-hp-bpf container image.
	$(MAKE) container-pull \
		IMAGE=$(IPV6_HP_BPF_IMAGE) \
		TAG=$(IPV6_HP_BPF_IMAGE_PLATFORM_TAG)

# cni

cni-image-name: # util target to print the cni image name.
	@echo $(CNI_IMAGE)

cni-image-name-and-tag: # util target to print the cni image name and tag.
	@echo $(IMAGE_REGISTRY)/$(CNI_IMAGE):$(CNI_PLATFORM_TAG)

cni-image: ## build cni container image.
	$(MAKE) container \
		DOCKERFILE=cni/Dockerfile \
		IMAGE=$(CNI_IMAGE) \
		PLATFORM=$(PLATFORM) \
		TAG=$(CNI_PLATFORM_TAG) \
		TARGET=$(OS) \
		OS=$(OS) \
		ARCH=$(ARCH) \
		EXTRA_BUILD_ARGS='--build-arg CNI_AI_PATH=$(CNI_AI_PATH) --build-arg CNI_AI_ID=$(CNI_AI_ID)'

cni-image-push: ## push cni container image.
	$(MAKE) container-push \
		IMAGE=$(CNI_IMAGE) \
		TAG=$(CNI_PLATFORM_TAG)

cni-image-pull: ## pull cni container image.
	$(MAKE) container-pull \
		IMAGE=$(CNI_IMAGE) \
		TAG=$(CNI_PLATFORM_TAG)


# cns

cns-image-name: # util target to print the CNS image name
	@echo $(CNS_IMAGE)

cns-image-name-and-tag: # util target to print the CNS image name and tag.
	@echo $(IMAGE_REGISTRY)/$(CNS_IMAGE):$(CNS_PLATFORM_TAG)

cns-image: ## build cns container image.
	$(MAKE) container \
		DOCKERFILE=cns/Dockerfile \
		IMAGE=$(CNS_IMAGE) \
		EXTRA_BUILD_ARGS='--build-arg CNS_AI_PATH=$(CNS_AI_PATH) --build-arg CNS_AI_ID=$(CNS_AI_ID)' \
		PLATFORM=$(PLATFORM) \
		TAG=$(CNS_PLATFORM_TAG) \
		TARGET=$(OS) \
		OS=$(OS) \
		ARCH=$(ARCH)

cns-image-push: ## push cns container image.
	$(MAKE) container-push \
		IMAGE=$(CNS_IMAGE) \
		TAG=$(CNS_PLATFORM_TAG)

cns-image-pull: ## pull cns container image.
	$(MAKE) container-pull \
		IMAGE=$(CNS_IMAGE) \
		TAG=$(CNS_PLATFORM_TAG)

# npm

npm-image-name: # util target to print the NPM image name
	@echo $(NPM_IMAGE)

npm-image-name-and-tag: # util target to print the NPM image name and tag.
	@echo $(IMAGE_REGISTRY)/$(NPM_IMAGE):$(NPM_PLATFORM_TAG)

npm-image: ## build the npm container image.
	$(MAKE) container-$(CONTAINER_BUILDER) \
		DOCKERFILE=npm/$(OS).Dockerfile \
		IMAGE=$(NPM_IMAGE) \
		EXTRA_BUILD_ARGS='--build-arg NPM_AI_PATH=$(NPM_AI_PATH) --build-arg NPM_AI_ID=$(NPM_AI_ID)' \
		PLATFORM=$(PLATFORM) \
		TAG=$(NPM_PLATFORM_TAG) \
		TARGET=$(OS) \
		OS=$(OS) \
		ARCH=$(ARCH)

npm-image-push: ## push npm container image.
	$(MAKE) container-push \
		IMAGE=$(NPM_IMAGE) \
		TAG=$(NPM_PLATFORM_TAG)

npm-image-pull: ## pull cns container image.
	$(MAKE) container-pull \
		IMAGE=$(NPM_IMAGE) \
		TAG=$(NPM_PLATFORM_TAG)

## Reusable targets for building multiplat container image manifests.

IMAGE_ARCHIVE_DIR ?= $(shell pwd)

manifest-create:
	$(CONTAINER_BUILDER) manifest create $(IMAGE_REGISTRY)/$(IMAGE):$(TAG)

manifest-add:
	$(CONTAINER_BUILDER) manifest add --os=$(OS) $(IMAGE_REGISTRY)/$(IMAGE):$(TAG) docker://$(IMAGE_REGISTRY)/$(IMAGE):$(subst /,-,$(PLATFORM))-$(TAG)

manifest-build: # util target to compose multiarch container manifests from platform specific images.
	$(MAKE) manifest-create
	$(foreach PLATFORM,$(PLATFORMS),\
		$(if $(filter $(PLATFORM),windows/amd64),\
			$(MAKE) manifest-add CONTAINER_BUILDER=$(CONTAINER_BUILDER) OS=windows OS_VERSION=$(OS_VERSION) PLATFORM=$(PLATFORM);,\
		$(MAKE) manifest-add PLATFORM=$(PLATFORM);\
		)\
	)\

manifest-push: # util target to push multiarch container manifest.
	$(CONTAINER_BUILDER) manifest push --all $(IMAGE_REGISTRY)/$(IMAGE):$(TAG) docker://$(IMAGE_REGISTRY)/$(IMAGE):$(TAG)

manifest-skopeo-archive: # util target to export tar archive of multiarch container manifest.
	skopeo copy --all docker://$(IMAGE_REGISTRY)/$(IMAGE):$(TAG) oci-archive:$(IMAGE_ARCHIVE_DIR)/$(IMAGE)-$(TAG).tar --debug

## Build specific multiplat images.

acncli-manifest-build: ## build acncli multiplat container manifest.
	$(MAKE) manifest-build \
		PLATFORMS="$(PLATFORMS)" \
		IMAGE=$(ACNCLI_IMAGE) \
		TAG=$(ACN_VERSION)

acncli-manifest-push: ## push acncli multiplat container manifest
	$(MAKE) manifest-push \
		IMAGE=$(ACNCLI_IMAGE) \
		TAG=$(ACN_VERSION)

acncli-skopeo-archive: ## export tar archive of acncli multiplat container manifest.
	$(MAKE) manifest-skopeo-archive \
		IMAGE=$(ACNCLI_IMAGE) \
		TAG=$(ACN_VERSION)

azure-ipam-manifest-build: ## build azure-ipam multiplat container manifest.
	$(MAKE) manifest-build \
		PLATFORMS="$(PLATFORMS)" \
		IMAGE=$(AZURE_IPAM_IMAGE) \
		TAG=$(AZURE_IPAM_VERSION)

azure-ipam-manifest-push: ## push azure-ipam multiplat container manifest
	$(MAKE) manifest-push \
		IMAGE=$(AZURE_IPAM_IMAGE) \
		TAG=$(AZURE_IPAM_VERSION)

azure-ipam-skopeo-archive: ## export tar archive of azure-ipam multiplat container manifest.
	$(MAKE) manifest-skopeo-archive \
		IMAGE=$(AZURE_IPAM_IMAGE) \
		TAG=$(AZURE_IPAM_VERSION)

azure-ip-masq-merger-manifest-build: ## build azure-ip-masq-merger multiplat container manifest.
	$(MAKE) manifest-build \
		PLATFORMS="$(PLATFORMS)" \
		IMAGE=$(AZURE_IP_MASQ_MERGER_IMAGE) \
		TAG=$(AZURE_IP_MASQ_MERGER_VERSION)

azure-ip-masq-merger-manifest-push: ## push azure-ip-masq-merger multiplat container manifest
	$(MAKE) manifest-push \
		IMAGE=$(AZURE_IP_MASQ_MERGER_IMAGE) \
		TAG=$(AZURE_IP_MASQ_MERGER_VERSION)

azure-ip-masq-merger-skopeo-archive: ## export tar archive of azure-ip-masq-merger multiplat container manifest.
	$(MAKE) manifest-skopeo-archive \
		IMAGE=$(AZURE_IP_MASQ_MERGER_IMAGE) \
		TAG=$(AZURE_IP_MASQ_MERGER_VERSION)

azure-iptables-monitor-manifest-build: ## build azure-iptables-monitor multiplat container manifest.
	$(MAKE) manifest-build \
		PLATFORMS="$(PLATFORMS)" \
		IMAGE=$(AZURE_IPTABLES_MONITOR_IMAGE) \
		TAG=$(AZURE_IPTABLES_MONITOR_VERSION)

azure-iptables-monitor-manifest-push: ## push azure-iptables-monitor multiplat container manifest
	$(MAKE) manifest-push \
		IMAGE=$(AZURE_IPTABLES_MONITOR_IMAGE) \
		TAG=$(AZURE_IPTABLES_MONITOR_VERSION)

azure-iptables-monitor-skopeo-archive: ## export tar archive of azure-iptables-monitor multiplat container manifest.
	$(MAKE) manifest-skopeo-archive \
		IMAGE=$(AZURE_IPTABLES_MONITOR_IMAGE) \
		TAG=$(AZURE_IPTABLES_MONITOR_VERSION)

ipv6-hp-bpf-manifest-build: ## build ipv6-hp-bpf multiplat container manifest.
	$(MAKE) manifest-build \
		PLATFORMS="$(PLATFORMS)" \
		IMAGE=$(IPV6_HP_BPF_IMAGE) \
		TAG=$(IPV6_HP_BPF_VERSION)

ipv6-hp-bpf-manifest-push: ## push ipv6-hp-bpf multiplat container manifest
	$(MAKE) manifest-push \
		IMAGE=$(IPV6_HP_BPF_IMAGE) \
		TAG=$(IPV6_HP_BPF_VERSION)

ipv6-hp-bpf-skopeo-archive: ## export tar archive of ipv6-hp-bpf multiplat container manifest.
	$(MAKE) manifest-skopeo-archive \
		IMAGE=$(IPV6_HP_BPF_IMAGE) \
		TAG=$(IPV6_HP_BPF_VERSION)

cni-manifest-build: ## build cni multiplat container manifest.
	$(MAKE) manifest-build \
		PLATFORMS="$(PLATFORMS)" \
		IMAGE=$(CNI_IMAGE) \
		TAG=$(CNI_VERSION)

cni-manifest-push: ## push cni multiplat container manifest
	$(MAKE) manifest-push \
		IMAGE=$(CNI_IMAGE) \
		TAG=$(CNI_VERSION)

cni-skopeo-archive: ## export tar archive of cni multiplat container manifest.
	$(MAKE) manifest-skopeo-archive \
		IMAGE=$(CNI_IMAGE) \
		TAG=$(CNI_VERSION)

cns-manifest-build: ## build azure-cns multiplat container manifest.
	$(MAKE) manifest-build \
		PLATFORMS="$(PLATFORMS)" \
		IMAGE=$(CNS_IMAGE) \
		TAG=$(CNS_VERSION)

cns-manifest-push: ## push cns multiplat container manifest
	$(MAKE) manifest-push \
		IMAGE=$(CNS_IMAGE) \
		TAG=$(CNS_VERSION)

cns-skopeo-archive: ## export tar archive of cns multiplat container manifest.
	$(MAKE) manifest-skopeo-archive \
		IMAGE=$(CNS_IMAGE) \
		TAG=$(CNS_VERSION)

npm-manifest-build: ## build azure-npm multiplat container manifest.
	$(MAKE) manifest-build \
		PLATFORMS="$(PLATFORMS)" \
		IMAGE=$(NPM_IMAGE) \
		TAG=$(NPM_VERSION)

npm-manifest-push: ## push multiplat container manifest
	$(MAKE) manifest-push \
		IMAGE=$(NPM_IMAGE) \
		TAG=$(NPM_VERSION)

npm-skopeo-archive: ## export tar archive of multiplat container manifest.
	$(MAKE) manifest-skopeo-archive \
		IMAGE=$(NPM_IMAGE) \
		TAG=$(NPM_VERSION)


########################### Archives ################################

# Create a CNI archive for the target platform.
.PHONY: cni-archive
cni-archive: azure-vnet-binary azure-vnet-stateless-binary azure-vnet-ipam-binary azure-vnet-ipamv6-binary azure-vnet-telemetry-binary
	$(MKDIR) $(CNI_BUILD_DIR)
	cp cni/azure-$(GOOS).conflist $(CNI_BUILD_DIR)/10-azure.conflist
	cp telemetry/azure-vnet-telemetry.config $(CNI_BUILD_DIR)/azure-vnet-telemetry.config
	cp $(STATELESS_CNI_BUILD_DIR)/azure-vnet$(EXE_EXT) $(CNI_BUILD_DIR)/azure-vnet-stateless$(EXE_EXT)
	cd $(CNI_BUILD_DIR) && $(ARCHIVE_CMD) $(CNI_ARCHIVE_NAME) azure-vnet$(EXE_EXT) azure-vnet-stateless$(EXE_EXT) azure-vnet-ipam$(EXE_EXT) azure-vnet-ipamv6$(EXE_EXT) azure-vnet-telemetry$(EXE_EXT) 10-azure.conflist azure-vnet-telemetry.config

ifeq ($(GOOS),windows)
	$(MKDIR) $(CNI_MULTITENANCY_BUILD_DIR)
	cp cni/azure-$(GOOS)-multitenancy.conflist $(CNI_MULTITENANCY_BUILD_DIR)/10-azure.conflist
	cp $(CNI_BUILD_DIR)/azure-vnet$(EXE_EXT) $(CNI_BUILD_DIR)/azure-vnet-ipam$(EXE_EXT) $(CNI_MULTITENANCY_BUILD_DIR)
	cd $(CNI_MULTITENANCY_BUILD_DIR) && $(ARCHIVE_CMD) $(CNI_MULTITENANCY_ARCHIVE_NAME) azure-vnet$(EXE_EXT) azure-vnet-ipam$(EXE_EXT) 10-azure.conflist
endif

ifeq ($(GOOS),linux)
	$(MKDIR) $(CNI_MULTITENANCY_TRANSPARENT_VLAN_BUILD_DIR)
	cp cni/azure-$(GOOS)-multitenancy-transparent-vlan.conflist $(CNI_MULTITENANCY_TRANSPARENT_VLAN_BUILD_DIR)/10-azure.conflist
	cp $(CNI_BUILD_DIR)/azure-vnet$(EXE_EXT) $(CNI_MULTITENANCY_TRANSPARENT_VLAN_BUILD_DIR)
	cp telemetry/azure-vnet-telemetry.config $(CNI_MULTITENANCY_TRANSPARENT_VLAN_BUILD_DIR)/azure-vnet-telemetry.config
	cp $(CNI_BUILD_DIR)/azure-vnet-telemetry$(EXE_EXT) $(CNI_MULTITENANCY_TRANSPARENT_VLAN_BUILD_DIR)
	cd $(CNI_MULTITENANCY_TRANSPARENT_VLAN_BUILD_DIR) && $(ARCHIVE_CMD) $(CNI_MULTITENANCY_TRANSPARENT_VLAN_ARCHIVE_NAME) azure-vnet$(EXE_EXT) azure-vnet-telemetry$(EXE_EXT) 10-azure.conflist azure-vnet-telemetry.config
endif

	$(MKDIR) $(CNI_SWIFT_BUILD_DIR)
	cp cni/azure-$(GOOS)-swift.conflist $(CNI_SWIFT_BUILD_DIR)/10-azure.conflist
	cp telemetry/azure-vnet-telemetry.config $(CNI_SWIFT_BUILD_DIR)/azure-vnet-telemetry.config
	cp $(CNI_BUILD_DIR)/azure-vnet$(EXE_EXT) $(CNI_BUILD_DIR)/azure-vnet-ipam$(EXE_EXT) $(CNI_BUILD_DIR)/azure-vnet-telemetry$(EXE_EXT) $(CNI_SWIFT_BUILD_DIR)
	cp $(STATELESS_CNI_BUILD_DIR)/azure-vnet$(EXE_EXT) $(CNI_SWIFT_BUILD_DIR)/azure-vnet-stateless$(EXE_EXT)
	cd $(CNI_SWIFT_BUILD_DIR) && $(ARCHIVE_CMD) $(CNI_SWIFT_ARCHIVE_NAME) azure-vnet$(EXE_EXT) azure-vnet-stateless$(EXE_EXT) azure-vnet-ipam$(EXE_EXT) azure-vnet-telemetry$(EXE_EXT) 10-azure.conflist azure-vnet-telemetry.config

	$(MKDIR) $(CNI_OVERLAY_BUILD_DIR)
	cp cni/azure-$(GOOS)-swift-overlay.conflist $(CNI_OVERLAY_BUILD_DIR)/10-azure.conflist
	cp telemetry/azure-vnet-telemetry.config $(CNI_OVERLAY_BUILD_DIR)/azure-vnet-telemetry.config
	cp $(CNI_BUILD_DIR)/azure-vnet$(EXE_EXT) $(CNI_BUILD_DIR)/azure-vnet-ipam$(EXE_EXT) $(CNI_BUILD_DIR)/azure-vnet-telemetry$(EXE_EXT) $(CNI_OVERLAY_BUILD_DIR)
	cp $(STATELESS_CNI_BUILD_DIR)/azure-vnet$(EXE_EXT) $(CNI_OVERLAY_BUILD_DIR)/azure-vnet-stateless$(EXE_EXT)
	cd $(CNI_OVERLAY_BUILD_DIR) && $(ARCHIVE_CMD) $(CNI_OVERLAY_ARCHIVE_NAME) azure-vnet$(EXE_EXT) azure-vnet-stateless$(EXE_EXT) azure-vnet-ipam$(EXE_EXT) azure-vnet-telemetry$(EXE_EXT) 10-azure.conflist azure-vnet-telemetry.config

	$(MKDIR) $(CNI_DUALSTACK_BUILD_DIR)
	cp cni/azure-$(GOOS)-swift-overlay-dualstack.conflist $(CNI_DUALSTACK_BUILD_DIR)/10-azure.conflist
	cp telemetry/azure-vnet-telemetry.config $(CNI_DUALSTACK_BUILD_DIR)/azure-vnet-telemetry.config
	cp $(CNI_BUILD_DIR)/azure-vnet$(EXE_EXT) $(CNI_BUILD_DIR)/azure-vnet-telemetry$(EXE_EXT) $(CNI_DUALSTACK_BUILD_DIR)
	cp $(STATELESS_CNI_BUILD_DIR)/azure-vnet$(EXE_EXT) $(CNI_DUALSTACK_BUILD_DIR)/azure-vnet-stateless$(EXE_EXT)
	cd $(CNI_DUALSTACK_BUILD_DIR) && $(ARCHIVE_CMD) $(CNI_DUALSTACK_ARCHIVE_NAME) azure-vnet$(EXE_EXT) azure-vnet-stateless$(EXE_EXT) azure-vnet-telemetry$(EXE_EXT) 10-azure.conflist azure-vnet-telemetry.config

#baremetal mode is windows only (at least for now)
ifeq ($(GOOS),windows)
	$(MKDIR) $(CNI_BAREMETAL_BUILD_DIR)
	cp cni/azure-$(GOOS)-baremetal.conflist $(CNI_BAREMETAL_BUILD_DIR)/10-azure.conflist
	cp $(CNI_BUILD_DIR)/azure-vnet$(EXE_EXT) $(CNI_BAREMETAL_BUILD_DIR)
	cd $(CNI_BAREMETAL_BUILD_DIR) && $(ARCHIVE_CMD) $(CNI_BAREMETAL_ARCHIVE_NAME) azure-vnet$(EXE_EXT) 10-azure.conflist
endif

# Create a cli archive for the target platform.
.PHONY: acncli-archive
acncli-archive: acncli-binary
ifeq ($(GOOS),linux)
	$(MKDIR) $(ACNCLI_BUILD_DIR)
	cd $(ACNCLI_BUILD_DIR) && $(ARCHIVE_CMD) $(ACNCLI_ARCHIVE_NAME) acn$(EXE_EXT)
endif

# Create a CNS archive for the target platform.
.PHONY: cns-archive
cns-archive: azure-cns-binary
	cp cns/configuration/cns_config.json $(CNS_BUILD_DIR)/cns_config.json
	cd $(CNS_BUILD_DIR) && $(ARCHIVE_CMD) $(CNS_ARCHIVE_NAME) azure-cns$(EXE_EXT) cns_config.json

# Create a NPM archive for the target platform. Only Linux is supported for now.
.PHONY: npm-archive
npm-archive: azure-npm-binary
	cd $(NPM_BUILD_DIR) && $(ARCHIVE_CMD) $(NPM_ARCHIVE_NAME) azure-npm$(EXE_EXT)

# Create a azure-ipam archive for the target platform.
.PHONY: azure-ipam-archive
azure-ipam-archive: azure-ipam-binary
ifeq ($(GOOS),linux)
	$(MKDIR) $(AZURE_IPAM_BUILD_DIR)
	cd $(AZURE_IPAM_BUILD_DIR) && $(ARCHIVE_CMD) $(AZURE_IPAM_ARCHIVE_NAME) azure-ipam$(EXE_EXT)
endif

# Create a azure-ip-masq-merger archive for the target platform.
.PHONY: azure-ip-masq-merger-archive
azure-ip-masq-merger-archive: azure-ip-masq-merger-binary
ifeq ($(GOOS),linux)
	$(MKDIR) $(AZURE_IP_MASQ_MERGER_BUILD_DIR)
	cd $(AZURE_IP_MASQ_MERGER_BUILD_DIR) && $(ARCHIVE_CMD) $(AZURE_IP_MASQ_MERGER_ARCHIVE_NAME) azure-ip-masq-merger$(EXE_EXT)
endif

# Create a azure-iptables-monitor archive for the target platform.
.PHONY: azure-iptables-monitor-archive
azure-iptables-monitor-archive: azure-iptables-monitor-binary
ifeq ($(GOOS),linux)
	$(MKDIR) $(AZURE_IPTABLES_MONITOR_BUILD_DIR)
	cd $(AZURE_IPTABLES_MONITOR_BUILD_DIR) && $(ARCHIVE_CMD) $(AZURE_IPTABLES_MONITOR_ARCHIVE_NAME) azure-iptables-monitor$(EXE_EXT)
endif

# Create a ipv6-hp-bpf archive for the target platform.
.PHONY: ipv6-hp-bpf-archive
ipv6-hp-bpf-archive: ipv6-hp-bpf-binary
ifeq ($(GOOS),linux)
	$(MKDIR) $(IPV6_HP_BPF_BUILD_DIR)
	cd $(IPV6_HP_BPF_BUILD_DIR) && $(ARCHIVE_CMD) $(IPV6_HP_BPF_ARCHIVE_NAME) ipv6-hp-bpf$(EXE_EXT)
endif

# Create a azure-block-iptables archive for the target platform.
.PHONY: azure-block-iptables-archive
azure-block-iptables-archive: azure-block-iptables-binary
ifeq ($(GOOS),linux)
	$(MKDIR) $(AZURE_BLOCK_IPTABLES_BUILD_DIR)
	cd $(AZURE_BLOCK_IPTABLES_BUILD_DIR) && $(ARCHIVE_CMD) $(AZURE_BLOCK_IPTABLES_ARCHIVE_NAME) azure-block-iptables$(EXE_EXT)
endif

##@ Utils

clean: ## Clean build artifacts.
	$(RMDIR) $(OUTPUT_DIR)
	$(RMDIR) go.work*


LINT_PKG ?= .

GOLANGCI_LINT = go tool -modfile=$(TOOLS_GO_MOD) golangci-lint

lint: ## Fast lint vs default branch showing only new issues.
	GOGC=20 $(GOLANGCI_LINT) run --timeout 25m -v $(LINT_PKG)/...

lint-all: ## Lint the current branch in entirety.
	GOGC=20 $(GOLANGCI_LINT) run -v $(LINT_PKG)/...


FMT_PKG ?= cni cns npm

fmt: ## run gofumpt on $FMT_PKG (default "cni cns npm").
	go tool -modfile=$(TOOLS_GO_MOD) gofumpt -s -w $(FMT_PKG)


workspace: ## Set up the Go workspace.
	go work init
	go work use .
	go work use ./azure-ipam
	go work use ./azure-ip-masq-merger
	go work use ./azure-iptables-monitor
	go work use ./cilium-log-collector
	go work use ./dropgz
	go work use ./zapai

##@ Test

COVER_PKG ?= .
#Restart case is used for cni load test pipeline for restarting the nodes cluster.
RESTART_CASE ?= false
# CNI type is a key to direct the types of state validation done on a cluster.
CNI_TYPE ?= cilium

test-all: test-azure-ipam test-azure-ip-masq-merger test-azure-iptables-monitor test-cilium-log-collector test-main ## run all unit tests.

test-main:
	go test -mod=readonly -buildvcs=false -tags "unit" --skip 'TestE2E*' -race -covermode atomic -coverprofile=coverage-main.out $(COVER_PKG)/...
	go tool cover -func=coverage-main.out

test-integration: ## run all integration tests.
	AZURE_IPAM_VERSION=$(AZURE_IPAM_VERSION) \
		CNI_VERSION=$(CNI_VERSION) \
		CNS_VERSION=$(CNS_VERSION) \
		go test -mod=readonly -buildvcs=false -timeout 1h -coverpkg=./... -race -covermode atomic -coverprofile=coverage.out -tags=integration --skip 'TestE2E*' ./test/integration...

test-load: ## run all load tests
	AZURE_IPAM_VERSION=$(AZURE_IPAM_VERSION) \
		CNI_VERSION=$(CNI_VERSION)
		CNS_VERSION=$(CNS_VERSION) \
		go test -timeout 40m -race -tags=load ./test/integration/load... -v

test-validate-state:
	cd test/integration/load && go test -mod=readonly -count=1 -timeout 30m -tags load --skip 'TestE2E*' -run ^TestValidateState
	cd ../../..

test-cyclonus: ## run the cyclonus test for npm.
	cd test/cyclonus && bash ./test-cyclonus.sh
	cd ..

test-cyclonus-windows: ## run the cyclonus test for npm.
	cd test/cyclonus && bash ./test-cyclonus.sh windows
	cd ..

test-extended-cyclonus: ## run the cyclonus test for npm.
	cd test/cyclonus && bash ./test-cyclonus.sh extended
	cd ..

test-azure-ipam: ## run the unit test for azure-ipam
	cd $(AZURE_IPAM_DIR) && go test -race -covermode atomic -coverprofile=../coverage-azure-ipam.out && go tool cover -func=../coverage-azure-ipam.out

test-azure-ip-masq-merger: ## run the unit test for azure-ip-masq-merger
	cd $(AZURE_IP_MASQ_MERGER_DIR) && go test -race -covermode atomic -coverprofile=../coverage-azure-ip-masq-merger.out && go tool cover -func=../coverage-azure-ip-masq-merger.out

test-azure-iptables-monitor: ## run the unit test for azure-iptables-monitor
	cd $(AZURE_IPTABLES_MONITOR_DIR) && go test -race -covermode atomic -coverprofile=../coverage-azure-iptables-monitor.out && go tool cover -func=../coverage-azure-iptables-monitor.out

kind:
	kind create cluster --config ./test/kind/kind.yaml

test-k8se2e: test-k8se2e-build test-k8se2e-only ## Alias to run build and test

test-k8se2e-build: ## Build k8s e2e test suite
	cd hack/scripts && bash ./k8se2e.sh $(GROUP) $(CLUSTER)
	cd ../..

test-k8se2e-only: ## Run k8s network conformance test, use TYPE=basic for only datapath tests
	cd hack/scripts && bash ./k8se2e-tests.sh $(OS) $(TYPE)
	cd ../..

##@ Utilities

dockerfiles: renderkit ## Render all Dockerfile templates with current state of world
	@make -f build/images.mk render PATH=cns
	@make -f build/images.mk render PATH=cni

regenerate-crd: ## Regenerate CRDs
	for makefile in $$(find ./crd/ -name "Makefile" -type f -printf '%h\n'); do \
		echo "Running make in $$makefile"; \
		make -C $$makefile; \
	done


$(REPO_ROOT)/.git/hooks/pre-push:
	@ln -s $(REPO_ROOT)/.hooks/pre-push $(REPO_ROOT)/.git/hooks/
	@echo installed pre-push hook

install-hooks: $(REPO_ROOT)/.git/hooks/pre-push ## installs git hooks

gitconfig: ## configure the local git repository
	@git config commit.gpgsign true
	@git config pull.rebase true
	@git config fetch.prune true
	@git config core.fsmonitor true
	@git config core.untrackedcache true

setup: tools install-hooks gitconfig ## performs common required repo setup


##@ Tools

tools: renderkit go-junit-report gocov gocov-xml

renderkit: ## Install renderkit for rendering Dockerfile templates
	@GOWORK=off go install -modfile=$(TOOLS_GO_MOD) github.com/orellazri/renderkit

go-junit-report: ## Install go-junit-report for converting test results to JUnit XML format
	@GOWORK=off go install -modfile=$(TOOLS_GO_MOD) github.com/jstemmer/go-junit-report

gocov: ## Install gocov .out > .json
	@GOWORK=off go install -modfile=$(TOOLS_GO_MOD) github.com/axw/gocov/gocov

gocov-xml: ## Install gocov-xml .json > .xml
	@GOWORK=off go install -modfile=$(TOOLS_GO_MOD) github.com/AlekSi/gocov-xml

##@ Help

help: ## Display this help
	@awk 'BEGIN {FS = ":.*##"; printf "\nUsage:\n  make \033[36m<target>\033[0m\n"} /^[0-9a-zA-Z_-]+:.*?##/ { printf "  \033[36m%-15s\033[0m %s\n", $$1, $$2 } /^##@/ { printf "\n\033[1m%s\033[0m\n", substr($$0, 5) } ' $(MAKEFILE_LIST)

proto-gen: ## Generates source code from protobuf files
	protoc --go_out=. --go-grpc_out=. cns/grpc/proto/server.proto
