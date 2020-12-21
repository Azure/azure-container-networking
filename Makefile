# Cross-platform directory creation/deletion
MKDIR := mkdir -p
RMDIR := rm -rf
ifeq ($(OS),Windows_NT)
MKDIR := powershell.exe -NoProfile -Command New-Item -ItemType Directory -Force
RMDIR := powershell.exe -NoProfile -Command Remove-Item -Recurse -Force
endif

# Go environment
GOOS := $(shell go env GOOS)
GOARCH := $(shell go env GOARCH)

# Target OS specific parameters.
ifeq ($(GOOS),linux)
	# Linux.
	ARCHIVE_CMD = tar -czvf
	ARCHIVE_EXT = tgz
else
	# Windows.
	ARCHIVE_CMD = zip -9lq
	ARCHIVE_EXT = zip
	EXE_EXT = .exe
endif

# Build directories.
ROOT_DIR := $(shell git rev-parse --show-toplevel)
CNM_DIR = cnm/plugin
CNI_NET_DIR = cni/network/plugin
CNI_IPAM_DIR = cni/ipam/plugin
CNI_IPAMV6_DIR = cni/ipam/pluginv6
CNI_TELEMETRY_DIR = cni/telemetry/service
ACNCLI_DIR = hack/acncli
TELEMETRY_CONF_DIR = telemetry
CNS_DIR = cns/service
CNMS_DIR = cnms/service
NPM_DIR = npm/plugin
OUTPUT_DIR = output
BUILD_DIR = $(ROOT_DIR)/$(OUTPUT_DIR)/$(GOOS)_$(GOARCH)
CNM_BUILD_DIR = $(BUILD_DIR)/cnm
CNI_BUILD_DIR = $(BUILD_DIR)/cni
ACNCLI_BUILD_DIR = $(BUILD_DIR)/acncli
CNI_MULTITENANCY_BUILD_DIR = $(BUILD_DIR)/cni-multitenancy
CNI_SWIFT_BUILD_DIR = $(BUILD_DIR)/cni-swift
CNS_BUILD_DIR = $(BUILD_DIR)/cns
CNMS_BUILD_DIR = $(BUILD_DIR)/cnms
NPM_BUILD_DIR = $(BUILD_DIR)/npm
NPM_TELEMETRY_DIR = $(NPM_BUILD_DIR)/telemetry
CNI_AI_ID = 5515a1eb-b2bc-406a-98eb-ba462e6f0411
NPM_AI_ID = 014c22bd-4107-459e-8475-67909e96edcb
ACN_PACKAGE_PATH = github.com/Azure/azure-container-networking

# Archive file names.
CNM_ARCHIVE_NAME = azure-vnet-cnm-$(GOOS)-$(GOARCH)-$(VERSION).$(ARCHIVE_EXT)
CNI_ARCHIVE_NAME = azure-vnet-cni-$(GOOS)-$(GOARCH)-$(VERSION).$(ARCHIVE_EXT)
ACNCLI_ARCHIVE_NAME = acncli-$(GOOS)-$(GOARCH)-$(VERSION).$(ARCHIVE_EXT)
CNI_MULTITENANCY_ARCHIVE_NAME = azure-vnet-cni-multitenancy-$(GOOS)-$(GOARCH)-$(VERSION).$(ARCHIVE_EXT)
CNI_SWIFT_ARCHIVE_NAME = azure-vnet-cni-swift-$(GOOS)-$(GOARCH)-$(VERSION).$(ARCHIVE_EXT)
CNS_ARCHIVE_NAME = azure-cns-$(GOOS)-$(GOARCH)-$(VERSION).$(ARCHIVE_EXT)
CNMS_ARCHIVE_NAME = azure-cnms-$(GOOS)-$(GOARCH)-$(VERSION).$(ARCHIVE_EXT)
NPM_ARCHIVE_NAME = azure-npm-$(GOOS)-$(GOARCH)-$(VERSION).$(ARCHIVE_EXT)
NPM_IMAGE_ARCHIVE_NAME = azure-npm-$(GOOS)-$(GOARCH)-$(VERSION).$(ARCHIVE_EXT)
TELEMETRY_IMAGE_ARCHIVE_NAME = azure-vnet-telemetry-$(GOOS)-$(GOARCH)-$(VERSION).$(ARCHIVE_EXT)
CNS_IMAGE_ARCHIVE_NAME = azure-cns-$(GOOS)-$(GOARCH)-$(VERSION).$(ARCHIVE_EXT)

