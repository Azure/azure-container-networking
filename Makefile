.DEFAULT_GOAL := help

# Default platform commands
SHELL=/bin/bash
MKDIR := mkdir -p
RMDIR := rm -rf
ARCHIVE_CMD = tar -czvf

# Default platform extensions
ARCHIVE_EXT = tgz

# Windows specific commands
ifeq ($(OS),Windows_NT)
MKDIR := powershell.exe -NoProfile -Command New-Item -ItemType Directory -Force
RMDIR := powershell.exe -NoProfile -Command Remove-Item -Recurse -Force
endif

# Build defaults.
GOOS ?= $(shell go env GOOS)
GOARCH ?= $(shell go env GOARCH)
GOOSES ?= "linux windows" # To override at the cli do: GOOSES="\"darwin bsd\""
GOARCHES ?= "amd64 arm64" # To override at the cli do: GOARCHES="\"ppc64 mips\""
WINVER ?= "10.0.20348.643"

# Windows specific extensions
ifeq ($(GOOS),windows)
ARCHIVE_CMD = zip -9lq
ARCHIVE_EXT = zip
EXE_EXT = .exe
endif

# Build directories.
REPO_ROOT = $(shell git rev-parse --show-toplevel)
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
AUZRE_IPAM_BUILD_DIR = $(BUILD_DIR)/azure-ipam
IMAGE_DIR  = $(OUTPUT_DIR)/images
CNM_BUILD_DIR = $(BUILD_DIR)/cnm
CNI_BUILD_DIR = $(BUILD_DIR)/cni
ACNCLI_BUILD_DIR = $(BUILD_DIR)/acncli
CNI_MULTITENANCY_BUILD_DIR = $(BUILD_DIR)/cni-multitenancy
CNI_MULTITENANCY_TRANSPARENT_VLAN_BUILD_DIR = $(BUILD_DIR)/cni-multitenancy-transparent-vlan
CNI_SWIFT_BUILD_DIR = $(BUILD_DIR)/cni-swift
CNI_OVERLAY_BUILD_DIR = $(BUILD_DIR)/cni-overlay
CNI_BAREMETAL_BUILD_DIR = $(BUILD_DIR)/cni-baremetal
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
CONTROLLER_GEN := $(TOOLS_BIN_DIR)/controller-gen
GOCOV := $(TOOLS_BIN_DIR)/gocov
GOCOV_XML := $(TOOLS_BIN_DIR)/gocov-xml
GOFUMPT := $(TOOLS_BIN_DIR)/gofumpt
GOLANGCI_LINT := $(TOOLS_BIN_DIR)/golangci-lint
GO_JUNIT_REPORT := $(TOOLS_BIN_DIR)/go-junit-report
MOCKGEN := $(TOOLS_BIN_DIR)/mockgen

# Archive file names.
CNM_ARCHIVE_NAME = azure-vnet-cnm-$(GOOS)-$(GOARCH)-$(VERSION).$(ARCHIVE_EXT)
CNI_ARCHIVE_NAME = azure-vnet-cni-$(GOOS)-$(GOARCH)-$(VERSION).$(ARCHIVE_EXT)
ACNCLI_ARCHIVE_NAME = acncli-$(GOOS)-$(GOARCH)-$(VERSION).$(ARCHIVE_EXT)
CNI_MULTITENANCY_ARCHIVE_NAME = azure-vnet-cni-multitenancy-$(GOOS)-$(GOARCH)-$(VERSION).$(ARCHIVE_EXT)
CNI_MULTITENANCY_TRANSPARENT_VLAN_ARCHIVE_NAME = azure-vnet-cni-multitenancy-transparent-vlan-$(GOOS)-$(GOARCH)-$(VERSION).$(ARCHIVE_EXT)
CNI_SWIFT_ARCHIVE_NAME = azure-vnet-cni-swift-$(GOOS)-$(GOARCH)-$(VERSION).$(ARCHIVE_EXT)
CNI_OVERLAY_ARCHIVE_NAME = azure-vnet-cni-overlay-$(GOOS)-$(GOARCH)-$(VERSION).$(ARCHIVE_EXT)
CNI_BAREMETAL_ARCHIVE_NAME = azure-vnet-cni-baremetal-$(GOOS)-$(GOARCH)-$(VERSION).$(ARCHIVE_EXT)
CNS_ARCHIVE_NAME = azure-cns-$(GOOS)-$(GOARCH)-$(VERSION).$(ARCHIVE_EXT)
NPM_ARCHIVE_NAME = azure-npm-$(GOOS)-$(GOARCH)-$(VERSION).$(ARCHIVE_EXT)
NPM_IMAGE_INFO_FILE = azure-npm-$(VERSION).txt
CNIDROPGZ_IMAGE_ARCHIVE_NAME = cni-dropgz-$(GOOS)-$(GOARCH)-$(VERSION).$(ARCHIVE_EXT)
CNIDROPGZ_IMAGE_INFO_FILE = cni-dropgz-$(VERSION).txt
CNS_IMAGE_INFO_FILE = azure-cns-$(VERSION).txt

