# Source files common to all targets.
COREFILES = \
	$(wildcard common/*.go) \
	$(wildcard ebtables/*.go) \
	$(wildcard ipam/*.go) \
	$(wildcard log/*.go) \
	$(wildcard netlink/*.go) \
	$(wildcard network/*.go) \
	$(wildcard telemetry/*.go) \
	$(wildcard aitelemetry/*.go) \
	$(wildcard network/epcommon/*.go) \
	$(wildcard network/policy/*.go) \
	$(wildcard platform/*.go) \
	$(wildcard store/*.go) \
	$(wildcard ovsctl/*.go) \
	$(wildcard network/ovssnat*.go) \
	$(wildcard network/ovsinfravnet*.go)

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
	$(wildcard cni/telemetry/service/*.go) \
	$(COREFILES)

CNSFILES = \
	$(wildcard cns/*.go) \
	$(wildcard cns/cnsclient/*.go) \
	$(wildcard cns/common/*.go) \
	$(wildcard cns/configuration/*.go) \
	$(wildcard cns/dockerclient/*.go) \
	$(wildcard cns/imdsclient/*.go) \
	$(wildcard cns/ipamclient/*.go) \
	$(wildcard cns/hnsclient/*.go) \
	$(wildcard cns/logger/*.go) \
	$(wildcard cns/nmagentclient/*.go) \
	$(wildcard cns/restserver/*.go) \
	$(wildcard cns/routes/*.go) \
	$(wildcard cns/service/*.go) \
	$(wildcard cns/networkcontainers/*.go) \
	$(wildcard cns/requestcontroller/*.go) \
	$(wildcard cns/requestcontroller/kubecontroller/*.go) \
	$(wildcard cns/multitenantcontroller/*.go) \
	$(wildcard cns/multitenantcontroller/multitenantoperator/*.go) \
	$(wildcard cns/fakes/*.go) \
	$(COREFILES) \
	$(CNMFILES)

CNMSFILES = \
	$(wildcard cnms/*.go) \
	$(wildcard cnms/service/*.go) \
	$(wildcard cnms/cnmspackage/*.go) \
	$(COREFILES)

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
ROOT_DIR = $(shell pwd)
REPO_ROOT = $(shell git rev-parse --show-toplevel)
CNM_DIR = cnm/plugin
CNI_NET_DIR = cni/network/plugin
CNI_IPAM_DIR = cni/ipam/plugin
CNI_IPAMV6_DIR = cni/ipam/pluginv6
CNI_TELEMETRY_DIR = cni/telemetry/service
ACNCLI_DIR = tools/acncli
TELEMETRY_CONF_DIR = telemetry
CNS_DIR = cns/service
CNMS_DIR = cnms/service
NPM_DIR = npm/plugin
OUTPUT_DIR = output
BUILD_DIR = $(OUTPUT_DIR)/$(GOOS)_$(GOARCH)
IMAGE_DIR  = $(OUTPUT_DIR)/images
CNM_BUILD_DIR = $(BUILD_DIR)/cnm
CNI_BUILD_DIR = $(BUILD_DIR)/cni
ACNCLI_BUILD_DIR = $(BUILD_DIR)/acncli
CNI_MULTITENANCY_BUILD_DIR = $(BUILD_DIR)/cni-multitenancy
CNI_SWIFT_BUILD_DIR = $(BUILD_DIR)/cni-swift
CNI_BAREMETAL_BUILD_DIR = $(BUILD_DIR)/cni-baremetal
CNS_BUILD_DIR = $(BUILD_DIR)/cns
CNMS_BUILD_DIR = $(BUILD_DIR)/cnms
NPM_BUILD_DIR = $(BUILD_DIR)/npm
NPM_TELEMETRY_DIR = $(NPM_BUILD_DIR)/telemetry
TOOLS_DIR = $(REPO_ROOT)/build/tools
TOOLS_BIN_DIR = $(TOOLS_DIR)/bin
CNI_AI_ID = 5515a1eb-b2bc-406a-98eb-ba462e6f0411
NPM_AI_ID = 014c22bd-4107-459e-8475-67909e96edcb
ACN_PACKAGE_PATH = github.com/Azure/azure-container-networking

# Containerized build parameters.
BUILD_CONTAINER_IMAGE = acn-build
BUILD_CONTAINER_NAME = acn-builder
BUILD_CONTAINER_REPO_PATH = /go/src/github.com/Azure/azure-container-networking


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
ACNCLI_ARCHIVE_NAME = acncli-$(GOOS)-$(GOARCH)-$(VERSION).$(ARCHIVE_EXT)
CNI_MULTITENANCY_ARCHIVE_NAME = azure-vnet-cni-multitenancy-$(GOOS)-$(GOARCH)-$(VERSION).$(ARCHIVE_EXT)
CNI_SWIFT_ARCHIVE_NAME = azure-vnet-cni-swift-$(GOOS)-$(GOARCH)-$(VERSION).$(ARCHIVE_EXT)
CNI_BAREMETAL_ARCHIVE_NAME = azure-vnet-cni-baremetal-$(GOOS)-$(GOARCH)-$(VERSION).$(ARCHIVE_EXT)
CNS_ARCHIVE_NAME = azure-cns-$(GOOS)-$(GOARCH)-$(VERSION).$(ARCHIVE_EXT)
CNMS_ARCHIVE_NAME = azure-cnms-$(GOOS)-$(GOARCH)-$(VERSION).$(ARCHIVE_EXT)
NPM_ARCHIVE_NAME = azure-npm-$(GOOS)-$(GOARCH)-$(VERSION).$(ARCHIVE_EXT)
NPM_IMAGE_ARCHIVE_NAME = azure-npm-$(GOOS)-$(GOARCH)-$(VERSION).$(ARCHIVE_EXT)
CNI_IMAGE_ARCHIVE_NAME = azure-cni-manager-$(GOOS)-$(GOARCH)-$(VERSION).$(ARCHIVE_EXT)
CNMS_IMAGE_ARCHIVE_NAME = azure-cnms-$(GOOS)-$(GOARCH)-$(VERSION).$(ARCHIVE_EXT)
TELEMETRY_IMAGE_ARCHIVE_NAME = azure-vnet-telemetry-$(GOOS)-$(GOARCH)-$(VERSION).$(ARCHIVE_EXT)
CNS_IMAGE_ARCHIVE_NAME = azure-cns-$(GOOS)-$(GOARCH)-$(VERSION).$(ARCHIVE_EXT)

# Docker libnetwork (CNM) plugin v2 image parameters.
CNM_PLUGIN_IMAGE ?= microsoft/azure-vnet-plugin
CNM_PLUGIN_ROOTFS = azure-vnet-plugin-rootfs

IMAGE_REGISTRY ?= acnpublic.azurecr.io

# Azure network policy manager parameters.
AZURE_NPM_IMAGE ?= $(IMAGE_REGISTRY)/azure-npm

# Azure cnms parameters
AZURE_CNMS_IMAGE ?= $(IMAGE_REGISTRY)/networkmonitor

# Azure CNI installer parameters
AZURE_CNI_IMAGE = $(IMAGE_REGISTRY)/azure-cni-manager

# Azure vnet telemetry image parameters.
AZURE_VNET_TELEMETRY_IMAGE = $(IMAGE_REGISTRY)/azure-vnet-telemetry

# Azure container networking service image paramters.
AZURE_CNS_IMAGE = $(IMAGE_REGISTRY)/azure-cns

VERSION ?= $(shell git describe --tags --always --dirty)
CNS_AI_ID = ce672799-8f08-4235-8c12-08563dc2acef
cnsaipath=github.com/Azure/azure-container-networking/cns/logger.aiMetadata
ENSURE_OUTPUT_DIR_EXISTS := $(shell mkdir -p $(OUTPUT_DIR))

# Shorthand target names for convenience.
azure-cnm-plugin: $(CNM_BUILD_DIR)/azure-vnet-plugin$(EXE_EXT) cnm-archive
azure-vnet: $(CNI_BUILD_DIR)/azure-vnet$(EXE_EXT)
azure-vnet-ipam: $(CNI_BUILD_DIR)/azure-vnet-ipam$(EXE_EXT)
azure-vnet-ipamv6: $(CNI_BUILD_DIR)/azure-vnet-ipamv6$(EXE_EXT)
azure-cni-plugin: azure-vnet azure-vnet-ipam azure-vnet-ipamv6 azure-vnet-telemetry cni-archive
azure-cns: $(CNS_BUILD_DIR)/azure-cns$(EXE_EXT) cns-archive
azure-vnet-telemetry: $(CNI_BUILD_DIR)/azure-vnet-telemetry$(EXE_EXT)
acncli: $(ACNCLI_BUILD_DIR)/acncli$(EXE_EXT) acncli-archive

# Tool paths
CONTROLLER_GEN := $(TOOLS_BIN_DIR)/controller-gen
GOCOV := $(TOOLS_BIN_DIR)/gocov
GOCOV_XML := $(TOOLS_BIN_DIR)/gocov-xml
GO_JUNIT_REPORT := $(TOOLS_BIN_DIR)/go-junit-report
GOLANGCI_LINT := $(TOOLS_BIN_DIR)/golangci-lint

# Azure-NPM only supports Linux for now.
ifeq ($(GOOS),linux)
azure-cnms: $(CNMS_BUILD_DIR)/azure-cnms$(EXE_EXT) cnms-archive
azure-npm: $(NPM_BUILD_DIR)/azure-npm$(EXE_EXT) npm-archive
endif

ifeq ($(GOOS),linux)
all-binaries: azure-cnm-plugin azure-cni-plugin azure-cns azure-cnms azure-npm
else
all-binaries: azure-cnm-plugin azure-cni-plugin azure-cns
endif

ifeq ($(GOOS),linux)
all-images: azure-npm-image azure-cns-image
else
all-images:
	@echo "Nothing to build. Skip."
endif


# Clean all build artifacts.
.PHONY: clean
clean:
	rm -rf $(OUTPUT_DIR)
	rm -rf $(TOOLS_BIN_DIR)

# Build the Azure CNM plugin.
$(CNM_BUILD_DIR)/azure-vnet-plugin$(EXE_EXT): $(CNMFILES)
	CGO_ENABLED=0 go build -v -o $(CNM_BUILD_DIR)/azure-vnet-plugin$(EXE_EXT) -ldflags "-X main.version=$(VERSION)" -gcflags="-dwarflocationlists=true" $(CNM_DIR)/*.go

# Build the Azure CNI network plugin.
$(CNI_BUILD_DIR)/azure-vnet$(EXE_EXT): $(CNIFILES)
	CGO_ENABLED=0 go build -v -o $(CNI_BUILD_DIR)/azure-vnet$(EXE_EXT) -ldflags "-X main.version=$(VERSION)" -gcflags="-dwarflocationlists=true" $(CNI_NET_DIR)/*.go

# Build the Azure CNI IPAM plugin.
$(CNI_BUILD_DIR)/azure-vnet-ipam$(EXE_EXT): $(CNIFILES)
	CGO_ENABLED=0 go build -v -o $(CNI_BUILD_DIR)/azure-vnet-ipam$(EXE_EXT) -ldflags "-X main.version=$(VERSION)" -gcflags="-dwarflocationlists=true" $(CNI_IPAM_DIR)/*.go

# Build the Azure CNI IPAMV6 plugin.
$(CNI_BUILD_DIR)/azure-vnet-ipamv6$(EXE_EXT): $(CNIFILES)
	CGO_ENABLED=0 go build -v -o $(CNI_BUILD_DIR)/azure-vnet-ipamv6$(EXE_EXT) -ldflags "-X main.version=$(VERSION)" -gcflags="-dwarflocationlists=true" $(CNI_IPAMV6_DIR)/*.go

# Build the Azure CNI telemetry plugin.
$(CNI_BUILD_DIR)/azure-vnet-telemetry$(EXE_EXT): $(CNIFILES)
	CGO_ENABLED=0 go build -v -o $(CNI_BUILD_DIR)/azure-vnet-telemetry$(EXE_EXT) -ldflags "-X main.version=$(VERSION) -X $(ACN_PACKAGE_PATH)/telemetry.aiMetadata=$(CNI_AI_ID)" -gcflags="-dwarflocationlists=true" $(CNI_TELEMETRY_DIR)/*.go

# Build the Azure CLI network plugin.
$(ACNCLI_BUILD_DIR)/acncli$(EXE_EXT): $(CNIFILES)
	CGO_ENABLED=0 CGO_ENABLED=0 go build -v -o $(ACNCLI_BUILD_DIR)/acn$(EXE_EXT) -ldflags "-X main.version=$(VERSION)" -gcflags="-dwarflocationlists=true" $(ACNCLI_DIR)/*.go

# Build the Azure CNS Service.
$(CNS_BUILD_DIR)/azure-cns$(EXE_EXT): $(CNSFILES)
	CGO_ENABLED=0 go build -v -o $(CNS_BUILD_DIR)/azure-cns$(EXE_EXT) -ldflags "-X main.version=$(VERSION) -X $(cnsaipath)=$(CNS_AI_ID)" -gcflags="-dwarflocationlists=true" $(CNS_DIR)/*.go

# Build the Azure CNMS Service.
$(CNMS_BUILD_DIR)/azure-cnms$(EXE_EXT): $(CNMSFILES)
	CGO_ENABLED=0 go build -v -o $(CNMS_BUILD_DIR)/azure-cnms$(EXE_EXT) -ldflags "-X main.version=$(VERSION)" -gcflags="-dwarflocationlists=true" $(CNMS_DIR)/*.go

# Build the Azure NPM plugin.
$(NPM_BUILD_DIR)/azure-npm$(EXE_EXT): $(NPMFILES)
	CGO_ENABLED=0 go build -v -o $(NPM_BUILD_DIR)/azure-vnet-telemetry$(EXE_EXT) -ldflags "-X main.version=$(VERSION)" -gcflags="-dwarflocationlists=true" $(CNI_TELEMETRY_DIR)/*.go
	CGO_ENABLED=0 go build -v -o $(NPM_BUILD_DIR)/azure-npm$(EXE_EXT) -ldflags "-X main.version=$(VERSION) -X $(ACN_PACKAGE_PATH)/npm.aiMetadata=$(NPM_AI_ID)" -gcflags="-dwarflocationlists=true" $(NPM_DIR)/*.go

# Build all binaries in a container.
.PHONY: all-containerized
all-containerized:
	pwd && ls -l
	docker build -f Dockerfile.build -t $(BUILD_CONTAINER_IMAGE):$(VERSION) --no-cache .
	docker run --name $(BUILD_CONTAINER_NAME) \
		-v /usr/bin/docker:/usr/bin/docker \
		-v /var/run/docker.sock:/var/run/docker.sock \
		$(BUILD_CONTAINER_IMAGE):$(VERSION) \
		bash -c '\
			pwd && ls -l && \
			export GOOS=$(GOOS) && \
			export GOARCH=$(GOARCH) && \
			make all-binaries && \
			make all-images && \
		'
	docker cp $(BUILD_CONTAINER_NAME):$(BUILD_CONTAINER_REPO_PATH)/$(BUILD_DIR) $(OUTPUT_DIR)
	docker rm $(BUILD_CONTAINER_NAME)
	docker rmi $(BUILD_CONTAINER_IMAGE):$(VERSION)

# Make both linux and windows binaries
.PHONY: all-binaries-platforms
all-binaries-platforms:
	export GOOS=linux; make all-binaries
	export GOOS=windows; make all-binaries


.PHONY: tools
tools: acncli

.PHONY: tools-images
tools-images:
	docker build --no-cache -f ./tools/acncli/Dockerfile --build-arg VERSION=$(VERSION) -t $(AZURE_CNI_IMAGE):$(VERSION) .
	docker save $(AZURE_CNI_IMAGE):$(VERSION) | gzip -c > $(IMAGE_DIR)/$(CNI_IMAGE_ARCHIVE_NAME)

# Build the Azure CNM plugin image, installable with "docker plugin install".
.PHONY: azure-vnet-plugin-image
azure-vnet-plugin-image: azure-cnm-plugin
	# Build the plugin image, keeping any old image during build for cache, but remove it afterwards.
	docker images -q $(CNM_PLUGIN_ROOTFS):$(VERSION) > cid
	docker build --no-cache \
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
ifeq ($(GOOS),linux)
	mkdir -p $(IMAGE_DIR)
	docker build \
	--no-cache \
	-f npm/Dockerfile \
	-t $(AZURE_NPM_IMAGE):$(VERSION) \
	--build-arg NPM_BUILD_DIR=$(NPM_BUILD_DIR) \
	.
	docker save $(AZURE_NPM_IMAGE):$(VERSION) | gzip -c > $(IMAGE_DIR)/$(NPM_IMAGE_ARCHIVE_NAME)
endif

# Publish the Azure NPM image to a Docker registry
.PHONY: publish-azure-npm-image
publish-azure-npm-image:
	docker push $(AZURE_NPM_IMAGE):$(VERSION)

# Build the Azure CNMS image
.PHONY: azure-cnms-image
azure-cnms-image: azure-cnms
ifeq ($(GOOS),linux)
	docker build \
	--no-cache \
	-f cnms/Dockerfile \
	-t $(AZURE_CNMS_IMAGE):$(VERSION) \
	--build-arg CNMS_BUILD_DIR=$(CNMS_BUILD_DIR) \
	.
	docker save $(AZURE_CNMS_IMAGE):$(VERSION) | gzip -c > $(IMAGE_DIR)/$(CNMS_IMAGE_ARCHIVE_NAME)
endif

# Build the Azure vnet telemetry image
.PHONY: azure-vnet-telemetry-image
azure-vnet-telemetry-image: azure-vnet-telemetry
	docker build \
	-f cni/telemetry/Dockerfile \
	-t $(AZURE_VNET_TELEMETRY_IMAGE):$(VERSION) \
	--build-arg TELEMETRY_BUILD_DIR=$(NPM_BUILD_DIR) \
	--build-arg TELEMETRY_CONF_DIR=$(TELEMETRY_CONF_DIR) \
	.
	docker save $(AZURE_VNET_TELEMETRY_IMAGE):$(VERSION) | gzip -c > $(NPM_BUILD_DIR)/$(TELEMETRY_IMAGE_ARCHIVE_NAME)

# Publish the Azure vnet telemetry image to a Docker registry
.PHONY: publish-azure-vnet-telemetry-image
publish-azure-vnet-telemetry-image:
	docker push $(AZURE_VNET_TELEMETRY_IMAGE):$(VERSION)

# Build the Azure CNS image
.PHONY: azure-cns-image
azure-cns-image:
ifeq ($(GOOS),linux)
	mkdir -p $(IMAGE_DIR)
	docker build \
	--no-cache \
	-f cns/Dockerfile \
	-t $(AZURE_CNS_IMAGE):$(VERSION) \
	--build-arg VERSION=$(VERSION) \
	--build-arg CNS_AI_PATH=$(cnsaipath) \
	--build-arg CNS_AI_ID=$(CNS_AI_ID) \
	.
	docker save $(AZURE_CNS_IMAGE):$(VERSION) | gzip -c > $(IMAGE_DIR)/$(CNS_IMAGE_ARCHIVE_NAME)
endif

# Publish the Azure NPM image to a Docker registry
.PHONY: publish-azure-cns-image
publish-azure-cns-image:
	docker push $(AZURE_CNS_IMAGE):$(VERSION)

# Create a CNI archive for the target platform.
.PHONY: cni-archive
cni-archive:
	cp cni/azure-$(GOOS).conflist $(CNI_BUILD_DIR)/10-azure.conflist
	cp telemetry/azure-vnet-telemetry.config $(CNI_BUILD_DIR)/azure-vnet-telemetry.config
	chmod 0755 $(CNI_BUILD_DIR)/azure-vnet$(EXE_EXT) $(CNI_BUILD_DIR)/azure-vnet-ipam$(EXE_EXT) $(CNI_BUILD_DIR)/azure-vnet-ipamv6$(EXE_EXT) $(CNI_BUILD_DIR)/azure-vnet-telemetry$(EXE_EXT)
	cd $(CNI_BUILD_DIR) && $(ARCHIVE_CMD) $(CNI_ARCHIVE_NAME) azure-vnet$(EXE_EXT) azure-vnet-ipam$(EXE_EXT) azure-vnet-ipamv6$(EXE_EXT) azure-vnet-telemetry$(EXE_EXT) 10-azure.conflist azure-vnet-telemetry.config

	mkdir -p $(CNI_MULTITENANCY_BUILD_DIR)
	cp cni/azure-$(GOOS)-multitenancy.conflist $(CNI_MULTITENANCY_BUILD_DIR)/10-azure.conflist
	cp telemetry/azure-vnet-telemetry.config $(CNI_MULTITENANCY_BUILD_DIR)/azure-vnet-telemetry.config
	cp $(CNI_BUILD_DIR)/azure-vnet$(EXE_EXT) $(CNI_BUILD_DIR)/azure-vnet-ipam$(EXE_EXT) $(CNI_BUILD_DIR)/azure-vnet-telemetry$(EXE_EXT) $(CNI_MULTITENANCY_BUILD_DIR)
	chmod 0755 $(CNI_MULTITENANCY_BUILD_DIR)/azure-vnet$(EXE_EXT) $(CNI_MULTITENANCY_BUILD_DIR)/azure-vnet-ipam$(EXE_EXT)
	cd $(CNI_MULTITENANCY_BUILD_DIR) && $(ARCHIVE_CMD) $(CNI_MULTITENANCY_ARCHIVE_NAME) azure-vnet$(EXE_EXT) azure-vnet-ipam$(EXE_EXT) azure-vnet-telemetry$(EXE_EXT) 10-azure.conflist azure-vnet-telemetry.config

#baremetal mode is windows only (at least for now)
ifeq ($(GOOS),windows)
	mkdir -p $(CNI_BAREMETAL_BUILD_DIR)
	cp cni/azure-$(GOOS)-baremetal.conflist $(CNI_BAREMETAL_BUILD_DIR)/10-azure.conflist
	cp $(CNI_BUILD_DIR)/azure-vnet$(EXE_EXT) $(CNI_BAREMETAL_BUILD_DIR)
	chmod 0755 $(CNI_BAREMETAL_BUILD_DIR)/azure-vnet$(EXE_EXT)
	cd $(CNI_BAREMETAL_BUILD_DIR) && $(ARCHIVE_CMD) $(CNI_BAREMETAL_ARCHIVE_NAME) azure-vnet$(EXE_EXT) 10-azure.conflist
endif

#swift mode is linux only
ifeq ($(GOOS),linux)
	mkdir -p $(CNI_SWIFT_BUILD_DIR)
	cp cni/azure-$(GOOS)-swift.conflist $(CNI_SWIFT_BUILD_DIR)/10-azure.conflist
	cp telemetry/azure-vnet-telemetry.config $(CNI_SWIFT_BUILD_DIR)/azure-vnet-telemetry.config
	cp $(CNI_BUILD_DIR)/azure-vnet$(EXE_EXT) $(CNI_BUILD_DIR)/azure-vnet-ipam$(EXE_EXT) $(CNI_BUILD_DIR)/azure-vnet-telemetry$(EXE_EXT) $(CNI_SWIFT_BUILD_DIR)
	chmod 0755 $(CNI_SWIFT_BUILD_DIR)/azure-vnet$(EXE_EXT) $(CNI_SWIFT_BUILD_DIR)/azure-vnet-ipam$(EXE_EXT)
	cd $(CNI_SWIFT_BUILD_DIR) && $(ARCHIVE_CMD) $(CNI_SWIFT_ARCHIVE_NAME) azure-vnet$(EXE_EXT) azure-vnet-ipam$(EXE_EXT) azure-vnet-telemetry$(EXE_EXT) 10-azure.conflist azure-vnet-telemetry.config
endif

# Create a CNM archive for the target platform.
.PHONY: cnm-archive
cnm-archive:
	chmod 0755 $(CNM_BUILD_DIR)/azure-vnet-plugin$(EXE_EXT)
	cd $(CNM_BUILD_DIR) && $(ARCHIVE_CMD) $(CNM_ARCHIVE_NAME) azure-vnet-plugin$(EXE_EXT)

# Create a CNM archive for the target platform.
.PHONY: acncli-archive
acncli-archive:
ifeq ($(GOOS),linux)
	mkdir -p $(ACNCLI_BUILD_DIR)
	chmod 0755 $(ACNCLI_BUILD_DIR)/acn$(EXE_EXT)
	cd $(ACNCLI_BUILD_DIR) && $(ARCHIVE_CMD) $(ACNCLI_ARCHIVE_NAME) acn$(EXE_EXT)
endif


# Create a CNS archive for the target platform.
.PHONY: cns-archive
cns-archive:
	cp cns/configuration/cns_config.json $(CNS_BUILD_DIR)/cns_config.json
	chmod 0755 $(CNS_BUILD_DIR)/azure-cns$(EXE_EXT)
	cd $(CNS_BUILD_DIR) && $(ARCHIVE_CMD) $(CNS_ARCHIVE_NAME) azure-cns$(EXE_EXT) cns_config.json

# Create a CNMS archive for the target platform. Only Linux is supported for now.
.PHONY: cnms-archive
cnms-archive:
ifeq ($(GOOS),linux)
	chmod 0755 $(CNMS_BUILD_DIR)/azure-cnms$(EXE_EXT)
	cd $(CNMS_BUILD_DIR) && $(ARCHIVE_CMD) $(CNMS_ARCHIVE_NAME) azure-cnms$(EXE_EXT)
endif

# Create a NPM archive for the target platform. Only Linux is supported for now.
.PHONY: npm-archive
npm-archive:
ifeq ($(GOOS),linux)
	chmod 0755 $(NPM_BUILD_DIR)/azure-npm$(EXE_EXT)
	cd $(NPM_BUILD_DIR) && $(ARCHIVE_CMD) $(NPM_ARCHIVE_NAME) azure-npm$(EXE_EXT)
endif

.PHONY: release
release:
	./scripts/semver-release.sh


PRETTYGOTEST := $(shell command -v gotest 2> /dev/null)

LINT_PKG ?= .

lint: $(GOLANGCI_LINT) ## Fast lint vs default branch showing only new issues
	$(GOLANGCI_LINT) run --new-from-rev=master -v $(LINT_PKG)/...

lint-old: $(GOLANGCI_LINT) ## Fast lint including previous issues
	$(GOLANGCI_LINT) run -v $(LINT_PKG)/...

# run all tests
.PHONY: test-all
test-all:
	go test -tags "unit" -coverpkg=./... -v -race -covermode atomic -failfast -coverprofile=coverage.out ./...


# run all tests
.PHONY: test-integration
test-integration:
	go test -coverpkg=./... -v -race -covermode atomic -coverprofile=coverage.out -tags=integration ./test/integration...

.PHONY: test-cyclonus
test-cyclonus:
	cd test/cyclonus && bash ./test-cyclonus.sh
	cd ..

.PHONY: kind
kind:
	kind create cluster --config ./test/kind/kind.yaml

$(TOOLS_DIR)/go.mod:
	cd $(TOOLS_DIR); go mod init && go mod tidy

$(CONTROLLER_GEN): $(TOOLS_DIR)/go.mod
	cd $(TOOLS_DIR); go mod download; go build -tags=tools -o bin/controller-gen sigs.k8s.io/controller-tools/cmd/controller-gen

controller-gen: $(CONTROLLER_GEN) ## Build controller-gen

$(GOCOV): $(TOOLS_DIR)/go.mod
	cd $(TOOLS_DIR); go mod download; go build -tags=tools -o bin/gocov github.com/axw/gocov/gocov

gocov: $(GOCOV) ## Build gocov

$(GOCOV_XML): $(TOOLS_DIR)/go.mod
	cd $(TOOLS_DIR); go mod download; go build -tags=tools -o bin/gocov-xml github.com/AlekSi/gocov-xml

gocov-xml: $(GOCOV_XML) ## Build gocov-xml

$(GO_JUNIT_REPORT): $(TOOLS_DIR)/go.mod
	cd $(TOOLS_DIR); go mod download; go build -tags=tools -o bin/go-junit-report github.com/jstemmer/go-junit-report

go-junit-report: $(GO_JUNIT_REPORT) ## Build go-junit-report

$(GOLANGCI_LINT): $(TOOLS_DIR)/go.mod
	cd $(TOOLS_DIR); go mod download; go build -tags=tools -o bin/golangci-lint github.com/golangci/golangci-lint/cmd/golangci-lint

golangci-lint: $(GOLANGCI_LINT) ## Build golangci-lint

tools: gocov gocov-xml go-junit-report golangci-lint ## Build bins for build tools
