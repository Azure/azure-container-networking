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
GOOS 	 ?= $(shell go env GOOS)
GOARCH   ?= $(shell go env GOARCH)
GOOSES   ?= "linux windows" # To override at the cli do: GOOSES="\"darwin bsd\""
GOARCHES ?= "amd64 arm64" # To override at the cli do: GOARCHES="\"ppc64 mips\""
ltsc2019  = "10.0.17763.4010"
ltsc2022  = "10.0.20348.643"

# Windows specific extensions
# set these based on the GOOS, not the OS
ifeq ($(GOOS),windows)
ARCHIVE_CMD = zip -9lq
ARCHIVE_EXT = zip
EXE_EXT 	= .exe
endif

# Interrogate the git repo and set some variables
REPO_ROOT 		   		 = $(shell git rev-parse --show-toplevel)
REVISION 		   		?= $(shell git rev-parse --short HEAD)
ACN_VERSION  	   		?= $(shell git describe --exclude "azure-ipam*" --exclude "dropgz*" --exclude "zapai*" --tags --always)
AZURE_IPAM_VERSION 		?= $(notdir $(shell git describe --match "azure-ipam*" --tags --always))
CNI_VERSION        		?= $(ACN_VERSION)
CNI_DROPGZ_VERSION 		?= $(notdir $(shell git describe --match "dropgz*" --tags --always))
CNI_DROPGZ_TEST_VERSION ?= $(notdir $(shell git describe --match "dropgz-test*" --tags --always))
CNS_VERSION  	   		?= $(ACN_VERSION)
NPM_VERSION        		?= $(ACN_VERSION)
ZAPAI_VERSION  	   		?= $(notdir $(shell git describe --match "zapai*" --tags --always))

# Build directories.
AZURE_IPAM_DIR = $(REPO_ROOT)/azure-ipam
CNM_DIR = $(REPO_ROOT)/cnm/plugin
CNI_NET_DIR = $(REPO_ROOT)/cni/network/plugin
CNI_IPAM_DIR = $(REPO_ROOT)/cni/ipam/plugin
CNI_IPAMV6_DIR = $(REPO_ROOT)/cni/ipam/pluginv6
CNI_TELEMETRY_DIR = $(REPO_ROOT)/cni/telemetry/service
ACNCLI_DIR = $(REPO_ROOT)/tools/acncli
CNS_DIR = $(REPO_ROOT)/cns/service
NPM_DIR = $(REPO_ROOT)/npm/cmd
OUTPUT_DIR = $(REPO_ROOT)/output
BUILD_DIR = $(OUTPUT_DIR)/$(GOOS)_$(GOARCH)
AZURE_IPAM_BUILD_DIR = $(BUILD_DIR)/azure-ipam
IMAGE_DIR  = $(OUTPUT_DIR)/images
CNM_BUILD_DIR = $(BUILD_DIR)/cnm
CNI_BUILD_DIR = $(BUILD_DIR)/cni
ACNCLI_BUILD_DIR = $(BUILD_DIR)/acncli
CNI_MULTITENANCY_BUILD_DIR = $(BUILD_DIR)/cni-multitenancy
CNI_MULTITENANCY_TRANSPARENT_VLAN_BUILD_DIR = $(BUILD_DIR)/cni-multitenancy-transparent-vlan
CNI_SWIFT_BUILD_DIR = $(BUILD_DIR)/cni-swift
CNI_OVERLAY_BUILD_DIR = $(BUILD_DIR)/cni-overlay
CNI_BAREMETAL_BUILD_DIR = $(BUILD_DIR)/cni-baremetal
CNI_DUALSTACK_BUILD_DIR = $(BUILD_DIR)/cni-dualstack
CNS_BUILD_DIR = $(BUILD_DIR)/cns
NPM_BUILD_DIR = $(BUILD_DIR)/npm
TOOLS_DIR = $(REPO_ROOT)/build/tools
TOOLS_BIN_DIR = $(TOOLS_DIR)/bin
CNI_AI_ID = 5515a1eb-b2bc-406a-98eb-ba462e6f0411
CNS_AI_ID = ce672799-8f08-4235-8c12-08563dc2acef
NPM_AI_ID = 014c22bd-4107-459e-8475-67909e96edcb
ACN_PACKAGE_PATH = github.com/Azure/azure-container-networking
CNI_AI_PATH=$(ACN_PACKAGE_PATH)/telemetry.aiMetadata
CNS_AI_PATH=$(ACN_PACKAGE_PATH)/cns/logger.aiMetadata
NPM_AI_PATH=$(ACN_PACKAGE_PATH)/npm.aiMetadata

# Tool paths
CONTROLLER_GEN  := $(TOOLS_BIN_DIR)/controller-gen
GOCOV           := $(TOOLS_BIN_DIR)/gocov
GOCOV_XML       := $(TOOLS_BIN_DIR)/gocov-xml
GOFUMPT         := $(TOOLS_BIN_DIR)/gofumpt
GOLANGCI_LINT   := $(TOOLS_BIN_DIR)/golangci-lint
GO_JUNIT_REPORT := $(TOOLS_BIN_DIR)/go-junit-report
MOCKGEN         := $(TOOLS_BIN_DIR)/mockgen

