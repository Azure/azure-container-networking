REPO_ROOT = $(shell git rev-parse --show-toplevel)
MOCKGEN = go tool -modfile=$(REPO_ROOT)/tools.go.mod mockgen

.PHONY: generate

generate: ## Generate mock clients
	$(MOCKGEN) -source=$(REPO_ROOT)/npm/pkg/dataplane/types.go -copyright_file=$(REPO_ROOT)/npm/pkg/dataplane/ignore_headers.txt -package=mocks > mocks/genericdataplane_generated.go