# Docker libnetwork (CNM) plugin v2 image parameters.
CNM_PLUGIN_IMAGE ?= microsoft/azure-vnet-plugin
CNM_PLUGIN_ROOTFS = azure-vnet-plugin-rootfs

IMAGE_REGISTRY ?= acnpublic.azurecr.io

# Azure network policy manager parameters.
AZURE_NPM_IMAGE ?= $(IMAGE_REGISTRY)/azure-npm

# Azure CNI installer parameters
AZURE_CNI_IMAGE = $(IMAGE_REGISTRY)/azure-cni-manager

# Azure vnet telemetry image parameters.
AZURE_VNET_TELEMETRY_IMAGE = $(IMAGE_REGISTRY)/azure-vnet-telemetry

# Azure container networking service image paramters.
AZURE_CNS_IMAGE = $(IMAGE_REGISTRY)/azure-cns

VERSION ?= $(shell git describe --tags --always --dirty)
CNS_AI_ID = ce672799-8f08-4235-8c12-08563dc2acef
cnsaipath=github.com/Azure/azure-container-networking/cns/logger.aiMetadata

# Default target is "all-binaries"
.PHONY: all-binaries
ifeq ($(GOOS),linux)
all-binaries: azure-cnm-plugin azure-cni-plugin azure-cns azure-cnms azure-npm 
else
all-binaries: azure-cnm-plugin azure-cni-plugin azure-cns
endif

# Make both linux and windows binaries
.PHONY: all-binaries-platforms
all-binaries-platforms: 
	export GOOS=linux; make all-binaries
	export GOOS=windows; make all-binaries

# Shorthand target names for convenience.
azure-cnm-plugin: cnm-binary cnm-archive
azure-cni-plugin: azure-vnet-binary azure-vnet-ipam-binary azure-vnet-ipamv6-binary azure-vnet-telemetry-binary cni-archive
azure-cns: azure-cns-binary cns-archive
acncli: acncli-binary acncli-archive

# Azure-NPM only supports Linux for now.
ifeq ($(GOOS),linux)
azure-cnms: azure-cnms-binary cnms-archive
azure-npm: azure-npm-binary npm-archive
endif

# Clean all build artifacts.
.PHONY: clean
clean:
	$(RMDIR) $(OUTPUT_DIR)

########################### Binaries ###########################

# Build cnm
.PHONY: cnm-binary
cnm-binary:
	cd $(CNM_DIR) && go build -mod=vendor -v -o $(CNM_BUILD_DIR)/azure-vnet-plugin$(EXE_EXT) -ldflags "-X main.version=$(VERSION) -s -w"

# Build the Azure CNI network plugin.
.PHONY: azure-vnet-binary
azure-vnet-binary:
	cd $(CNI_NET_DIR) && go build -mod=vendor -v -o $(CNI_BUILD_DIR)/azure-vnet$(EXE_EXT) -ldflags "-X main.version=$(VERSION) -s -w"

# Build the Azure CNI IPAM plugin.
.PHONY: azure-vnet-ipam-binary
azure-vnet-ipam-binary:
	cd $(CNI_IPAM_DIR) && go build -mod=vendor -v -o $(CNI_BUILD_DIR)/azure-vnet-ipam$(EXE_EXT) -ldflags "-X main.version=$(VERSION) -s -w"

# Build the Azure CNI IPAMV6 plugin.
.PHONY: azure-vnet-ipamv6-binary
azure-vnet-ipamv6-binary:
	cd $(CNI_IPAMV6_DIR) && go build -mod=vendor -v -o $(CNI_BUILD_DIR)/azure-vnet-ipamv6$(EXE_EXT) -ldflags "-X main.version=$(VERSION) -s -w"

