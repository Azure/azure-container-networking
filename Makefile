# Source files common to all targets.
COREFILES = \
	$(wildcard common/*.go) \
	$(wildcard ebtables/*.go) \
	$(wildcard ipam/*.go) \
	$(wildcard log/*.go) \
	$(wildcard netlink/*.go) \
	$(wildcard network/*.go) \
	$(wildcard platform/*.go) \
	$(wildcard store/*.go)

# Source files for building CNM plugin.
CNMFILES = \
	$(wildcard cnm/*.go) \
	$(wildcard cnm/ipam/*.go) \
	$(wildcard cnm/network/*.go) \
	$(wildcard cnm/plugin/*.go) \
	$(COREFILES)

# Source files for building CNI plugin.
CNIFILES = \
	$(wildcard cni/*.go) \
	$(wildcard cni/ipam/*.go) \
	$(wildcard cni/ipam/plugin/*.go) \
	$(wildcard cni/network/*.go) \
	$(wildcard cni/network/plugin/*.go) \
	$(COREFILES)

CNSFILES = \
	$(wildcard cns/*.go) \
	$(wildcard cns/cnsclient/*.go) \
	$(wildcard cns/common/*.go) \
	$(wildcard cns/dockerclient/*.go) \
	$(wildcard cns/imdsclient/*.go) \
	$(wildcard cns/ipamclient/*.go) \
	$(wildcard cns/restserver/*.go) \
	$(wildcard cns/routes/*.go) \
	$(wildcard cns/service/*.go) \
	$(COREFILES) \
	$(CNMFILES)

NPMFILES = \
	$(wildcard npm/*.go) \
	$(wildcard npm/ipsm/*.go) \
	$(wildcard npm/iptm/*.go) \
	$(wildcard npm/util/*.go) \
	$(wildcard npm/plugin/*.go) \
	$(COREFILES)

# Build defaults.
GOOS ?= linux
GOARCH ?= amd64

# Build directories.
CNM_DIR = cnm/plugin
CNI_NET_DIR = cni/network/plugin
CNI_IPAM_DIR = cni/ipam/plugin
CNS_DIR = cns/service
NPM_DIR = npm/plugin
OUTPUT_DIR = output
BUILD_DIR = $(OUTPUT_DIR)/$(GOOS)_$(GOARCH)
CNM_BUILD_DIR = $(BUILD_DIR)/cnm
CNI_BUILD_DIR = $(BUILD_DIR)/cni
CNI_MULTITENANCY_BUILD_DIR = $(BUILD_DIR)/cni-multitenancy
CNS_BUILD_DIR = $(BUILD_DIR)/cns
NPM_BUILD_DIR = $(BUILD_DIR)/npm

# Containerized build parameters.
BUILD_CONTAINER_IMAGE = acn-build
BUILD_CONTAINER_NAME = acn-builder
BUILD_CONTAINER_REPO_PATH = /go/src/github.com/Azure/azure-container-networking
BUILD_USER ?= $(shell id -u)

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

# Archive file names.
CNM_ARCHIVE_NAME = azure-vnet-cnm-$(GOOS)-$(GOARCH)-$(VERSION).$(ARCHIVE_EXT)
CNI_ARCHIVE_NAME = azure-vnet-cni-$(GOOS)-$(GOARCH)-$(VERSION).$(ARCHIVE_EXT)
CNI_MULTITENANCY_ARCHIVE_NAME = azure-vnet-cni-multitenancy-$(GOOS)-$(GOARCH)-$(VERSION).$(ARCHIVE_EXT)
CNS_ARCHIVE_NAME = azure-cns-$(GOOS)-$(GOARCH)-$(VERSION).$(ARCHIVE_EXT)
NPM_ARCHIVE_NAME = azure-npm-$(GOOS)-$(GOARCH)-$(VERSION).$(ARCHIVE_EXT)

# Docker libnetwork (CNM) plugin v2 image parameters.
CNM_PLUGIN_IMAGE ?= microsoft/azure-vnet-plugin
CNM_PLUGIN_ROOTFS = azure-vnet-plugin-rootfs

# Azure network policy manager parameters.
AZURE_NPM_IMAGE = containernetworking/azure-npm

VERSION ?= $(shell git describe --tags --always --dirty)
AZURE_NPM_VERSION = VERSION

ENSURE_OUTPUT_DIR_EXISTS := $(shell mkdir -p $(OUTPUT_DIR))

# Shorthand target names for convenience.
azure-cnm-plugin: $(CNM_BUILD_DIR)/azure-vnet-plugin$(EXE_EXT) cnm-archive
azure-vnet: $(CNI_BUILD_DIR)/azure-vnet$(EXE_EXT)
azure-vnet-ipam: $(CNI_BUILD_DIR)/azure-vnet-ipam$(EXE_EXT)
azure-cni-plugin: azure-vnet azure-vnet-ipam cni-archive
azure-cns: $(CNS_BUILD_DIR)/azure-cns$(EXE_EXT) cns-archive
# Azure-NPM only supports Linux for now.
ifeq ($(GOOS),linux)
azure-npm: $(NPM_BUILD_DIR)/azure-npm$(EXE_EXT) npm-archive
endif

ifeq ($(GOOS),linux)
all-binaries: azure-cnm-plugin azure-cni-plugin azure-cns azure-npm
else
all-binaries: azure-cnm-plugin azure-cni-plugin azure-cns
endif

# Clean all build artifacts.
.PHONY: clean
clean:
	rm -rf $(OUTPUT_DIR)

# Build the Azure CNM plugin.
$(CNM_BUILD_DIR)/azure-vnet-plugin$(EXE_EXT): $(CNMFILES)
	go build -v -o $(CNM_BUILD_DIR)/azure-vnet-plugin$(EXE_EXT) -ldflags "-X main.version=$(VERSION) -s -w" $(CNM_DIR)/*.go

# Build the Azure CNI network plugin.
$(CNI_BUILD_DIR)/azure-vnet$(EXE_EXT): $(CNIFILES)
	go build -v -o $(CNI_BUILD_DIR)/azure-vnet$(EXE_EXT) -ldflags "-X main.version=$(VERSION) -s -w" $(CNI_NET_DIR)/*.go

# Build the Azure CNI IPAM plugin.
$(CNI_BUILD_DIR)/azure-vnet-ipam$(EXE_EXT): $(CNIFILES)
	go build -v -o $(CNI_BUILD_DIR)/azure-vnet-ipam$(EXE_EXT) -ldflags "-X main.version=$(VERSION) -s -w" $(CNI_IPAM_DIR)/*.go

# Build the Azure CNS Service.
$(CNS_BUILD_DIR)/azure-cns$(EXE_EXT): $(CNSFILES)
	go build -v -o $(CNS_BUILD_DIR)/azure-cns$(EXE_EXT) -ldflags "-X main.version=$(VERSION) -s -w" $(CNS_DIR)/*.go

# Build the Azure NPM plugin.
$(NPM_BUILD_DIR)/azure-npm$(EXE_EXT): $(NPMFILES)
	go build -v -o $(NPM_BUILD_DIR)/azure-npm$(EXE_EXT) -ldflags "-X main.version=$(VERSION) -s -w" $(NPM_DIR)/*.go

# Build all binaries in a container.
.PHONY: all-binaries-containerized
all-binaries-containerized:
	pwd && ls -l
	docker build -f Dockerfile.build -t $(BUILD_CONTAINER_IMAGE):$(VERSION) .
	docker run --name $(BUILD_CONTAINER_NAME) \
		$(BUILD_CONTAINER_IMAGE):$(VERSION) \
		bash -c '\
			pwd && ls -l && \
			export GOOS=$(GOOS) && \
			export GOARCH=$(GOARCH) && \
			make all-binaries && \
			chown -R $(BUILD_USER):$(BUILD_USER) $(BUILD_DIR) \
		'
	docker cp $(BUILD_CONTAINER_NAME):$(BUILD_CONTAINER_REPO_PATH)/$(BUILD_DIR) $(OUTPUT_DIR)
	docker rm $(BUILD_CONTAINER_NAME)
	docker rmi $(BUILD_CONTAINER_IMAGE):$(VERSION)

# Build the Azure CNM plugin image, installable with "docker plugin install".
.PHONY: azure-vnet-plugin-image
azure-vnet-plugin-image: azure-cnm-plugin
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
	mkdir -p $(OUTPUT_DIR)/$(CID)/rootfs
	docker export $(CID) | tar -x -C $(OUTPUT_DIR)/$(CID)/rootfs
	docker rm -vf $(CID)

	# Copy the plugin configuration and set ownership.
	cp cnm/config.json $(OUTPUT_DIR)/$(CID)
	chgrp -R docker $(OUTPUT_DIR)/$(CID)

	# Create the plugin.
	docker plugin rm $(CNM_PLUGIN_IMAGE):$(VERSION) || true
	docker plugin create $(CNM_PLUGIN_IMAGE):$(VERSION) $(OUTPUT_DIR)/$(CID)

	# Cleanup temporary files.
	rm -rf $(OUTPUT_DIR)/$(CID)
	rm cid

# Publish the Azure CNM plugin image to a Docker registry.
.PHONY: publish-azure-vnet-plugin-image
publish-azure-vnet-plugin-image:
	docker plugin push $(CNM_PLUGIN_IMAGE):$(VERSION)

# Build the Azure NPM image.
.PHONY: azure-npm-image
azure-npm-image: azure-npm
	# Build the plugin image.
	docker build \
	-f npm/Dockerfile \
	-t $(AZURE_NPM_IMAGE):$(AZURE_NPM_VERSION) \
	--build-arg NPM_BUILD_DIR=$(NPM_BUILD_DIR) \
	.
	
# Publish the Azure NPM image to a Docker registry
.PHONY: publish-azure-npm-image
publish-azure-npm-image:
	docker push $(AZURE_NPM_IMAGE):$(AZURE_NPM_VERSION)

# Create a CNI archive for the target platform.
.PHONY: cni-archive
cni-archive:
	cp cni/azure-$(GOOS).conflist $(CNI_BUILD_DIR)/10-azure.conflist
	chmod 0755 $(CNI_BUILD_DIR)/azure-vnet$(EXE_EXT) $(CNI_BUILD_DIR)/azure-vnet-ipam$(EXE_EXT)
	cd $(CNI_BUILD_DIR) && $(ARCHIVE_CMD) $(CNI_ARCHIVE_NAME) azure-vnet$(EXE_EXT) azure-vnet-ipam$(EXE_EXT) 10-azure.conflist
	chown $(BUILD_USER):$(BUILD_USER) $(CNI_BUILD_DIR)/$(CNI_ARCHIVE_NAME)
	mkdir -p $(CNI_MULTITENANCY_BUILD_DIR)
	cp cni/azure-$(GOOS)-multitenancy.conflist $(CNI_MULTITENANCY_BUILD_DIR)/10-azure.conflist
	cp $(CNI_BUILD_DIR)/azure-vnet$(EXE_EXT) $(CNI_BUILD_DIR)/azure-vnet-ipam$(EXE_EXT) $(CNI_MULTITENANCY_BUILD_DIR)
	chmod 0755 $(CNI_MULTITENANCY_BUILD_DIR)/azure-vnet$(EXE_EXT) $(CNI_MULTITENANCY_BUILD_DIR)/azure-vnet-ipam$(EXE_EXT)
	cd $(CNI_MULTITENANCY_BUILD_DIR) && $(ARCHIVE_CMD) $(CNI_MULTITENANCY_ARCHIVE_NAME) azure-vnet$(EXE_EXT) azure-vnet-ipam$(EXE_EXT) 10-azure.conflist
	chown $(BUILD_USER):$(BUILD_USER) $(CNI_MULTITENANCY_BUILD_DIR)/$(CNI_MULTITENANCY_ARCHIVE_NAME)

# Create a CNM archive for the target platform.
.PHONY: cnm-archive
cnm-archive:
	chmod 0755 $(CNM_BUILD_DIR)/azure-vnet-plugin$(EXE_EXT)
	cd $(CNM_BUILD_DIR) && $(ARCHIVE_CMD) $(CNM_ARCHIVE_NAME) azure-vnet-plugin$(EXE_EXT)
	chown $(BUILD_USER):$(BUILD_USER) $(CNM_BUILD_DIR)/$(CNM_ARCHIVE_NAME)

# Create a CNS archive for the target platform.
.PHONY: cns-archive
cns-archive:
	chmod 0755 $(CNS_BUILD_DIR)/azure-cns$(EXE_EXT)
	cd $(CNS_BUILD_DIR) && $(ARCHIVE_CMD) $(CNS_ARCHIVE_NAME) azure-cns$(EXE_EXT)
	chown $(BUILD_USER):$(BUILD_USER) $(CNS_BUILD_DIR)/$(CNS_ARCHIVE_NAME)

# Create a NPM archive for the target platform. Only Linux is supported for now.
.PHONY: npm-archive
npm-archive:
ifeq ($(GOOS),linux)
	chmod 0755 $(NPM_BUILD_DIR)/azure-npm$(EXE_EXT)
	cd $(NPM_BUILD_DIR) && $(ARCHIVE_CMD) $(NPM_ARCHIVE_NAME) azure-npm$(EXE_EXT)
	chown $(BUILD_USER):$(BUILD_USER) $(NPM_BUILD_DIR)/$(NPM_ARCHIVE_NAME)
endif