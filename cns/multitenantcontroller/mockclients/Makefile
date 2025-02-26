REPO_ROOT = $(shell git rev-parse --show-toplevel)
MOCKGEN = go tool mockgen

generate: ## Generate mock clients
	@go mod vendor
	$(MOCKGEN) -source=$(REPO_ROOT)/cns/multitenantcontroller/multitenantoperator/multitenantcrdreconciler.go -package=mockclients cnsRESTservice > cnsrestservice_generated.go
	$(MOCKGEN) -source=$(REPO_ROOT)/vendor/sigs.k8s.io/controller-runtime/pkg/client/interfaces.go -package=mockclients Client,SubResourceWriter > kubeclient_generated.go
	@sed -i s,$(REPO_ROOT)/,,g cnsrestservice_generated.go kubeclient_generated.go
	@rm -rf $(REPO_ROOT)/vendor
