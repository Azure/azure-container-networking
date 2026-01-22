# This file should be included in the repo root Makefile

CILIUM_LOG_COLLECTOR_VERSION ?= $(notdir $(shell git describe --match "cilium-log-collector*" --tags --always))
CILIUM_LOG_COLLECTOR_DIR = $(REPO_ROOT)/cilium-log-collector
CILIUM_LOG_COLLECTOR_BUILD_DIR = $(BUILD_DIR)/cilium-log-collector
CILIUM_LOG_COLLECTOR_ARCHIVE_NAME = cilium-log-collector-$(GOOS)-$(GOARCH)-$(CILIUM_LOG_COLLECTOR_VERSION).$(ARCHIVE_EXT)

cilium-log-collector: cilium-log-collector-binary cilium-log-collector-archive

cilium-log-collector-version: ## prints the cilium-log-collector version
	@echo $(CILIUM_LOG_COLLECTOR_VERSION)


# Build the cilium-log-collector plugin "so" file
cilium-log-collector-binary:
	$(MKDIR) $(CILIUM_LOG_COLLECTOR_BUILD_DIR)
	cd $(CILIUM_LOG_COLLECTOR_DIR) && go build -buildmode=c-shared -a -o $(CILIUM_LOG_COLLECTOR_BUILD_DIR)/out_azure_app_insights.so -trimpath -ldflags "-X main.version=$(CILIUM_LOG_COLLECTOR_VERSION)" -gcflags="-dwarflocationlists=true" .


CILIUM_LOG_COLLECTOR_IMAGE = cilium-log-collector
CILIUM_LOG_COLLECTOR_PLATFORM_TAG	?= $(subst /,-,$(PLATFORM))-$(CILIUM_LOG_COLLECTOR_VERSION)


cilium-log-collector-image-name: # util target to print the cilium-log-collector image name.
	@echo $(CILIUM_LOG_COLLECTOR_IMAGE)

cilium-log-collector-image-name-and-tag: # util target to print the cilium-log-collector image name and tag.
	@echo $(IMAGE_REGISTRY)/$(CILIUM_LOG_COLLECTOR_IMAGE):$(CILIUM_LOG_COLLECTOR_PLATFORM_TAG)

cilium-log-collector-image: ## build cilium-log-collector container image.
	$(MAKE) container \
		DOCKERFILE=cilium-log-collector/Dockerfile \
		IMAGE=$(CILIUM_LOG_COLLECTOR_IMAGE) \
		PLATFORM=$(PLATFORM) \
		TAG=$(CILIUM_LOG_COLLECTOR_PLATFORM_TAG) \
		TARGET=$(OS) \
		OS=$(OS) \
		ARCH=$(ARCH) \

cilium-log-collector-image-push: ## push cilium-log-collector container image.
	$(MAKE) container-push \
		IMAGE=$(CILIUM_LOG_COLLECTOR_IMAGE) \
		TAG=$(CILIUM_LOG_COLLECTOR_PLATFORM_TAG)

cilium-log-collector-image-pull: ## pull cilium-log-collector container image.
	$(MAKE) container-pull \
		IMAGE=$(CILIUM_LOG_COLLECTOR_IMAGE) \
		TAG=$(CILIUM_LOG_COLLECTOR_PLATFORM_TAG)

cilium-log-collector-manifest-build: ## build cilium-log-collector multiplat container manifest.
	$(MAKE) manifest-build \
		PLATFORMS="$(PLATFORMS)" \
		IMAGE=$(CILIUM_LOG_COLLECTOR_IMAGE) \
		TAG=$(CILIUM_LOG_COLLECTOR_VERSION)

cilium-log-collector-manifest-push: ## push cilium-log-collector multiplat container manifest
	$(MAKE) manifest-push \
		IMAGE=$(CILIUM_LOG_COLLECTOR_IMAGE) \
		TAG=$(CILIUM_LOG_COLLECTOR_VERSION)

cilium-log-collector-skopeo-archive: ## export tar archive of cilium-log-collector multiplat container manifest.
	$(MAKE) manifest-skopeo-archive \
		IMAGE=$(CILIUM_LOG_COLLECTOR_IMAGE) \
		TAG=$(CILIUM_LOG_COLLECTOR_VERSION)

# Create a cilium-log-collector archive for the target platform.
.PHONY: cilium-log-collector-archive
cilium-log-collector-archive: cilium-log-collector-binary
ifeq ($(GOOS),linux)
	$(MKDIR) $(CILIUM_LOG_COLLECTOR_BUILD_DIR)
	cd $(CILIUM_LOG_COLLECTOR_BUILD_DIR) && $(ARCHIVE_CMD) $(CILIUM_LOG_COLLECTOR_ARCHIVE_NAME) out_azure_app_insights.so
endif

test-cilium-log-collector: ## run the unit test for cilium-log-collector
	cd $(CILIUM_LOG_COLLECTOR_DIR) && go test -race -covermode atomic -coverprofile=../coverage-cilium-log-collector.out && go tool cover -func=../coverage-cilium-log-collector.out