# Build the Azure CNI telemetry plugin.
.PHONY: azure-vnet-telemetry-binary
azure-vnet-telemetry-binary:
	cd $(CNI_TELEMETRY_DIR) && go build -mod=vendor -v -o $(CNI_BUILD_DIR)/azure-vnet-telemetry$(EXE_EXT) -ldflags "-X main.version=$(VERSION) -X $(ACN_PACKAGE_PATH)/telemetry.aiMetadata=$(CNI_AI_ID) -s -w"

# Build the Azure CLI network plugin.
.PHONY: acncli-binary
acncli-binary: export CGO_ENABLED = 0
acncli-binary:
	cd $(ACNCLI_DIR) && go build -mod=vendor -v -o $(ACNCLI_BUILD_DIR)/acn$(EXE_EXT) -ldflags "-X main.version=$(VERSION) -s -w"

# Build the Azure CNS Service.
.PHONY: azure-cns-binary
azure-cns-binary:
	cd $(CNS_DIR) && go build -mod=vendor -v -o $(CNS_BUILD_DIR)/azure-cns$(EXE_EXT) -ldflags "-X main.version=$(VERSION) -X $(cnsaipath)=$(CNS_AI_ID) -s -w"

# Build the Azure CNMS Service.
.PHONY: azure-cnms-binary
azure-cnms-binary:
	cd $(CNMS_DIR) && go build -mod=vendor -v -o $(CNMS_BUILD_DIR)/azure-cnms$(EXE_EXT) -ldflags "-X main.version=$(VERSION) -s -w"

# Build the Azure NPM plugin.
.PHONY: azure-npm-binary
azure-npm-binary:
	cd $(CNI_TELEMETRY_DIR) && go build -mod=vendor -v -o $(NPM_BUILD_DIR)/azure-vnet-telemetry$(EXE_EXT) -ldflags "-X main.version=$(VERSION) -s -w"
	cd $(NPM_DIR) && go build -mod=vendor -v -o $(NPM_BUILD_DIR)/azure-npm$(EXE_EXT) -ldflags "-X main.version=$(VERSION) -X $(ACN_PACKAGE_PATH)/npm.aiMetadata=$(NPM_AI_ID) -s -w"

# Build the hack binaries
.PHONY: hack
hack: acncli

########################### Container Images ###########################

# Build the hack images
.PHONY: hack-images
hack-images: 
	docker build -f ./hack/acncli/Dockerfile --build-arg VERSION=$(VERSION) -t $(AZURE_CNI_IMAGE):$(VERSION) .

# Build the Azure CNM plugin image, installable with "docker plugin install".
.PHONY: azure-cnm-plugin-image
azure-cnm-plugin-image: azure-cnm-plugin
	# Build the plugin image, keeping any old image during build for cache, but remove it afterwards.
	docker images -q $(CNM_PLUGIN_ROOTFS):$(VERSION) > cid
	docker build \
		-f Dockerfile.cnm \
		-t $(CNM_PLUGIN_ROOTFS):$(VERSION) \
		--build-arg CNM_BUILD_DIR=$(CNM_BUILD_DIR) \
		.
	$(eval CID := `cat cid`)
	docker rmi $(CID) || true

	# Create a container using the image and export its rootfs.
	docker create $(CNM_PLUGIN_ROOTFS):$(VERSION) > cid
	$(eval CID := `cat cid`)
	$(MKDIR) $(OUTPUT_DIR)/$(CID)/rootfs
	docker export $(CID) | tar -x -C $(OUTPUT_DIR)/$(CID)/rootfs
	docker rm -vf $(CID)

	# Copy the plugin configuration and set ownership.
	cp cnm/config.json $(OUTPUT_DIR)/$(CID)
	chgrp -R docker $(OUTPUT_DIR)/$(CID)

	# Create the plugin.
	docker plugin rm $(CNM_PLUGIN_IMAGE):$(VERSION) || true
	docker plugin create $(CNM_PLUGIN_IMAGE):$(VERSION) $(OUTPUT_DIR)/$(CID)

	# Cleanup temporary files.
	$(MKDIR) $(OUTPUT_DIR)/$(CID)
	$(RMDIR)