# Docker libnetwork (CNM) plugin v2 image parameters.
CNM_PLUGIN_IMAGE ?= microsoft/azure-vnet-plugin
CNM_PLUGIN_ROOTFS = azure-vnet-plugin-rootfs

REVISION ?= $(shell git rev-parse --short HEAD)
VERSION  ?= $(shell git describe --exclude "zapai*" --tags --always --dirty)

# Default target
all-binaries-platforms: ## Make all platform binaries
	@for goos in "$(GOOSES)"; do \
		for goarch in "$(GOARCHES)"; do \
			make all-binaries GOOS=$$goos GOARCH=$$goarch; \
		done \
	done

# OS specific binaries/images
ifeq ($(GOOS),linux)
all-binaries: acncli azure-cnm-plugin azure-cni-plugin azure-cns azure-npm
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


##@ Binaries 

# Build the delegated IPAM plugin binary.
azure-ipam-binary:
	cd $(AZURE_IPAM_DIR) && CGO_ENABLED=0 go build -v -o $(AUZRE_IPAM_BUILD_DIR)/azure-ipam$(EXE_EXT) -ldflags "-X main.version=$(VERSION)" -gcflags="-dwarflocationlists=true"
# Build the Azure CNM binary.
cnm-binary:
	cd $(CNM_DIR) && CGO_ENABLED=0 go build -v -o $(CNM_BUILD_DIR)/azure-vnet-plugin$(EXE_EXT) -ldflags "-X main.version=$(VERSION)" -gcflags="-dwarflocationlists=true"

# Build the Azure CNI network binary.
azure-vnet-binary:
	cd $(CNI_NET_DIR) && CGO_ENABLED=0 go build -v -o $(CNI_BUILD_DIR)/azure-vnet$(EXE_EXT) -ldflags "-X main.version=$(VERSION)" -gcflags="-dwarflocationlists=true"

# Build the Azure CNI IPAM binary.
azure-vnet-ipam-binary:
	cd $(CNI_IPAM_DIR) && CGO_ENABLED=0 go build -v -o $(CNI_BUILD_DIR)/azure-vnet-ipam$(EXE_EXT) -ldflags "-X main.version=$(VERSION)" -gcflags="-dwarflocationlists=true"

# Build the Azure CNI IPAMV6 binary.
azure-vnet-ipamv6-binary:
	cd $(CNI_IPAMV6_DIR) && CGO_ENABLED=0 go build -v -o $(CNI_BUILD_DIR)/azure-vnet-ipamv6$(EXE_EXT) -ldflags "-X main.version=$(VERSION)" -gcflags="-dwarflocationlists=true"

# Build the Azure CNI telemetry binary.
azure-vnet-telemetry-binary:
	cd $(CNI_TELEMETRY_DIR) && CGO_ENABLED=0 go build -v -o $(CNI_BUILD_DIR)/azure-vnet-telemetry$(EXE_EXT) -ldflags "-X main.version=$(VERSION) -X $(CNI_AI_PATH)=$(CNI_AI_ID)" -gcflags="-dwarflocationlists=true"