# Archive file names.
ACNCLI_ARCHIVE_NAME = acncli-$(GOOS)-$(GOARCH)-$(ACN_VERSION).$(ARCHIVE_EXT)
CNI_ARCHIVE_NAME = azure-vnet-cni-$(GOOS)-$(GOARCH)-$(CNI_VERSION).$(ARCHIVE_EXT)
CNI_MULTITENANCY_ARCHIVE_NAME = azure-vnet-cni-multitenancy-$(GOOS)-$(GOARCH)-$(CNI_VERSION).$(ARCHIVE_EXT)
CNI_MULTITENANCY_TRANSPARENT_VLAN_ARCHIVE_NAME = azure-vnet-cni-multitenancy-transparent-vlan-$(GOOS)-$(GOARCH)-$(CNI_VERSION).$(ARCHIVE_EXT)
CNI_SWIFT_ARCHIVE_NAME = azure-vnet-cni-swift-$(GOOS)-$(GOARCH)-$(CNI_VERSION).$(ARCHIVE_EXT)
CNI_OVERLAY_ARCHIVE_NAME = azure-vnet-cni-overlay-$(GOOS)-$(GOARCH)-$(CNI_VERSION).$(ARCHIVE_EXT)
CNI_BAREMETAL_ARCHIVE_NAME = azure-vnet-cni-baremetal-$(GOOS)-$(GOARCH)-$(CNI_VERSION).$(ARCHIVE_EXT)
CNI_DUALSTACK_ARCHIVE_NAME = azure-vnet-cni-overlay-dualstack-$(GOOS)-$(GOARCH)-$(CNI_VERSION).$(ARCHIVE_EXT)
CNM_ARCHIVE_NAME = azure-vnet-cnm-$(GOOS)-$(GOARCH)-$(ACN_VERSION).$(ARCHIVE_EXT)
CNS_ARCHIVE_NAME = azure-cns-$(GOOS)-$(GOARCH)-$(CNS_VERSION).$(ARCHIVE_EXT)
NPM_ARCHIVE_NAME = azure-npm-$(GOOS)-$(GOARCH)-$(NPM_VERSION).$(ARCHIVE_EXT)
AZURE_IPAM_ARCHIVE_NAME = azure-ipam-$(GOOS)-$(GOARCH)-$(AZURE_IPAM_VERSION).$(ARCHIVE_EXT)

# Image info file names.
CNI_DROPGZ_IMAGE_INFO_FILE = cni-dropgz-$(CNI_DROPGZ_VERSION).txt
CNS_IMAGE_INFO_FILE 	   = azure-cns-$(CNS_VERSION).txt
NPM_IMAGE_INFO_FILE 	   = azure-npm-$(NPM_VERSION).txt

# Docker libnetwork (CNM) plugin v2 image parameters.
CNM_PLUGIN_IMAGE ?= microsoft/azure-vnet-plugin
CNM_PLUGIN_ROOTFS = azure-vnet-plugin-rootfs

# Default target
all-binaries-platforms: ## Make all platform binaries
	@for goos in "$(GOOSES)"; do \
		for goarch in "$(GOARCHES)"; do \
			make all-binaries GOOS=$$goos GOARCH=$$goarch; \
		done \
	done

# OS specific binaries/images
ifeq ($(GOOS),linux)
all-binaries: acncli azure-cnm-plugin azure-cni-plugin azure-cns azure-npm azure-ipam
all-images: npm-image cns-image cni-manager-image
else
all-binaries: azure-cnm-plugin azure-cni-plugin azure-cns azure-npm
all-images:
	@echo "Nothing to build. Skip."
endif

# Shorthand target names for convenience.
azure-cnm-plugin: cnm-binary cnm-archive
azure-cni-plugin: azure-vnet-binary azure-vnet-ipam-binary azure-vnet-ipamv6-binary azure-vnet-telemetry-binary cni-archive
azure-cns: azure-cns-binary cns-archive
acncli: acncli-binary acncli-archive
azure-cnms: azure-cnms-binary cnms-archive
azure-npm: azure-npm-binary npm-archive
azure-ipam: azure-ipam-binary azure-ipam-archive


##@ Versioning

revision: ## print the current git revision
	@echo $(REVISION)

version: ## prints the root version
	@echo $(ACN_VERSION)

acncli-version: version

azure-ipam-version: ## prints the azure-ipam version
	@echo $(AZURE_IPAM_VERSION)

cni-version: ## prints the cni version
	@echo $(CNI_VERSION)

cni-dropgz-version: ## prints the cni-dropgz version
	@echo $(CNI_DROPGZ_VERSION)

cni-dropgz-test-version: ## prints the cni-dropgz version
	@echo $(CNI_DROPGZ_TEST_VERSION)

cns-version:
	@echo $(CNS_VERSION)

npm-version:
	@echo $(NPM_VERSION)

zapai-version: ## prints the zapai version
	@echo $(ZAPAI_VERSION)

##@ Binaries

