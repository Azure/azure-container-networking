REPO_ROOT = $(shell git rev-parse --show-toplevel)
TOOLS_DIR = $(REPO_ROOT)/build/tools
TOOLS_BIN_DIR = $(REPO_ROOT)/build/tools/bin
CONTROLLER_GEN = $(TOOLS_BIN_DIR)/controller-gen

.PHONY: generate manifests

generate: $(CONTROLLER_GEN)
	$(CONTROLLER_GEN) object:headerFile="boilerplate.go.txt" paths="./..."

manifests: $(CONTROLLER_GEN)
	mkdir -p manifests
	$(CONTROLLER_GEN) crd:trivialVersions=true paths="./..." output:crd:artifacts:config=manifests/

$(CONTROLLER_GEN):
	@make -C $(REPO_ROOT) $(CONTROLLER_GEN)