# Build the Azure CLI network binary.
acncli-binary:
	cd $(ACNCLI_DIR) && CGO_ENABLED=0 go build -v -o $(ACNCLI_BUILD_DIR)/acn$(EXE_EXT) -ldflags "-X main.version=$(VERSION)" -gcflags="-dwarflocationlists=true"

# Build the Azure CNS binary.
azure-cns-binary:
	cd $(CNS_DIR) && CGO_ENABLED=0 go build -v -o $(CNS_BUILD_DIR)/azure-cns$(EXE_EXT) -ldflags "-X main.version=$(VERSION) -X $(CNS_AI_PATH)=$(CNS_AI_ID) -X $(CNI_AI_PATH)=$(CNI_AI_ID)" -gcflags="-dwarflocationlists=true"

# Build the Azure NPM binary.
azure-npm-binary:
	cd $(CNI_TELEMETRY_DIR) && CGO_ENABLED=0 go build -v -o $(NPM_BUILD_DIR)/azure-vnet-telemetry$(EXE_EXT) -ldflags "-X main.version=$(VERSION)" -gcflags="-dwarflocationlists=true"
	cd $(NPM_DIR) && CGO_ENABLED=0 go build -v -o $(NPM_BUILD_DIR)/azure-npm$(EXE_EXT) -ldflags "-X main.version=$(VERSION) -X $(NPM_AI_PATH)=$(NPM_AI_ID)" -gcflags="-dwarflocationlists=true"


##@ Containers

ACNCLI_IMAGE    = acncli
CNIDROPGZ_IMAGE = cni-dropgz
CNS_IMAGE       = azure-cns
NPM_IMAGE       = azure-npm

TAG               ?= $(VERSION)
IMAGE_REGISTRY    ?= acnpublic.azurecr.io
OS                ?= $(GOOS)
ARCH              ?= $(GOARCH)
PLATFORM          ?= $(GOOS)/$(GOARCH)
CONTAINER_BUILDER  = buildah

# prefer buildah, if available, but fall back to docker if that binary is not in the path.
ifeq (, $(shell which $(CONTAINER_BUILDER)))
CONTAINER_BUILDER = docker
endif

## This section is for building individual container images.

container-platform-tag: # util target to print the container tag
	@echo $(subst /,-,$(PLATFORM))-$(TAG)

containerize-buildah: # util target to build container images using buildah. do not invoke directly.
	buildah bud \
		--jobs 16 \
		--platform $(PLATFORM) \
		-f $(DOCKERFILE) \
		--build-arg VERSION=$(VERSION) $(EXTRA_BUILD_ARGS) \
		-t $(IMAGE_REGISTRY)/$(IMAGE):$(TAG) \
		.

containerize-docker: # util target to build container images using docker buildx. do not invoke directly.
	docker buildx build \
		--platform $(PLATFORM) \
		-f $(DOCKERFILE) \
		--build-arg VERSION=$(VERSION) $(EXTRA_BUILD_ARGS) \
		-t $(IMAGE_REGISTRY)/$(IMAGE):$(TAG) \
		.

container-tag-test: # util target to retag an image with -test suffix. do not invoke directly.
	$(CONTAINER_BUILDER) tag \
		$(IMAGE_REGISTRY)/$(IMAGE):$(TAG) \
		$(IMAGE_REGISTRY)/$(IMAGE):$(TAG)-test

container-push: # util target to publish container image. do not invoke directly.
	$(CONTAINER_BUILDER) push \
		$(IMAGE_REGISTRY)/$(IMAGE):$(TAG)

container-pull: # util target to pull container image. do not invoke directly.
	$(CONTAINER_BUILDER) pull \
		$(IMAGE_REGISTRY)/$(IMAGE):$(TAG)

container-info: # util target to write container info file. do not invoke directly.
	# these commands need to be root due to some ongoing perms issues in the pipeline.
	sudo mkdir -p $(IMAGE_DIR) 
	sudo chown -R $$(whoami) $(IMAGE_DIR) 
	sudo chmod -R 777 $(IMAGE_DIR)