# Build the delegated IPAM plugin binary.
azure-ipam-binary:
	cd $(AZURE_IPAM_DIR) && CGO_ENABLED=0 go build -v -o $(AZURE_IPAM_BUILD_DIR)/azure-ipam$(EXE_EXT) -ldflags "-X github.com/Azure/azure-container-networking/azure-ipam/internal/buildinfo.Version=$(AZURE_IPAM_VERSION)" -gcflags="-dwarflocationlists=true"

# Build the Azure CNM binary.
cnm-binary:
	cd $(CNM_DIR) && CGO_ENABLED=0 go build -v -o $(CNM_BUILD_DIR)/azure-vnet-plugin$(EXE_EXT) -ldflags "-X main.version=$(ACN_VERSION)" -gcflags="-dwarflocationlists=true"

# Build the Azure CNI network binary.
azure-vnet-binary:
	cd $(CNI_NET_DIR) && CGO_ENABLED=0 go build -v -o $(CNI_BUILD_DIR)/azure-vnet$(EXE_EXT) -ldflags "-X main.version=$(CNI_VERSION)" -gcflags="-dwarflocationlists=true"

# Build the Azure CNI IPAM binary.
azure-vnet-ipam-binary:
	cd $(CNI_IPAM_DIR) && CGO_ENABLED=0 go build -v -o $(CNI_BUILD_DIR)/azure-vnet-ipam$(EXE_EXT) -ldflags "-X main.version=$(CNI_VERSION)" -gcflags="-dwarflocationlists=true"

# Build the Azure CNI IPAMV6 binary.
azure-vnet-ipamv6-binary:
	cd $(CNI_IPAMV6_DIR) && CGO_ENABLED=0 go build -v -o $(CNI_BUILD_DIR)/azure-vnet-ipamv6$(EXE_EXT) -ldflags "-X main.version=$(CNI_VERSION)" -gcflags="-dwarflocationlists=true"

# Build the Azure CNI telemetry binary.
azure-vnet-telemetry-binary:
	cd $(CNI_TELEMETRY_DIR) && CGO_ENABLED=0 go build -v -o $(CNI_BUILD_DIR)/azure-vnet-telemetry$(EXE_EXT) -ldflags "-X main.version=$(CNI_VERSION) -X $(CNI_AI_PATH)=$(CNI_AI_ID)" -gcflags="-dwarflocationlists=true"

# Build the Azure CLI network binary.
acncli-binary:
	cd $(ACNCLI_DIR) && CGO_ENABLED=0 go build -v -o $(ACNCLI_BUILD_DIR)/acn$(EXE_EXT) -ldflags "-X main.version=$(ACN_VERSION)" -gcflags="-dwarflocationlists=true"

# Build the Azure CNS binary.
azure-cns-binary:
	cd $(CNS_DIR) && CGO_ENABLED=0 go build -v -o $(CNS_BUILD_DIR)/azure-cns$(EXE_EXT) -ldflags "-X main.version=$(CNS_VERSION) -X $(CNS_AI_PATH)=$(CNS_AI_ID) -X $(CNI_AI_PATH)=$(CNI_AI_ID)" -gcflags="-dwarflocationlists=true"

# Build the Azure NPM binary.
azure-npm-binary:
	cd $(CNI_TELEMETRY_DIR) && CGO_ENABLED=0 go build -v -o $(NPM_BUILD_DIR)/azure-vnet-telemetry$(EXE_EXT) -ldflags "-X main.version=$(NPM_VERSION)" -gcflags="-dwarflocationlists=true"
	cd $(NPM_DIR) && CGO_ENABLED=0 go build -v -o $(NPM_BUILD_DIR)/azure-npm$(EXE_EXT) -ldflags "-X main.version=$(NPM_VERSION) -X $(NPM_AI_PATH)=$(NPM_AI_ID)" -gcflags="-dwarflocationlists=true"

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
ACNCLI_IMAGE     	  = acncli
CNI_DROPGZ_IMAGE 	  = cni-dropgz
CNI_DROPGZ_TEST_IMAGE = cni-dropgz-test
CNS_IMAGE        	  = azure-cns
NPM_IMAGE        	  = azure-npm

## Image platform tags.
ACNCLI_PLATFORM_TAG    		 ?= $(subst /,-,$(PLATFORM))$(if $(OS_VERSION),-$(OS_VERSION),)-$(ACN_VERSION)
CNI_DROPGZ_PLATFORM_TAG 	 ?= $(subst /,-,$(PLATFORM))$(if $(OS_VERSION),-$(OS_VERSION),)-$(CNI_DROPGZ_VERSION)
CNI_DROPGZ_TEST_PLATFORM_TAG ?= $(subst /,-,$(PLATFORM))$(if $(OS_VERSION),-$(OS_VERSION),)-$(CNI_DROPGZ_TEST_VERSION)
CNS_PLATFORM_TAG        	 ?= $(subst /,-,$(PLATFORM))$(if $(OS_VERSION),-$(OS_VERSION),)-$(CNS_VERSION)
CNS_WINDOWS_PLATFORM_TAG 	 ?= $(subst /,-,$(PLATFORM))$(if $(OS_VERSION),-$(OS_VERSION),)-$(CNS_VERSION)-$(WINDOWS_OS_SKU)
NPM_PLATFORM_TAG        	 ?= $(subst /,-,$(PLATFORM))$(if $(OS_VERSION),-$(OS_VERSION),)-$(NPM_VERSION)


