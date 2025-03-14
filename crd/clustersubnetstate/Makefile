.DEFAULT_GOAL = all

REPO_ROOT = $(shell git rev-parse --show-toplevel)
CONTROLLER_GEN = go tool sigs.k8s.io/controller-tools/cmd/controller-gen

all: generate manifests

generate: 
	$(CONTROLLER_GEN) object paths="./..."

.PHONY: manifests
manifests: 
	mkdir -p manifests
	$(CONTROLLER_GEN) crd paths="./..." output:crd:artifacts:config=manifests/

$(CONTROLLER_GEN):
	@make -C $(REPO_ROOT) $(CONTROLLER_GEN)