acncli-image-name: # util target to print the CNI manager image name.
	@echo $(ACNCLI_IMAGE)

acncli-image: ## build cni-manager container image.
	$(MAKE) containerize-$(CONTAINER_BUILDER) \
		PLATFORM=$(PLATFORM) \
		DOCKERFILE=tools/acncli/Dockerfile \
		REGISTRY=$(IMAGE_REGISTRY) \
		IMAGE=$(ACNCLI_IMAGE) \
		TAG=$(TAG)

acncli-image-info: # util target to write cni-manager container info file.
	$(MAKE) container-info IMAGE=$(ACNCLI_IMAGE) TAG=$(TAG) FILE=$(ACNCLI_IMAGE_INFO_FILE)

acncli-image-push: ## push cni-manager container image.
	$(MAKE) container-push \
		PLATFORM=$(PLATFORM) \
		REGISTRY=$(IMAGE_REGISTRY) \
		IMAGE=$(ACNCLI_IMAGE) \
		TAG=$(TAG)

acncli-image-pull: ## pull cni-manager container image.
	$(MAKE) container-pull \
		PLATFORM=$(PLATFORM) \
		REGISTRY=$(IMAGE_REGISTRY) \
		IMAGE=$(ACNCLI_IMAGE) \
		TAG=$(TAG)

cni-dropgz-image-name: # util target to print the CNI dropgz image name.
	@echo $(CNIDROPGZ_IMAGE)

cni-dropgz-image: ## build cni-dropgz container image.
	$(MAKE) containerize-$(CONTAINER_BUILDER) \
		PLATFORM=$(PLATFORM) \
		DOCKERFILE=dropgz/build/cni.Dockerfile \
		REGISTRY=$(IMAGE_REGISTRY) \
		IMAGE=$(CNIDROPGZ_IMAGE) \
		TAG=$(TAG)

cni-dropgz-image-info: # util target to write cni-dropgz container info file.
	$(MAKE) container-info IMAGE=$(CNIDROPGZ_IMAGE) TAG=$(TAG) FILE=$(CNIDROPGZ_IMAGE_INFO_FILE)

cni-dropgz-image-push: ## push cni-dropgz container image.
	$(MAKE) container-push \
		PLATFORM=$(PLATFORM) \
		REGISTRY=$(IMAGE_REGISTRY) \
		IMAGE=$(CNIDROPGZ_IMAGE) \
		TAG=$(TAG)

cni-dropgz-image-pull: ## pull cni-dropgz container image.
	$(MAKE) container-pull \
		PLATFORM=$(PLATFORM) \
		REGISTRY=$(IMAGE_REGISTRY) \
		IMAGE=$(CNIDROPGZ_IMAGE) \
		TAG=$(TAG)

cns-image-name: # util target to print the CNS image name
	@echo $(CNS_IMAGE)

cns-image: ## build cns container image.
	$(MAKE) containerize-$(CONTAINER_BUILDER) \
		PLATFORM=$(PLATFORM) \
		DOCKERFILE=cns/Dockerfile \
		REGISTRY=$(IMAGE_REGISTRY) \
		IMAGE=$(CNS_IMAGE) \
		EXTRA_BUILD_ARGS='--build-arg CNS_AI_PATH=$(CNS_AI_PATH) --build-arg CNS_AI_ID=$(CNS_AI_ID)' \
		TAG=$(TAG)

cns-image-info: # util target to write cns container info file.
	$(MAKE) container-info IMAGE=$(CNS_IMAGE) TAG=$(TAG) FILE=$(CNS_IMAGE_INFO_FILE)

cns-image-push: ## push cns container image.
	$(MAKE) container-push \
		PLATFORM=$(PLATFORM) \
		REGISTRY=$(IMAGE_REGISTRY) \
		IMAGE=$(CNS_IMAGE) \
		TAG=$(TAG)

cns-image-pull: ## pull cns container image.
	$(MAKE) container-pull \
		PLATFORM=$(PLATFORM) \
		REGISTRY=$(IMAGE_REGISTRY) \
		IMAGE=$(CNS_IMAGE) \
		TAG=$(TAG)