qemu-user-static: ## Set up the host to run qemu multiplatform container builds.
	sudo $(CONTAINER_RUNTIME) run --rm --privileged multiarch/qemu-user-static --reset -p yes


## Reusable build targets for building individual container images.

container-buildah: # util target to build container images using buildah. do not invoke directly.
	buildah bud \
		--jobs 16 \
		--platform $(PLATFORM) \
		-f $(DOCKERFILE) \
		--build-arg VERSION=$(TAG) $(EXTRA_BUILD_ARGS) \
		-t $(IMAGE_REGISTRY)/$(IMAGE):$(TAG) \
		.
	buildah push $(IMAGE_REGISTRY)/$(IMAGE):$(TAG)

container-docker: # util target to build container images using docker buildx. do not invoke directly.
	docker buildx create --use --platform $(PLATFORM)
	docker buildx build \
		$(BUILDX_ACTION) \
		--platform $(PLATFORM) \
		-f $(DOCKERFILE) \
		--build-arg VERSION=$(TAG) $(EXTRA_BUILD_ARGS) \
		-t $(IMAGE_REGISTRY)/$(IMAGE):$(TAG) \
		.

container: # util target to build container images. do not invoke directly.
	$(MAKE) container-$(CONTAINER_BUILDER) \
		PLATFORM=$(PLATFORM) \
		TAG=$(TAG) \
		OS=$(OS) \
		ARCH=$(ARCH) \
		OS_VERSION=$(OS_VERSION)

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

# cni-dropgz

cni-dropgz-image-name: # util target to print the CNI dropgz image name.
	@echo $(CNI_DROPGZ_IMAGE)

cni-dropgz-image-name-and-tag: # util target to print the CNI dropgz image name and tag.
	@echo $(IMAGE_REGISTRY)/$(CNI_DROPGZ_IMAGE):$(CNI_DROPGZ_PLATFORM_TAG)

cni-dropgz-image: ## build cni-dropgz container image.
	$(MAKE) container \
		DOCKERFILE=dropgz/build/$(OS).Dockerfile \
		EXTRA_BUILD_ARGS='--build-arg OS=$(OS) --build-arg ARCH=$(ARCH) --build-arg OS_VERSION=$(OS_VERSION)' \
		IMAGE=$(CNI_DROPGZ_IMAGE) \
		TAG=$(CNI_DROPGZ_PLATFORM_TAG)

cni-dropgz-image-push: ## push cni-dropgz container image.
	$(MAKE) container-push \
		IMAGE=$(CNI_DROPGZ_IMAGE) \
		TAG=$(CNI_DROPGZ_PLATFORM_TAG)

cni-dropgz-image-pull: ## pull cni-dropgz container image.
	$(MAKE) container-pull \
		IMAGE=$(CNI_DROPGZ_IMAGE) \
		TAG=$(CNI_DROPGZ_PLATFORM_TAG)

# cni-dropgz-test

cni-dropgz-test-image-name: # util target to print the CNI dropgz test image name.
	@echo $(CNI_DROPGZ_TEST_IMAGE)

cni-dropgz-test-image-name-and-tag: # util target to print the CNI dropgz test image name and tag.
	@echo $(IMAGE_REGISTRY)/$(CNI_DROPGZ_TEST_IMAGE):$(CNI_DROPGZ_TEST_PLATFORM_TAG)

cni-dropgz-test-image: ## build cni-dropgz-test container image.
	$(MAKE) container \
		DOCKERFILE=dropgz/build/cniTest_$(OS).Dockerfile \
		EXTRA_BUILD_ARGS='--build-arg OS=$(OS)  --build-arg ARCH=$(ARCH) --build-arg OS_VERSION=$(OS_VERSION)' \
		IMAGE=$(CNI_DROPGZ_TEST_IMAGE) \
		TAG=$(CNI_DROPGZ_TEST_PLATFORM_TAG)

cni-dropgz-test-image-push: ## push cni-dropgz-test container image.
	$(MAKE) container-push \
		IMAGE=$(CNI_DROPGZ_TEST_IMAGE) \
		TAG=$(CNI_DROPGZ_TEST_PLATFORM_TAG)

cni-dropgz-test-image-pull: ## pull cni-dropgz-test container image.
	$(MAKE) container-pull \
		IMAGE=$(CNI_DROPGZ_TEST_IMAGE) \
		TAG=$(CNI_DROPGZ_TEST_PLATFORM_TAG)

# cns

cns-image-name: # util target to print the CNS image name
	@echo $(CNS_IMAGE)

cns-image-name-and-tag: # util target to print the CNS image name and tag.
	@echo $(IMAGE_REGISTRY)/$(CNS_IMAGE):$(CNS_PLATFORM_TAG)

