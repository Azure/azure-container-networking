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

# Windows specific extensions
ifeq ($(GOOS),windows)
ARCHIVE_CMD = zip -9lq
ARCHIVE_EXT = zip
EXE_EXT = .exe
endif

# Build defaults.
GOOS ?= $(shell go env GOOS)
GOARCH ?= $(shell go env GOARCH)
GOOSES ?= "linux windows" # To override at the cli do: GOOSES="\"darwin bsd\""
GOARCHES ?= "amd64 arm64" # To override at the cli do: GOARCHES="\"ppc64 mips\""

# Build directories.
REPO_ROOT = $(shell git rev-parse --show-toplevel)
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
IMAGE_DIR  = $(OUTPUT_DIR)/images
CNM_BUILD_DIR = $(BUILD_DIR)/cnm
CNI_BUILD_DIR = $(BUILD_DIR)/cni
ACNCLI_BUILD_DIR = $(BUILD_DIR)/acncli
CNI_MULTITENANCY_BUILD_DIR = $(BUILD_DIR)/cni-multitenancy
CNI_SWIFT_BUILD_DIR = $(BUILD_DIR)/cni-swift
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
CNI_SWIFT_ARCHIVE_NAME = azure-vnet-cni-swift-$(GOOS)-$(GOARCH)-$(VERSION).$(ARCHIVE_EXT)
CNI_BAREMETAL_ARCHIVE_NAME = azure-vnet-cni-baremetal-$(GOOS)-$(GOARCH)-$(VERSION).$(ARCHIVE_EXT)
CNS_ARCHIVE_NAME = azure-cns-$(GOOS)-$(GOARCH)-$(VERSION).$(ARCHIVE_EXT)
NPM_ARCHIVE_NAME = azure-npm-$(GOOS)-$(GOARCH)-$(VERSION).$(ARCHIVE_EXT)
NPM_IMAGE_INFO_FILE = azure-npm-$(VERSION).txt
CNI_IMAGE_ARCHIVE_NAME = azure-cni-manager-$(GOOS)-$(GOARCH)-$(VERSION).$(ARCHIVE_EXT)
CNS_IMAGE_INFO_FILE = azure-cns-$(VERSION).txt

# Docker libnetwork (CNM) plugin v2 image parameters.
CNM_PLUGIN_IMAGE ?= microsoft/azure-vnet-plugin
CNM_PLUGIN_ROOTFS = azure-vnet-plugin-rootfs

IMAGE_REGISTRY ?= acnpublic.azurecr.io

# Azure network policy manager parameters.
AZURE_NPM_IMAGE ?= $(IMAGE_REGISTRY)/azure-npm

# Azure CNI installer parameters
AZURE_CNI_IMAGE = $(IMAGE_REGISTRY)/azure-cni-manager

# Azure container networking service image paramters.
AZURE_CNS_IMAGE = $(IMAGE_REGISTRY)/azure-cns

IMAGE_PLATFORM_ARCHES ?= linux/amd64,linux/arm64
IMAGE_ACTION ?= push

VERSION ?= $(shell git describe --tags --always --dirty)

# Default target
.PHONY: all-binaries-platforms
all-binaries-platforms: ## Make all platform binaries
	@for goos in "$(GOOSES)"; do \
		for goarch in "$(GOARCHES)"; do \
			make all-binaries GOOS=$$goos GOARCH=$$goarch; \
		done \
	done

# OS specific binaries/images
ifeq ($(GOOS),linux)
all-binaries: azure-cnm-plugin azure-cni-plugin azure-cns azure-npm
all-images: azure-npm-image azure-cns-image
else
all-binaries: azure-cnm-plugin azure-cni-plugin azure-cns
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

# Clean all build artifacts.
.PHONY: clean
clean:
	$(RMDIR) $(OUTPUT_DIR)
	$(RMDIR) $(TOOLS_BIN_DIR)

########################### Binaries ################################

# Build the Azure CNM binary.
.PHONY: cnm-binary
cnm-binary:
	cd $(CNM_DIR) && CGO_ENABLED=0 go build -v -o $(CNM_BUILD_DIR)/azure-vnet-plugin$(EXE_EXT) -ldflags "-X main.version=$(VERSION)" -gcflags="-dwarflocationlists=true"

# Build the Azure CNI network binary.
.PHONY: azure-vnet-binary
azure-vnet-binary:
	cd $(CNI_NET_DIR) && CGO_ENABLED=0 go build -v -o $(CNI_BUILD_DIR)/azure-vnet$(EXE_EXT) -ldflags "-X main.version=$(VERSION)" -gcflags="-dwarflocationlists=true"