npm-image-name: # util target to print the NPM image name
	@echo $(NPM_IMAGE)

npm-image: ## build the npm container image.
	$(MAKE) containerize-$(CONTAINER_BUILDER) \
			PLATFORM=$(PLATFORM) \
			DOCKERFILE=npm/Dockerfile \
			REGISTRY=$(IMAGE_REGISTRY) \
			IMAGE=$(NPM_IMAGE) \
			EXTRA_BUILD_ARGS='--build-arg NPM_AI_PATH=$(NPM_AI_PATH) --build-arg NPM_AI_ID=$(NPM_AI_ID)' \
			TAG=$(TAG)

npm-image-info: # util target to write npm container info file.
	$(MAKE) container-info IMAGE=$(NPM_IMAGE) TAG=$(TAG) FILE=$(NPM_IMAGE_INFO_FILE)

npm-image-push: ## push npm container image.
	$(MAKE) container-push \
		PLATFORM=$(PLATFORM) \
		REGISTRY=$(IMAGE_REGISTRY) \
		IMAGE=$(NPM_IMAGE) \
		TAG=$(TAG)

npm-image-pull: ## pull cns container image.
	$(MAKE) container-pull \
		PLATFORM=$(PLATFORM) \
		REGISTRY=$(IMAGE_REGISTRY) \
		IMAGE=$(NPM_IMAGE) \
		TAG=$(TAG)

## can probably be combined with above with a GOOS.Dockerfile change?
# Build the windows cns image
cns-image-windows:
	$(MKDIR) $(IMAGE_DIR); 
	docker build \
	--no-cache \
	-f cns/windows.Dockerfile \
	-t $(IMAGE_REGISTRY)/$(CNS_IMAGE)-win:$(TAG) \
	--build-arg VERSION=$(TAG) \
	--build-arg CNS_AI_PATH=$(CNS_AI_PATH) \
	--build-arg CNS_AI_ID=$(CNS_AI_ID) \
	.

	echo $(CNS_IMAGE)-win:$(TAG) > $(IMAGE_DIR)/$(CNS_IMAGE_INFO_FILE)


## Legacy

# Build the Azure CNM plugin image, installable with "docker plugin install".
azure-cnm-plugin-image: azure-cnm-plugin ## build the azure-cnm plugin container image.
	docker images -q $(CNM_PLUGIN_ROOTFS):$(TAG) > cid
	docker build --no-cache \
		-f Dockerfile.cnm \
		-t $(CNM_PLUGIN_ROOTFS):$(TAG) \
		--build-arg CNM_BUILD_DIR=$(CNM_BUILD_DIR) \
		.
	$(eval CID := `cat cid`)
	docker rmi $(CID) || true

	# Create a container using the image and export its rootfs.
	docker create $(CNM_PLUGIN_ROOTFS):$(TAG) > cid
	$(eval CID := `cat cid`)
	$(MKDIR) $(OUTPUT_DIR)/$(CID)/rootfs
	docker export $(CID) | tar -x -C $(OUTPUT_DIR)/$(CID)/rootfs
	docker rm -vf $(CID)

	# Copy the plugin configuration and set ownership.
	cp cnm/config.json $(OUTPUT_DIR)/$(CID)
	chgrp -R docker $(OUTPUT_DIR)/$(CID)

	# Create the plugin.
	docker plugin rm $(CNM_PLUGIN_IMAGE):$(TAG) || true
	docker plugin create $(CNM_PLUGIN_IMAGE):$(TAG) $(OUTPUT_DIR)/$(CID)

	# Cleanup temporary files.
	rm -rf $(OUTPUT_DIR)/$(CID)
	rm cid


## This section is for building multi-arch/os container image manifests.