cns-image: ## build cns container image.
	$(MAKE) container \
		DOCKERFILE=cns/$(OS).Dockerfile \
		IMAGE=$(CNS_IMAGE) \
		EXTRA_BUILD_ARGS='--build-arg CNS_AI_PATH=$(CNS_AI_PATH) --build-arg CNS_AI_ID=$(CNS_AI_ID) --build-arg OS_VERSION=$(OS_VERSION)' \
		PLATFORM=$(PLATFORM) \
		TAG=$(CNS_PLATFORM_TAG) \
		OS=$(OS) \
		ARCH=$(ARCH) \
		OS_VERSION=$(OS_VERSION)

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
		EXTRA_BUILD_ARGS='--build-arg NPM_AI_PATH=$(NPM_AI_PATH) --build-arg NPM_AI_ID=$(NPM_AI_ID) --build-arg OS_VERSION=$(OS_VERSION)' \
		PLATFORM=$(PLATFORM) \
		TAG=$(NPM_PLATFORM_TAG)\
		OS=$(OS) \
		ARCH=$(ARCH) \
		OS_VERSION=$(OS_VERSION)

npm-image-push: ## push npm container image.
	$(MAKE) container-push \
		IMAGE=$(NPM_IMAGE) \
		TAG=$(NPM_PLATFORM_TAG)

npm-image-pull: ## pull cns container image.
	$(MAKE) container-pull \
		IMAGE=$(NPM_IMAGE) \
		TAG=$(NPM_PLATFORM_TAG)


## Legacy

# Build the Azure CNM plugin image, installable with "docker plugin install".
azure-cnm-plugin-image: azure-cnm-plugin ## build the azure-cnm plugin container image.
	docker images -q $(CNM_PLUGIN_ROOTFS):$(ACN_VERSION) > cid
	docker build --no-cache \
		-f Dockerfile.cnm \
		-t $(CNM_PLUGIN_ROOTFS):$(ACN_VERSION) \
		--build-arg CNM_BUILD_DIR=$(CNM_BUILD_DIR) \
		.
	$(eval CID := `cat cid`)
	docker rmi $(CID) || true

	# Create a container using the image and export its rootfs.
	docker create $(CNM_PLUGIN_ROOTFS):$(ACN_VERSION) > cid
	$(eval CID := `cat cid`)
	$(MKDIR) $(OUTPUT_DIR)/$(CID)/rootfs
	docker export $(CID) | tar -x -C $(OUTPUT_DIR)/$(CID)/rootfs
	docker rm -vf $(CID)

	# Copy the plugin configuration and set ownership.
	cp cnm/config.json $(OUTPUT_DIR)/$(CID)
	chgrp -R docker $(OUTPUT_DIR)/$(CID)

	# Create the plugin.
	docker plugin rm $(CNM_PLUGIN_IMAGE):$(ACN_VERSION) || true
	docker plugin create $(CNM_PLUGIN_IMAGE):$(ACN_VERSION) $(OUTPUT_DIR)/$(CID)

	# Cleanup temporary files.
	rm -rf $(OUTPUT_DIR)/$(CID)
	rm cid


## Reusable targets for building multiplat container image manifests.

IMAGE_ARCHIVE_DIR ?= $(shell pwd)

manifest-create:
	$(CONTAINER_BUILDER) manifest create $(IMAGE_REGISTRY)/$(IMAGE):$(TAG)

manifest-add:
	$(CONTAINER_BUILDER) manifest add --os=$(OS) --os-version=$($(OS_VERSION)) $(IMAGE_REGISTRY)/$(IMAGE):$(TAG) docker://$(IMAGE_REGISTRY)/$(IMAGE):$(subst /,-,$(PLATFORM))$(if $(OS_VERSION),-$(OS_VERSION),)-$(TAG)

manifest-build: # util target to compose multiarch container manifests from platform specific images.
	$(MAKE) manifest-create
	$(foreach PLATFORM,$(PLATFORMS),\
		$(if $(filter $(PLATFORM),windows/amd64),\
			$(foreach OS_VERSION,$(OS_VERSIONS),\
				$(MAKE) manifest-add CONTAINER_BUILDER=$(CONTAINER_BUILDER) OS=windows OS_VERSION=$(OS_VERSION) PLATFORM=$(PLATFORM);\
			),\
			$(MAKE) manifest-add PLATFORM=$(PLATFORM);\
		)\
	)\



manifest-push: # util target to push multiarch container manifest.
	$(CONTAINER_BUILDER) manifest push --all $(IMAGE_REGISTRY)/$(IMAGE):$(TAG) docker://$(IMAGE_REGISTRY)/$(IMAGE):$(TAG)

manifest-skopeo-archive: # util target to export tar archive of multiarch container manifest.
	skopeo copy --all docker://$(IMAGE_REGISTRY)/$(IMAGE):$(TAG) oci-archive:$(IMAGE_ARCHIVE_DIR)/$(IMAGE)-$(TAG).tar

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

cni-dropgz-manifest-build: ## build cni-dropgz multiplat container manifest.
	$(MAKE) manifest-build \
		PLATFORMS="$(PLATFORMS)" \
		IMAGE=$(CNI_DROPGZ_IMAGE) \
		TAG=$(CNI_DROPGZ_VERSION) \
		OS_VERSIONS="$(OS_VERSIONS)"