# Build the Azure CNI IPAM binary.
.PHONY: azure-vnet-ipam-binary
azure-vnet-ipam-binary:
	cd $(CNI_IPAM_DIR) && CGO_ENABLED=0 go build -v -o $(CNI_BUILD_DIR)/azure-vnet-ipam$(EXE_EXT) -ldflags "-X main.version=$(VERSION)" -gcflags="-dwarflocationlists=true"

# Build the Azure CNI IPAMV6 binary.
.PHONY: azure-vnet-ipamv6-binary
azure-vnet-ipamv6-binary:
	cd $(CNI_IPAMV6_DIR) && CGO_ENABLED=0 go build -v -o $(CNI_BUILD_DIR)/azure-vnet-ipamv6$(EXE_EXT) -ldflags "-X main.version=$(VERSION)" -gcflags="-dwarflocationlists=true"

# Build the Azure CNI telemetry binary.
.PHONY: azure-vnet-telemetry-binary
azure-vnet-telemetry-binary:
	cd $(CNI_TELEMETRY_DIR) && CGO_ENABLED=0 go build -v -o $(CNI_BUILD_DIR)/azure-vnet-telemetry$(EXE_EXT) -ldflags "-X main.version=$(VERSION) -X $(CNI_AI_PATH)=$(CNI_AI_ID)" -gcflags="-dwarflocationlists=true"

# Build the Azure CLI network binary.
.PHONY: acncli-binary
acncli-binary:
	cd $(ACNCLI_DIR) && CGO_ENABLED=0 go build -v -o $(ACNCLI_BUILD_DIR)/acn$(EXE_EXT) -ldflags "-X main.version=$(VERSION)" -gcflags="-dwarflocationlists=true"

# Build the Azure CNS binary.
.PHONY: azure-cns-binary
azure-cns-binary:
	cd $(CNS_DIR) && CGO_ENABLED=0 go build -v -o $(CNS_BUILD_DIR)/azure-cns$(EXE_EXT) -ldflags "-X main.version=$(VERSION) -X $(CNS_AI_PATH)=$(CNS_AI_ID)" -gcflags="-dwarflocationlists=true"

# Build the Azure NPM binary.
.PHONY: azure-npm-binary
azure-npm-binary:
	cd $(CNI_TELEMETRY_DIR) && CGO_ENABLED=0 go build -v -o $(NPM_BUILD_DIR)/azure-vnet-telemetry$(EXE_EXT) -ldflags "-X main.version=$(VERSION)" -gcflags="-dwarflocationlists=true"
	cd $(NPM_DIR) && CGO_ENABLED=0 go build -v -o $(NPM_BUILD_DIR)/azure-npm$(EXE_EXT) -ldflags "-X main.version=$(VERSION) -X $(NPM_AI_PATH)=$(NPM_AI_ID)" -gcflags="-dwarflocationlists=true"


########################### Container Images ########################

# Build the tools images
.PHONY: tools-images
tools-images:
	$(MKDIR) $(IMAGE_DIR)
	docker build --no-cache -f ./tools/acncli/Dockerfile --build-arg VERSION=$(VERSION) -t $(AZURE_CNI_IMAGE):$(VERSION) .
	docker save $(AZURE_CNI_IMAGE):$(VERSION) | gzip -c > $(IMAGE_DIR)/$(CNI_IMAGE_ARCHIVE_NAME)

# Build the Azure CNM plugin image, installable with "docker plugin install".
.PHONY: azure-cnm-plugin-image
azure-cnm-plugin-image: azure-cnm-plugin
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
	rm -rf $(OUTPUT_DIR)/$(CID)
	rm cid

# Build the Azure NPM image.
.PHONY: azure-npm-image
azure-npm-image:
ifeq ($(GOOS),linux)
	$(MKDIR) $(IMAGE_DIR)
	docker buildx create --use
	docker buildx build \
	--no-cache \
	-f npm/Dockerfile \
	-t $(AZURE_NPM_IMAGE):$(VERSION) \
	--build-arg VERSION=$(VERSION) \
	--build-arg NPM_AI_PATH=$(NPM_AI_PATH) \
	--build-arg NPM_AI_ID=$(NPM_AI_ID) \
	--platform=$(IMAGE_PLATFORM_ARCHES) \
	--$(IMAGE_ACTION) \
	.
	
	echo $(AZURE_NPM_IMAGE):$(VERSION) > $(IMAGE_DIR)/$(NPM_IMAGE_INFO_FILE)
endif