# Build the Azure NPM image.
.PHONY: azure-npm-image
azure-npm-image: azure-npm-binary
ifeq ($(GOOS),linux)
	docker build \
	--no-cache \
	-f npm/Dockerfile \
	-t $(AZURE_NPM_IMAGE):$(VERSION) \
	--build-arg NPM_BUILD_DIR=$(NPM_BUILD_DIR) \
	.
	docker save $(AZURE_NPM_IMAGE):$(VERSION) | gzip -c > $(NPM_BUILD_DIR)/$(NPM_IMAGE_ARCHIVE_NAME)
endif

# Build the Azure vnet telemetry image
.PHONY: azure-vnet-telemetry-image
azure-vnet-telemetry-image: azure-vnet-telemetry-binary
	docker build \
	-f cni/telemetry/Dockerfile \
	-t $(AZURE_VNET_TELEMETRY_IMAGE):$(VERSION) \
	--build-arg TELEMETRY_BUILD_DIR=$(NPM_BUILD_DIR) \
	--build-arg TELEMETRY_CONF_DIR=$(TELEMETRY_CONF_DIR) \
	.
	docker save $(AZURE_VNET_TELEMETRY_IMAGE):$(VERSION) | gzip -c > $(NPM_BUILD_DIR)/$(TELEMETRY_IMAGE_ARCHIVE_NAME)

# Build the Azure CNS image.
.PHONY: azure-cns-image
azure-cns-image: azure-cns-binary
ifeq ($(GOOS),linux)
	docker build \
	-f cns/Dockerfile \
	-t $(AZURE_CNS_IMAGE):$(VERSION) \
	--build-arg CNS_BUILD_ARCHIVE=$(CNS_BUILD_DIR)/$(CNS_IMAGE_ARCHIVE_NAME) \
	.
	docker save $(AZURE_CNS_IMAGE):$(VERSION) | gzip -c > $(CNS_BUILD_DIR)/$(CNS_IMAGE_ARCHIVE_NAME)
endif

# Build the Azure CNS image for AKS Swift.
.PHONY: azure-cns-aks-swift-image
azure-cns-aks-swift-image:
ifeq ($(GOOS),linux)
	docker build \
	-f cns/aks.Dockerfile \
	-t $(AZURE_CNS_IMAGE):$(VERSION) \
	--build-arg VERSION=$(VERSION) \
	--build-arg CNS_AI_PATH=$(cnsaipath) \
	--build-arg CNS_AI_ID=$(CNS_AI_ID) \
	.
endif

########################### Archives ###########################

# Create a CNI archive for the target platform.
.PHONY: cni-archive
cni-archive:
	cp cni/azure-$(GOOS).conflist $(CNI_BUILD_DIR)/10-azure.conflist
	cp telemetry/azure-vnet-telemetry.config $(CNI_BUILD_DIR)/azure-vnet-telemetry.config
	cd $(CNI_BUILD_DIR) && $(ARCHIVE_CMD) $(CNI_ARCHIVE_NAME) azure-vnet$(EXE_EXT) azure-vnet-ipam$(EXE_EXT) azure-vnet-ipamv6$(EXE_EXT) azure-vnet-telemetry$(EXE_EXT) 10-azure.conflist azure-vnet-telemetry.config

	$(MKDIR) $(CNI_MULTITENANCY_BUILD_DIR)
	cp cni/azure-$(GOOS)-multitenancy.conflist $(CNI_MULTITENANCY_BUILD_DIR)/10-azure.conflist
	cp telemetry/azure-vnet-telemetry.config $(CNI_MULTITENANCY_BUILD_DIR)/azure-vnet-telemetry.config
	cp $(CNI_BUILD_DIR)/azure-vnet$(EXE_EXT) $(CNI_BUILD_DIR)/azure-vnet-ipam$(EXE_EXT) $(CNI_BUILD_DIR)/azure-vnet-telemetry$(EXE_EXT) $(CNI_MULTITENANCY_BUILD_DIR)
	cd $(CNI_MULTITENANCY_BUILD_DIR) && $(ARCHIVE_CMD) $(CNI_MULTITENANCY_ARCHIVE_NAME) azure-vnet$(EXE_EXT) azure-vnet-ipam$(EXE_EXT) azure-vnet-telemetry$(EXE_EXT) 10-azure.conflist azure-vnet-telemetry.config