cni-dropgz-manifest-push: ## push cni-dropgz multiplat container manifest
	$(MAKE) manifest-push \
		IMAGE=$(CNI_DROPGZ_IMAGE) \
		TAG=$(CNI_DROPGZ_VERSION)

cni-dropgz-skopeo-archive: ## export tar archive of cni-dropgz multiplat container manifest.
	$(MAKE) manifest-skopeo-archive \
		IMAGE=$(CNI_DROPGZ_IMAGE) \
		TAG=$(CNI_DROPGZ_VERSION)

cni-dropgz-test-manifest-build: ## build cni-dropgz multiplat container manifest.
	$(MAKE) manifest-build \
		PLATFORMS="$(PLATFORMS)" \
		IMAGE=$(CNI_DROPGZ_TEST_IMAGE) \
		TAG=$(CNI_DROPGZ_TEST_VERSION) \
		OS_VERSIONS="$(OS_VERSIONS)"

cni-dropgz-test-manifest-push: ## push cni-dropgz multiplat container manifest
	$(MAKE) manifest-push \
		IMAGE=$(CNI_DROPGZ_TEST_IMAGE) \
		TAG=$(CNI_DROPGZ_TEST_VERSION)

cni-dropgz-test-skopeo-archive: ## export tar archive of cni-dropgz multiplat container manifest.
	$(MAKE) manifest-skopeo-archive \
		IMAGE=$(CNI_DROPGZ_TEST_IMAGE) \
		TAG=$(CNI_DROPGZ_TEST_VERSION)

cns-manifest-build: ## build azure-cns multiplat container manifest.
	$(MAKE) manifest-build \
		PLATFORMS="$(PLATFORMS)" \
		IMAGE=$(CNS_IMAGE) \
		TAG=$(CNS_VERSION) \
		OS_VERSIONS="$(OS_VERSIONS)"

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
		TAG=$(NPM_VERSION) \
		OS_VERSIONS="$(OS_VERSIONS)"

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
cni-archive: azure-vnet-binary azure-vnet-ipam-binary azure-vnet-ipamv6-binary azure-vnet-telemetry-binary
	$(MKDIR) $(CNI_BUILD_DIR)
	cp cni/azure-$(GOOS).conflist $(CNI_BUILD_DIR)/10-azure.conflist
	cp telemetry/azure-vnet-telemetry.config $(CNI_BUILD_DIR)/azure-vnet-telemetry.config
	cd $(CNI_BUILD_DIR) && $(ARCHIVE_CMD) $(CNI_ARCHIVE_NAME) azure-vnet$(EXE_EXT) azure-vnet-ipam$(EXE_EXT) azure-vnet-ipamv6$(EXE_EXT) azure-vnet-telemetry$(EXE_EXT) 10-azure.conflist azure-vnet-telemetry.config

	$(MKDIR) $(CNI_MULTITENANCY_BUILD_DIR)
	cp cni/azure-$(GOOS)-multitenancy.conflist $(CNI_MULTITENANCY_BUILD_DIR)/10-azure.conflist
	cp $(CNI_BUILD_DIR)/azure-vnet$(EXE_EXT) $(CNI_BUILD_DIR)/azure-vnet-ipam$(EXE_EXT) $(CNI_MULTITENANCY_BUILD_DIR)
ifeq ($(GOOS),linux)
	cp telemetry/azure-vnet-telemetry.config $(CNI_MULTITENANCY_BUILD_DIR)/azure-vnet-telemetry.config
	cp $(CNI_BUILD_DIR)/azure-vnet-telemetry$(EXE_EXT) $(CNI_MULTITENANCY_BUILD_DIR)
endif
	cd $(CNI_MULTITENANCY_BUILD_DIR) && $(ARCHIVE_CMD) $(CNI_MULTITENANCY_ARCHIVE_NAME) azure-vnet$(EXE_EXT) azure-vnet-ipam$(EXE_EXT) azure-vnet-telemetry$(EXE_EXT) 10-azure.conflist azure-vnet-telemetry.config

	$(MKDIR) $(CNI_MULTITENANCY_TRANSPARENT_VLAN_BUILD_DIR)
	cp cni/azure-$(GOOS)-multitenancy-transparent-vlan.conflist $(CNI_MULTITENANCY_TRANSPARENT_VLAN_BUILD_DIR)/10-azure.conflist
	cp $(CNI_BUILD_DIR)/azure-vnet$(EXE_EXT) $(CNI_BUILD_DIR)/azure-vnet-ipam$(EXE_EXT) $(CNI_MULTITENANCY_TRANSPARENT_VLAN_BUILD_DIR)
ifeq ($(GOOS),linux)
	cp telemetry/azure-vnet-telemetry.config $(CNI_MULTITENANCY_TRANSPARENT_VLAN_BUILD_DIR)/azure-vnet-telemetry.config
	cp $(CNI_BUILD_DIR)/azure-vnet-telemetry$(EXE_EXT) $(CNI_MULTITENANCY_TRANSPARENT_VLAN_BUILD_DIR)