# Build the Azure CNS image
.PHONY: azure-cns-image
azure-cns-image:
ifeq ($(GOOS),linux)
	$(MKDIR) $(IMAGE_DIR)
	docker buildx create --use
	docker buildx build \
	--no-cache \
	-f cns/Dockerfile \
	-t $(AZURE_CNS_IMAGE):$(VERSION) \
	--build-arg VERSION=$(VERSION) \
	--build-arg CNS_AI_PATH=$(CNS_AI_PATH) \
	--build-arg CNS_AI_ID=$(CNS_AI_ID) \
	--platform=$(IMAGE_PLATFORM_ARCHES) \
	--$(IMAGE_ACTION) \
	.

	echo $(AZURE_CNS_IMAGE):$(VERSION) > $(IMAGE_DIR)/$(CNS_IMAGE_INFO_FILE)
endif

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
	cp telemetry/azure-vnet-telemetry.config $(CNI_MULTITENANCY_BUILD_DIR)/azure-vnet-telemetry.config
	cp $(CNI_BUILD_DIR)/azure-vnet$(EXE_EXT) $(CNI_BUILD_DIR)/azure-vnet-ipam$(EXE_EXT) $(CNI_BUILD_DIR)/azure-vnet-telemetry$(EXE_EXT) $(CNI_MULTITENANCY_BUILD_DIR)
	cd $(CNI_MULTITENANCY_BUILD_DIR) && $(ARCHIVE_CMD) $(CNI_MULTITENANCY_ARCHIVE_NAME) azure-vnet$(EXE_EXT) azure-vnet-ipam$(EXE_EXT) azure-vnet-telemetry$(EXE_EXT) 10-azure.conflist azure-vnet-telemetry.config

#baremetal mode is windows only (at least for now)
ifeq ($(GOOS),windows)
	$(MKDIR) $(CNI_BAREMETAL_BUILD_DIR)
	cp cni/azure-$(GOOS)-baremetal.conflist $(CNI_BAREMETAL_BUILD_DIR)/10-azure.conflist
	cp $(CNI_BUILD_DIR)/azure-vnet$(EXE_EXT) $(CNI_BAREMETAL_BUILD_DIR)
	cd $(CNI_BAREMETAL_BUILD_DIR) && $(ARCHIVE_CMD) $(CNI_BAREMETAL_ARCHIVE_NAME) azure-vnet$(EXE_EXT) 10-azure.conflist
endif

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

############################ Linting ################################

PRETTYGOTEST := $(shell command -v gotest 2> /dev/null)

LINT_PKG ?= .

lint: $(GOLANGCI_LINT) ## Fast lint vs default branch showing only new issues
	$(GOLANGCI_LINT) run --new-from-rev master --timeout 10m -v $(LINT_PKG)/...

lint-old: $(GOLANGCI_LINT) ## Fast lint including previous issues
	$(GOLANGCI_LINT) run -v $(LINT_PKG)/...

FMT_PKG ?= cni cns npm

fmt format: $(GOFUMPT) ## run gofumpt on $FMT_PKG (default "cni cns npm")
	$(GOFUMPT) -s -w $(FMT_PKG)

COVER_PKG ?= .

# run all tests
# COVER_FILTER omits folders with all files tagged with one of 'unit', '!ignore_uncovered', or '!ignore_autogenerated'
.PHONY: test-all
test-all:
	@$(eval COVER_FILTER=`go list --tags ignore_uncovered,ignore_autogenerated $(COVER_PKG)/... | tr '\n' ','`)
	@echo Test coverpkg: $(COVER_FILTER)
	go test -tags "unit" -coverpkg=$(COVER_FILTER) -v -race -covermode atomic -failfast -coverprofile=coverage.out $(COVER_PKG)/...

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

version: ## prints the version
	@echo $(VERSION)

######################## Build tools ################################
.PHONY: tools
tools: acncli

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

$(GOFUMPT): $(TOOLS_DIR)/go.mod
	cd $(TOOLS_DIR); go mod download; go build -tags=tools -o bin/gofumpt mvdan.cc/gofumpt

gofmt gofumpt: $(GOFUMPT) ## Build gofumpt

$(GOLANGCI_LINT): $(TOOLS_DIR)/go.mod
	cd $(TOOLS_DIR); go mod download; go build -tags=tools -o bin/golangci-lint github.com/golangci/golangci-lint/cmd/golangci-lint

golangci-lint: $(GOLANGCI_LINT) ## Build golangci-lint

$(GO_JUNIT_REPORT): $(TOOLS_DIR)/go.mod
	cd $(TOOLS_DIR); go mod download; go build -tags=tools -o bin/go-junit-report github.com/jstemmer/go-junit-report

go-junit-report: $(GO_JUNIT_REPORT) ## Build go-junit-report

$(MOCKGEN): $(TOOLS_DIR)/go.mod
	cd $(TOOLS_DIR); go mod download; go build -tags=tools -o bin/mockgen github.com/golang/mock/mockgen

mockgen: $(MOCKGEN) ## Build mockgen

tools: gocov gocov-xml go-junit-report golangci-lint gofmt ## Build bins for build tools