#swift mode is linux only
ifeq ($(GOOS),linux)
	$(MKDIR) $(CNI_SWIFT_BUILD_DIR)
	cp cni/azure-$(GOOS)-swift.conflist $(CNI_SWIFT_BUILD_DIR)/10-azure.conflist
	cp telemetry/azure-vnet-telemetry.config $(CNI_SWIFT_BUILD_DIR)/azure-vnet-telemetry.config
	cp $(CNI_BUILD_DIR)/azure-vnet$(EXE_EXT) $(CNI_BUILD_DIR)/azure-vnet-ipam$(EXE_EXT) $(CNI_BUILD_DIR)/azure-vnet-telemetry$(EXE_EXT) $(CNI_SWIFT_BUILD_DIR)
	cd $(CNI_SWIFT_BUILD_DIR) && $(ARCHIVE_CMD) $(CNI_SWIFT_ARCHIVE_NAME) azure-vnet$(EXE_EXT) azure-vnet-ipam$(EXE_EXT) azure-vnet-telemetry$(EXE_EXT) 10-azure.conflist azure-vnet-telemetry.config
endif	

# Create a CNM archive for the target platform.
.PHONY: cnm-archive
cnm-archive:
	cd $(CNM_BUILD_DIR) && $(ARCHIVE_CMD) $(CNM_ARCHIVE_NAME) azure-vnet-plugin$(EXE_EXT)

# Create a CNM archive for the target platform.
.PHONY: acncli-archive
acncli-archive:
ifeq ($(GOOS),linux)
	$(MKDIR) $(ACNCLI_BUILD_DIR)
	cd $(ACNCLI_BUILD_DIR) && $(ARCHIVE_CMD) $(ACNCLI_ARCHIVE_NAME) acn$(EXE_EXT)
endif

# Create a CNS archive for the target platform.
.PHONY: cns-archive
cns-archive:
	cp cns/configuration/cns_config.json $(CNS_BUILD_DIR)/cns_config.json
	cd $(CNS_BUILD_DIR) && $(ARCHIVE_CMD) $(CNS_ARCHIVE_NAME) azure-cns$(EXE_EXT) cns_config.json

# Create a CNMS archive for the target platform. Only Linux is supported for now.
.PHONY: cnms-archive
cnms-archive:
ifeq ($(GOOS),linux)
	cd $(CNMS_BUILD_DIR) && $(ARCHIVE_CMD) $(CNMS_ARCHIVE_NAME) azure-cnms$(EXE_EXT)
endif

# Create a NPM archive for the target platform. Only Linux is supported for now.
.PHONY: npm-archive
npm-archive:
ifeq ($(GOOS),linux)
	cd $(NPM_BUILD_DIR) && $(ARCHIVE_CMD) $(NPM_ARCHIVE_NAME) azure-npm$(EXE_EXT)
endif

########################### Tasks ###########################

# Publish the Azure NPM image to a Docker registry
.PHONY: publish-azure-npm-image
publish-azure-npm-image:
	docker push $(AZURE_NPM_IMAGE):$(VERSION)

# Publish the Azure CNM plugin image to a Docker registry.
.PHONY: publish-azure-cnm-plugin-image
publish-azure-cnm-plugin-image:
	docker plugin push $(CNM_PLUGIN_IMAGE):$(VERSION)

# Publish the Azure vnet telemetry image to a Docker registry
.PHONY: publish-azure-vnet-telemetry-image
publish-azure-vnet-telemetry-image:
	docker push $(AZURE_VNET_TELEMETRY_IMAGE):$(VERSION)

# Publish the Azure CNS image to a Docker registry
.PHONY: publish-azure-cns-image
publish-azure-cns-image:
	docker push $(AZURE_CNS_IMAGE):$(VERSION)

# run all tests
.PHONY: test-all
test-all:
	go test -coverpkg=./... -v -race -covermode atomic -coverprofile=coverage.out ./...