endif
	cd $(CNI_MULTITENANCY_TRANSPARENT_VLAN_BUILD_DIR) && $(ARCHIVE_CMD) $(CNI_MULTITENANCY_TRANSPARENT_VLAN_ARCHIVE_NAME) azure-vnet$(EXE_EXT) azure-vnet-ipam$(EXE_EXT) azure-vnet-telemetry$(EXE_EXT) 10-azure.conflist azure-vnet-telemetry.config

	$(MKDIR) $(CNI_SWIFT_BUILD_DIR)
	cp cni/azure-$(GOOS)-swift.conflist $(CNI_SWIFT_BUILD_DIR)/10-azure.conflist
	cp telemetry/azure-vnet-telemetry.config $(CNI_SWIFT_BUILD_DIR)/azure-vnet-telemetry.config
	cp $(CNI_BUILD_DIR)/azure-vnet$(EXE_EXT) $(CNI_BUILD_DIR)/azure-vnet-ipam$(EXE_EXT) $(CNI_BUILD_DIR)/azure-vnet-telemetry$(EXE_EXT) $(CNI_SWIFT_BUILD_DIR)
	cd $(CNI_SWIFT_BUILD_DIR) && $(ARCHIVE_CMD) $(CNI_SWIFT_ARCHIVE_NAME) azure-vnet$(EXE_EXT) azure-vnet-ipam$(EXE_EXT) azure-vnet-telemetry$(EXE_EXT) 10-azure.conflist azure-vnet-telemetry.config

	$(MKDIR) $(CNI_OVERLAY_BUILD_DIR)
	cp cni/azure-$(GOOS)-swift-overlay.conflist $(CNI_OVERLAY_BUILD_DIR)/10-azure.conflist
	cp telemetry/azure-vnet-telemetry.config $(CNI_OVERLAY_BUILD_DIR)/azure-vnet-telemetry.config
	cp $(CNI_BUILD_DIR)/azure-vnet$(EXE_EXT) $(CNI_BUILD_DIR)/azure-vnet-ipam$(EXE_EXT) $(CNI_BUILD_DIR)/azure-vnet-telemetry$(EXE_EXT) $(CNI_OVERLAY_BUILD_DIR)
	cd $(CNI_OVERLAY_BUILD_DIR) && $(ARCHIVE_CMD) $(CNI_OVERLAY_ARCHIVE_NAME) azure-vnet$(EXE_EXT) azure-vnet-ipam$(EXE_EXT) azure-vnet-telemetry$(EXE_EXT) 10-azure.conflist azure-vnet-telemetry.config

	$(MKDIR) $(CNI_DUALSTACK_BUILD_DIR)
	cp cni/azure-$(GOOS)-swift-overlay-dualstack.conflist $(CNI_DUALSTACK_BUILD_DIR)/10-azure.conflist
	cp telemetry/azure-vnet-telemetry.config $(CNI_DUALSTACK_BUILD_DIR)/azure-vnet-telemetry.config
	cp $(CNI_BUILD_DIR)/azure-vnet$(EXE_EXT) $(CNI_BUILD_DIR)/azure-vnet-telemetry$(EXE_EXT) $(CNI_DUALSTACK_BUILD_DIR)
	cd $(CNI_DUALSTACK_BUILD_DIR) && $(ARCHIVE_CMD) $(CNI_DUALSTACK_ARCHIVE_NAME) azure-vnet$(EXE_EXT) azure-vnet-telemetry$(EXE_EXT) 10-azure.conflist azure-vnet-telemetry.config

#baremetal mode is windows only (at least for now)
ifeq ($(GOOS),windows)
	$(MKDIR) $(CNI_BAREMETAL_BUILD_DIR)
	cp cni/azure-$(GOOS)-baremetal.conflist $(CNI_BAREMETAL_BUILD_DIR)/10-azure.conflist
	cp $(CNI_BUILD_DIR)/azure-vnet$(EXE_EXT) $(CNI_BAREMETAL_BUILD_DIR)
	cd $(CNI_BAREMETAL_BUILD_DIR) && $(ARCHIVE_CMD) $(CNI_BAREMETAL_ARCHIVE_NAME) azure-vnet$(EXE_EXT) 10-azure.conflist
endif

# Create a CNM archive for the target platform.
.PHONY: cnm-archive
cnm-archive: cnm-binary
	cd $(CNM_BUILD_DIR) && $(ARCHIVE_CMD) $(CNM_ARCHIVE_NAME) azure-vnet-plugin$(EXE_EXT)

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
ifeq ($(GOOS),linux)
	cd $(NPM_BUILD_DIR) && $(ARCHIVE_CMD) $(NPM_ARCHIVE_NAME) azure-npm$(EXE_EXT)
endif

# Create a azure-ipam archive for the target platform.
.PHONY: azure-ipam-archive
azure-ipam-archive: azure-ipam-binary
ifeq ($(GOOS),linux)
	$(MKDIR) $(AZURE_IPAM_BUILD_DIR)
	cd $(AZURE_IPAM_BUILD_DIR) && $(ARCHIVE_CMD) $(AZURE_IPAM_ARCHIVE_NAME) azure-ipam$(EXE_EXT)
endif


##@ Utils

