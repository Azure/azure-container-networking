REPO_ROOT = $(shell git rev-parse --show-toplevel)
TOOLS_BIN_DIR = $(REPO_ROOT)/build/tools/bin
MOCKGEN = $(TOOLS_BIN_DIR)/mockgen

.PHONY: generate

generate: $(MOCKGEN) ## Generate mock clients
	$(MOCKGEN) -source=$(REPO_ROOT)/platform/windows/adapter/network_adapter.go -package=mocks NetworkAdapter > windows/adapter/mocks/networkadapter_generated.go

$(MOCKGEN):
	@make -C $(REPO_ROOT) $(MOCKGEN)
