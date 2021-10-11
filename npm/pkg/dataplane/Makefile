REPO_ROOT = $(shell git rev-parse --show-toplevel)
TOOLS_DIR = $(REPO_ROOT)/build/tools
TOOLS_BIN_DIR = $(REPO_ROOT)/build/tools/bin
MOCKGEN = $(TOOLS_BIN_DIR)/mockgen

.PHONY: generate

generate: $(MOCKGEN) ## Generate mock clients
	$(MOCKGEN) -source=$(REPO_ROOT)/npm/pkg/dataplane/types.go -copyright_file=$(REPO_ROOT)/npm/pkg/dataplane/ignore_headers.txt -package=mocks > mocks/genericdataplane_generated.go

$(MOCKGEN):
	@make -C $(REPO_ROOT) $(MOCKGEN)