clean: ## Clean build artifacts.
	$(RMDIR) $(OUTPUT_DIR)
	$(RMDIR) $(TOOLS_BIN_DIR)
	$(RMDIR) go.work*


LINT_PKG ?= .

lint: $(GOLANGCI_LINT) ## Fast lint vs default branch showing only new issues.
	GOGC=20 $(GOLANGCI_LINT) run --new-from-rev master --timeout 10m -v $(LINT_PKG)/...

lint-all: $(GOLANGCI_LINT) ## Lint the current branch in entirety.
	GOGC=20 $(GOLANGCI_LINT) run -v $(LINT_PKG)/...


FMT_PKG ?= cni cns npm

fmt: $(GOFUMPT) ## run gofumpt on $FMT_PKG (default "cni cns npm").
	$(GOFUMPT) -s -w $(FMT_PKG)


workspace: ## Set up the Go workspace.
	go work init
	go work use .
	go work use ./azure-ipam
	go work use ./build/tools
	go work use ./dropgz
	go work use ./zapai

##@ Test

COVER_PKG ?= .
#Restart case is used for cni load test pipeline for restarting the nodes cluster.
RESTART_CASE ?= false
# CNI type is a key to direct the types of state validation done on a cluster.
CNI_TYPE ?= cilium

# COVER_FILTER omits folders with all files tagged with one of 'unit', '!ignore_uncovered', or '!ignore_autogenerated'
test-all: ## run all unit tests.
	@$(eval COVER_FILTER=`go list --tags ignore_uncovered,ignore_autogenerated $(COVER_PKG)/... | tr '\n' ','`)
	@echo Test coverpkg: $(COVER_FILTER)
	go test -mod=readonly -buildvcs=false -tags "unit" -coverpkg=$(COVER_FILTER) -race -covermode atomic -coverprofile=coverage.out $(COVER_PKG)/...

test-integration: ## run all integration tests.
	CNI_DROPGZ_VERSION=$(CNI_DROPGZ_VERSION) \
		CNS_VERSION=$(CNS_VERSION) \
		go test -mod=readonly -buildvcs=false -timeout 1h -coverpkg=./... -race -covermode atomic -coverprofile=coverage.out -tags=integration ./test/integration...

test-validate-state:
	cd test/integration/load && go test -mod=readonly -count=1 -timeout 30m -tags load -run ^TestValidateState -restart-case=$(RESTART_CASE) -os=$(OS) -cni=$(CNI_TYPE)
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
	cd $(AZURE_IPAM_DIR) && go test

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

$(TOOLS_DIR)/go.mod:
	cd $(TOOLS_DIR); go mod init && go mod tidy

$(CONTROLLER_GEN): $(TOOLS_DIR)/go.mod
	cd $(TOOLS_DIR); go mod download; go build -o bin/controller-gen sigs.k8s.io/controller-tools/cmd/controller-gen

controller-gen: $(CONTROLLER_GEN) ## Build controller-gen

protoc:
	source ${REPO_ROOT}/scripts/install-protoc.sh

$(GOCOV): $(TOOLS_DIR)/go.mod
	cd $(TOOLS_DIR); go mod download; go build -o bin/gocov github.com/axw/gocov/gocov

gocov: $(GOCOV) ## Build gocov

$(GOCOV_XML): $(TOOLS_DIR)/go.mod
	cd $(TOOLS_DIR); go mod download; go build -o bin/gocov-xml github.com/AlekSi/gocov-xml

gocov-xml: $(GOCOV_XML) ## Build gocov-xml

$(GOFUMPT): $(TOOLS_DIR)/go.mod
	cd $(TOOLS_DIR); go mod download; go build -o bin/gofumpt mvdan.cc/gofumpt

gofumpt: $(GOFUMPT) ## Build gofumpt

$(GOLANGCI_LINT): $(TOOLS_DIR)/go.mod
	cd $(TOOLS_DIR); go mod download; go build -o bin/golangci-lint github.com/golangci/golangci-lint/cmd/golangci-lint

golangci-lint: $(GOLANGCI_LINT) ## Build golangci-lint

$(GO_JUNIT_REPORT): $(TOOLS_DIR)/go.mod
	cd $(TOOLS_DIR); go mod download; go build -o bin/go-junit-report github.com/jstemmer/go-junit-report

go-junit-report: $(GO_JUNIT_REPORT) ## Build go-junit-report

$(MOCKGEN): $(TOOLS_DIR)/go.mod
	cd $(TOOLS_DIR); go mod download; go build -o bin/mockgen github.com/golang/mock/mockgen

mockgen: $(MOCKGEN) ## Build mockgen

clean-tools:
	rm -r build/tools/bin

tools: acncli gocov gocov-xml go-junit-report golangci-lint gofumpt protoc ## Build bins for build tools


##@ Help

help: ## Display this help
	@awk 'BEGIN {FS = ":.*##"; printf "\nUsage:\n  make \033[36m<target>\033[0m\n"} /^[0-9a-zA-Z_-]+:.*?##/ { printf "  \033[36m%-15s\033[0m %s\n", $$1, $$2 } /^##@/ { printf "\n\033[1m%s\033[0m\n", substr($$0, 5) } ' $(MAKEFILE_LIST)
