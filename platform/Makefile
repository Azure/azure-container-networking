REPO_ROOT = $(shell git rev-parse --show-toplevel)
MOCKGEN = go tool mockgen

.PHONY: generate

generate: ## Generate mock clients
	$(MOCKGEN) -source=$(REPO_ROOT)/platform/windows/adapter/network_adapter.go -package=mocks NetworkAdapter > windows/adapter/mocks/networkadapter_generated.go
