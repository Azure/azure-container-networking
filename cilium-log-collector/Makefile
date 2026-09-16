# This file should be included in the repo root Makefile

CILIUM_LOG_COLLECTOR_VERSION ?= $(notdir $(shell git describe --match "cilium-log-collector*" --tags --always))
CILIUM_LOG_COLLECTOR_DIR = $(REPO_ROOT)/cilium-log-collector
CILIUM_LOG_COLLECTOR_BUILD_DIR = $(BUILD_DIR)/cilium-log-collector
CILIUM_LOG_COLLECTOR_ARCHIVE_NAME = cilium-log-collector-$(GOOS)-$(GOARCH)-$(CILIUM_LOG_COLLECTOR_VERSION).$(ARCHIVE_EXT)

cilium-log-collector: cilium-log-collector-binary cilium-log-collector-archive

cilium-log-collector-version: ## prints the cilium-log-collector version
	@echo $(CILIUM_LOG_COLLECTOR_VERSION)


# Build the cilium-log-collector plugin "so" file
# CGO_ENABLED=1 + -buildmode=c-shared here requires a working C compiler and
# a full target-arch sysroot (libc headers/libs). AzureLinux/CBL-Mariner (the
# build agent's distro) ships an aarch64 cross-*compiler*
# (gcc-aarch64-linux-gnu) but no aarch64 cross-sysroot package at all (unlike
# Debian's libc6-dev-arm64-cross), so CGO_ENABLED=1 cross-compilation to
# GOARCH=arm64 on an amd64 host cannot work here today: it fails to find
# <errno.h>/<stdint.h>/etc. for the target. This .so has never been part of
# the portable release archives (0 cilium-log-collector assets across all
# GitHub releases checked) - its only consumer is the cilium-log-collector
# container image, built separately via Docker's own native/QEMU multi-arch
# buildx flow (which compiles "natively" inside an arch-matched container,
# not via a host cross-toolchain, so it isn't affected by this gap). So,
# rather than fail (and abort) the whole all-binaries-platforms release
# build over an arch/binary combination it never actually needed, skip this
# cross-compile combination specifically.
# Uses its own "uname -m" (rather than the root Makefile's UNAME_M) since
# this file is included at the very top of the root Makefile, before
# UNAME_M is defined there - ifeq is evaluated at parse time, so relying on
# that variable here would always see it unset.
CILIUM_LOG_COLLECTOR_UNAME_M := $(shell uname -m)
CILIUM_LOG_COLLECTOR_NATIVE_GOARCH := $(if $(filter x86_64,$(CILIUM_LOG_COLLECTOR_UNAME_M)),amd64,$(if $(filter aarch64 arm64,$(CILIUM_LOG_COLLECTOR_UNAME_M)),arm64,$(CILIUM_LOG_COLLECTOR_UNAME_M)))
# Same parse-time-vs-included-at-top-of-file problem applies to GOARCH itself:
# the root Makefile only defaults GOARCH (line ~21, "GOARCH ?= $(shell go env
# GOARCH)") after this file is already included (line 1), so a plain
# "make cilium-log-collector" with no explicit GOARCH= on the command line
# would see GOARCH as empty here and always take the "skip" branch below, even
# on a native build. Command-line-supplied GOARCH (e.g. from
# all-binaries-platforms' recursive "make ... GOARCH=$$goarch") IS visible
# here already, so only fall back to "go env GOARCH" when it's unset.
CILIUM_LOG_COLLECTOR_GOARCH := $(if $(GOARCH),$(GOARCH),$(shell go env GOARCH))
cilium-log-collector-binary:
	$(MKDIR) $(CILIUM_LOG_COLLECTOR_BUILD_DIR)
ifeq ($(CILIUM_LOG_COLLECTOR_GOARCH),$(CILIUM_LOG_COLLECTOR_NATIVE_GOARCH))
	# This plugin is loaded into cilium's own controlled runtime and, like
	# cni.sh/cns.sh, is expected to always use Microsoft's FIPS-capable OpenSSL
	# crypto backend (systemcrypto). On MS-Go 1.26, systemcrypto is already the
	# ambient default, so we don't need to (and, on MS-Go 1.27+, must not - the
	# GOEXPERIMENT=systemcrypto/nosystemcrypto values were removed and now
	# error if set explicitly) pass GOEXPERIMENT=systemcrypto ourselves. We do
	# still need to strip any inherited MS_GO_NOSYSTEMCRYPTO=1 (set by
	# all-binaries-platforms for portable Linux release archives), since that
	# variable takes precedence over the ambient systemcrypto default and
	# would otherwise silently downgrade this plugin to non-FIPS crypto.
	cd $(CILIUM_LOG_COLLECTOR_DIR) && env -u MS_GO_NOSYSTEMCRYPTO CGO_ENABLED=1 go build -buildmode=c-shared -a -o $(CILIUM_LOG_COLLECTOR_BUILD_DIR)/out_azure_app_insights.so -trimpath -ldflags "-X main.version=$(CILIUM_LOG_COLLECTOR_VERSION)" -gcflags="-dwarflocationlists=true" .
else
	@echo "Skipping cilium-log-collector-binary for GOARCH=$(CILIUM_LOG_COLLECTOR_GOARCH) on $(CILIUM_LOG_COLLECTOR_UNAME_M) host: CGO_ENABLED=1 cross-compilation needs a target sysroot AzureLinux doesn't provide, and this .so is not part of the portable release archives (only built into the cilium-log-collector container image via Docker's native/QEMU multi-arch buildx)."
endif


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
# Same parse-time-vs-included-at-top-of-file gap as CILIUM_LOG_COLLECTOR_GOARCH
# above applies to GOOS too: the root Makefile only defaults GOOS (line ~20,
# "GOOS ?= $(shell go env GOOS)") after this file is already included, so a
# plain "make cilium-log-collector" with no explicit GOOS= would see GOOS as
# empty here and always skip the archive step, even on Linux.
CILIUM_LOG_COLLECTOR_GOOS := $(if $(GOOS),$(GOOS),$(shell go env GOOS))
.PHONY: cilium-log-collector-archive
cilium-log-collector-archive: cilium-log-collector-binary
ifeq ($(CILIUM_LOG_COLLECTOR_GOOS),linux)
ifeq ($(CILIUM_LOG_COLLECTOR_GOARCH),$(CILIUM_LOG_COLLECTOR_NATIVE_GOARCH))
	$(MKDIR) $(CILIUM_LOG_COLLECTOR_BUILD_DIR)
	cd $(CILIUM_LOG_COLLECTOR_BUILD_DIR) && $(ARCHIVE_CMD) $(CILIUM_LOG_COLLECTOR_ARCHIVE_NAME) out_azure_app_insights.so
else
	@echo "Skipping cilium-log-collector-archive for GOARCH=$(CILIUM_LOG_COLLECTOR_GOARCH) on $(CILIUM_LOG_COLLECTOR_UNAME_M) host: no out_azure_app_insights.so was built (see cilium-log-collector-binary)."
endif
endif

test-cilium-log-collector: ## run the unit test for cilium-log-collector
	cd $(CILIUM_LOG_COLLECTOR_DIR) && go test -race -covermode atomic -coverprofile=../coverage-cilium-log-collector.out && go tool cover -func=../coverage-cilium-log-collector.out
