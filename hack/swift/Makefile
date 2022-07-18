.DEFAULT_GOAL: help

# construct containerized azcli command
KUBECFG = $(HOME)/.kube
SSH     = $(HOME)/.ssh
AZCFG   = $(HOME)/.azure
AZIMG   = mcr.microsoft.com/azure-cli
AZCLI   = docker run -v $(AZCFG):/root/.azure -v $(KUBECFG):/root/.kube -v $(SSH):/root/.ssh $(AZIMG) az

# overrideable variables
USER       ?= $(whoami)
CLUSTER    ?= $(USER)-$(REGION)
GROUP      ?= $(CLUSTER)
REGION     ?= centraluseuap
SUB        ?= $(AZURE_SUBSCRIPTION)
VNET       ?= $(CLUSTER)


##@ Help

help:  ## Display this help
	@awk 'BEGIN {FS = ":.*##"; printf "\nUsage:\n  make \033[36m<target>\033[0m\n"} /^[a-zA-Z_-]+:.*?##/ { printf "  \033[36m%-15s\033[0m %s\n", $$1, $$2 } /^##@/ { printf "\n\033[1m%s\033[0m\n", substr($$0, 5) } ' $(MAKEFILE_LIST)

##@ Utilities

azlogin:
	@$(AZCLI) login

azcfg:
	@$(AZCLI) account set -s $(SUB)
	@$(AZCLI) group create --location $(REGION) --name $(GROUP)
	@$(AZCLI) extension add --name aks-preview --yes

set-kubeconf: ## Adds the kubeconf for $CLUSTER
	$(AZCLI) aks get-credentials -n $(CLUSTER) -g $(GROUP)

unset-kubeconf: ## Deletes the kubeconf for $CLUSTER
	@kubectl config unset current-context
	@kubectl config delete-cluster $(CLUSTER)
	@kubectl config delete-context $(CLUSTER)
	@kubectl config delete-user clusterUser_$(CLUSTER)_$(CLUSTER)

shell: ## print $AZCLI so it can be used outside of make
	@echo $(AZCLI)


##@ SWIFT Infra
vars: ## Show the env vars configured for the swift command
	@echo SUB=$(SUB)
	@echo GROUP=$(GROUP)
	@echo REGION=$(REGION)
	@echo VNET=$(VNET)
	@echo CLUSTER=$(CLUSTER)

swift-net-up: azcfg ## Create required swift vnet/subnets
	$(AZCLI) network vnet create -g $(GROUP) -l $(REGION) --name $(VNET) --address-prefixes 10.0.0.0/8 -o none
	$(AZCLI) network vnet subnet create -g $(GROUP) --vnet-name $(VNET) --name nodenet --address-prefixes 10.240.0.0/16 -o none
	$(AZCLI) network vnet subnet create -g $(GROUP) --vnet-name $(VNET) --name podnet --address-prefixes 10.241.0.0/16 -o none

rg-down: azcfg ## Delete the $GROUP in $SUB/$REGION
	$(AZCLI) group delete -g $(GROUP) --yes

##@ AKS Clusters

swift-up: azcfg swift-net-up ## Brings up a swift cluster $name in $SUB/$REGION
	$(AZCLI) aks create -n $(CLUSTER) -g $(GROUP) -l $(REGION) \
		--max-pods 250 \
		--node-count 3 \
		--network-plugin azure \
		--vnet-subnet-id /subscriptions/$(SUB)/resourceGroups/$(GROUP)/providers/Microsoft.Network/virtualNetworks/$(VNET)/subnets/nodenet \
		--pod-subnet-id /subscriptions/$(SUB)/resourceGroups/$(GROUP)/providers/Microsoft.Network/virtualNetworks/$(VNET)/subnets/podnet \
		--yes
	@$(MAKE) set-kubeconf

byocni-up: azcfg swift-net-up ## Brings up a swift cluster $name in $SUB/$REGION
	$(AZCLI) aks create -n $(CLUSTER) -g $(GROUP) -l $(REGION) \
		--max-pods 250 \
		--node-count 3 \
		--network-plugin none \
		--vnet-subnet-id /subscriptions/$(SUB)/resourceGroups/$(GROUP)/providers/Microsoft.Network/virtualNetworks/$(VNET)/subnets/nodenet \
		--pod-subnet-id /subscriptions/$(SUB)/resourceGroups/$(GROUP)/providers/Microsoft.Network/virtualNetworks/$(VNET)/subnets/podnet \
		--yes
	@$(MAKE) set-kubeconf

down: azcfg ## Deletes the swift resources $SUB/$REGION
	$(AZCLI) aks delete -g $(GROUP) -n $(CLUSTER) --yes
	@$(MAKE) unset-kubeconf
	@$(MAKE) swift-rg-down
