# Source images
export GO_IMG					?= mcr.microsoft.com/oss/go/microsoft/golang:1.26-azurelinux3.0
export MARINER_CORE_IMG			?= mcr.microsoft.com/azurelinux/base/core:3.0
export MARINER_DISTROLESS_IMG	?= mcr.microsoft.com/azurelinux/distroless/base:3.0
export WIN_HPC_IMG				?= mcr.microsoft.com/oss/kubernetes/windows-host-process-containers-base-image:v1.0.0


# Print any variable: make -f build/images.mk print-GO_IMG
print-%:
	@echo $($*)

# Set GO_IMG tag: make -f build/images.mk set-GO_IMG NEW_TAG=<version>-azurelinux3.0
set-GO_IMG:
ifndef NEW_TAG
	$(error NEW_TAG is required — usage: make -f build/images.mk set-GO_IMG NEW_TAG=<version>-azurelinux3.0)
endif
ifeq ($(shell uname -s),Darwin)
	sed -i '' -E 's|(GO_IMG[[:space:]]*\?=[[:space:]]*.*golang:)[^ ]*|\1$(NEW_TAG)|' $(CURDIR)/build/images.mk
else
	sed -i -E 's|(GO_IMG[[:space:]]*\?=[[:space:]]*.*golang:)[^ ]*|\1$(NEW_TAG)|' $(CURDIR)/build/images.mk
endif