multiarch-manifest-create: # util target to compose multiarch container manifests from os/arch images.
	$(CONTAINER_BUILDER) manifest create $(IMAGE_REGISTRY)/$(IMAGE):$(TAG)
	$(foreach PLATFORM,$(PLATFORMS),                                                                                                                                        \
		$(if $(filter $(PLATFORM),windows/amd64),                                                                                                                           \
			$(CONTAINER_BUILDER) manifest add --os-version=$(WINVER) $(IMAGE_REGISTRY)/$(IMAGE):$(TAG) docker://$(IMAGE_REGISTRY)/$(IMAGE):$(subst /,-,$(PLATFORM))-$(TAG); \
		,                                                                                                                                                                   \
			$(CONTAINER_BUILDER) manifest add $(IMAGE_REGISTRY)/$(IMAGE):$(TAG) docker://$(IMAGE_REGISTRY)/$(IMAGE):$(subst /,-,$(PLATFORM))-$(TAG);))

multiarch-manifest-push: # util target to push multiarch container manifest.
	$(CONTAINER_BUILDER) manifest push --all $(IMAGE_REGISTRY)/$(IMAGE):$(TAG) docker://$(IMAGE_REGISTRY)/$(IMAGE):$(TAG)

acncli-multiarch-manifest-create: ## build acncli multi-arch container manifest.
	$(MAKE) multiarch-manifest-create \
		PLATFORMS="$(PLATFORMS)" \
		IMAGE=$(ACNCLI_IMAGE) \
		TAG=$(TAG)

cni-dropgz-multiarch-manifest-create: ## build cni-dropgz multi-arch container manifest.
	$(MAKE) multiarch-manifest-create \
		PLATFORMS="$(PLATFORMS)" \
		IMAGE=$(CNIDROPGZ_IMAGE) \
		TAG=$(TAG)

cns-multiarch-manifest-create: ## build azure-cns multi-arch container manifest.
	$(MAKE) multiarch-manifest-create \
		PLATFORMS="$(PLATFORMS)" \
		IMAGE=$(CNS_IMAGE) \
		TAG=$(TAG)

npm-multiarch-manifest-create: ## build azure-npm multi-arch container manifest.
	$(MAKE) multiarch-manifest-create \
		PLATFORMS="$(PLATFORMS)" \
		IMAGE=$(NPM_IMAGE) \
		TAG=$(TAG)

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

########################### Tasks ###################################
# Release tag
.PHONY: release
release:
	./scripts/semver-release.sh

# Publish the Azure CNM plugin image to a Docker registry.
.PHONY: publish-azure-cnm-plugin-image
publish-azure-cnm-plugin-image:
	docker plugin push $(CNM_PLUGIN_IMAGE):$(VERSION)


##@ Utils 

clean: ## Clean build artifacts.
	$(RMDIR) $(OUTPUT_DIR)
	$(RMDIR) $(TOOLS_BIN_DIR)
	$(RMDIR) go.work*


LINT_PKG ?= .

lint: $(GOLANGCI_LINT) ## Fast lint vs default branch showing only new issues.
	$(GOLANGCI_LINT) run --new-from-rev master --timeout 10m -v $(LINT_PKG)/...

lint-all: $(GOLANGCI_LINT) ## Lint the current branch in entirety.
	$(GOLANGCI_LINT) run -v $(LINT_PKG)/...


FMT_PKG ?= cni cns npm

fmt: $(GOFUMPT) ## run gofumpt on $FMT_PKG (default "cni cns npm").
	$(GOFUMPT) -s -w $(FMT_PKG)


workspace: ## Set up the Go workspace.
	go work init
	go work use .
	go work use ./build/tools
	go work use ./dropgz
	go work use ./zapai

##@ Test 

COVER_PKG ?= .

# COVER_FILTER omits folders with all files tagged with one of 'unit', '!ignore_uncovered', or '!ignore_autogenerated'
test-all: ## run all unit tests.
	@$(eval COVER_FILTER=`go list --tags ignore_uncovered,ignore_autogenerated $(COVER_PKG)/... | tr '\n' ','`)
	@echo Test coverpkg: $(COVER_FILTER)
	go test -buildvcs=false -tags "unit" -coverpkg=$(COVER_FILTER) -race -covermode atomic -failfast -coverprofile=coverage.out $(COVER_PKG)/...


test-integration: ## run all integration tests.
	go test -buildvcs=false -timeout 1h -coverpkg=./... -race -covermode atomic -coverprofile=coverage.out -tags=integration ./test/integration...

test-cyclonus: ## run the cyclonus test for npm.
	cd test/cyclonus && bash ./test-cyclonus.sh
	cd ..

test-cyclonus-windows: ## run the cyclonus test for npm.
	cd test/cyclonus && bash ./test-cyclonus.sh windows
	cd ..

test-extended-cyclonus: ## run the cyclonus test for npm.
	cd test/cyclonus && bash ./test-cyclonus.sh extended
	cd ..

.PHONY: kind
kind:
	kind create cluster --config ./test/kind/kind.yaml


##@ Utilities

$(REPO_ROOT)/.git/hooks/pre-push:
	@ln -s $(REPO_ROOT)/.hooks/pre-push $(REPO_ROOT)/.git/hooks/
	@echo installed pre-push hook

install-hooks: $(REPO_ROOT)/.git/hooks/pre-push ## installs git hooks

setup: tools install-hooks ## performs common required repo setup

revision: ## print the current git revision
	@echo $(REVISION)
	
version: ## prints the version
	@echo $(VERSION)


##@ Tools 

$(TOOLS_DIR)/go.mod:
	cd $(TOOLS_DIR); go mod init && go mod tidy

$(CONTROLLER_GEN): $(TOOLS_DIR)/go.mod
	cd $(TOOLS_DIR); go mod download; go build -tags=tools -o bin/controller-gen sigs.k8s.io/controller-tools/cmd/controller-gen

controller-gen: $(CONTROLLER_GEN) ## Build controller-gen

protoc: 
	source ${REPO_ROOT}/scripts/install-protoc.sh

$(GOCOV): $(TOOLS_DIR)/go.mod
	cd $(TOOLS_DIR); go mod download; go build -tags=tools -o bin/gocov github.com/axw/gocov/gocov

gocov: $(GOCOV) ## Build gocov

$(GOCOV_XML): $(TOOLS_DIR)/go.mod
	cd $(TOOLS_DIR); go mod download; go build -tags=tools -o bin/gocov-xml github.com/AlekSi/gocov-xml

gocov-xml: $(GOCOV_XML) ## Build gocov-xml

$(GOFUMPT): $(TOOLS_DIR)/go.mod
	cd $(TOOLS_DIR); go mod download; go build -tags=tools -o bin/gofumpt mvdan.cc/gofumpt

gofumpt: $(GOFUMPT) ## Build gofumpt

$(GOLANGCI_LINT): $(TOOLS_DIR)/go.mod
	cd $(TOOLS_DIR); go mod download; go build -tags=tools -o bin/golangci-lint github.com/golangci/golangci-lint/cmd/golangci-lint

golangci-lint: $(GOLANGCI_LINT) ## Build golangci-lint

$(GO_JUNIT_REPORT): $(TOOLS_DIR)/go.mod
	cd $(TOOLS_DIR); go mod download; go build -tags=tools -o bin/go-junit-report github.com/jstemmer/go-junit-report

go-junit-report: $(GO_JUNIT_REPORT) ## Build go-junit-report

$(MOCKGEN): $(TOOLS_DIR)/go.mod
	cd $(TOOLS_DIR); go mod download; go build -tags=tools -o bin/mockgen github.com/golang/mock/mockgen

mockgen: $(MOCKGEN) ## Build mockgen

clean-tools: 
	rm -r build/tools/bin

tools: acncli gocov gocov-xml go-junit-report golangci-lint gofumpt protoc ## Build bins for build tools


##@ Help 

help: ## Display this help
	@awk 'BEGIN {FS = ":.*##"; printf "\nUsage:\n  make \033[36m<target>\033[0m\n"} /^[a-zA-Z_-]+:.*?##/ { printf "  \033[36m%-15s\033[0m %s\n", $$1, $$2 } /^##@/ { printf "\n\033[1m%s\033[0m\n", substr($$0, 5) } ' $(MAKEFILE_LIST)
