REPO_ROOT = $(shell git rev-parse --show-toplevel)
TOOLS_DIR = $(REPO_ROOT)/build/tools
TOOLS_BIN_DIR = $(REPO_ROOT)/build/tools/bin
MOCKGEN = $(TOOLS_BIN_DIR)/mockgen

.PHONY: generate

generate: $(MOCKGEN) ## Generate mock clients
	$(MOCKGEN) -source=$(REPO_ROOT)/cns/cnsclient/apiclient.go -package=mockclients APIClient > cnsclient.go
	$(MOCKGEN) -source=$(REPO_ROOT)/vendor/sigs.k8s.io/controller-runtime/pkg/client/interfaces.go -package=mockclients Client > kubeclient.go

$(MOCKGEN):
	@make -C $(REPO_ROOT) $(MOCKGEN)
