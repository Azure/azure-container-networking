.DEFAULT_GOAL: help

# construct containerized azcli command
KUBECFG = $(HOME)/.kube
SSH     = $(HOME)/.ssh
AZCFG   = $(HOME)/.azure
AZIMG   = mcr.microsoft.com/azure-cli
AZCLI   ?= docker run --rm -v $(AZCFG):/root/.azure -v $(KUBECFG):/root/.kube -v $(SSH):/root/.ssh $(AZIMG) az

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
	@$(AZCLI) account set -s $(SUB)

azcfg:
	@$(AZCLI) extension add --name aks-preview --yes
	@$(AZCLI) extension update --name aks-preview 

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

rg-up: ## Create resource group $GROUP in $SUB/$REGION
	@$(AZCLI) group create --location $(REGION) --name $(GROUP)

rg-down: ## Delete the $GROUP in $SUB/$REGION
	$(AZCLI) group delete -g $(GROUP) --yes

net-up: ## Create required swift vnet/subnets
	$(AZCLI) network vnet create -g $(GROUP) -l $(REGION) --name $(VNET) --address-prefixes 10.0.0.0/8 -o none
	$(AZCLI) network vnet subnet create -g $(GROUP) --vnet-name $(VNET) --name nodenet --address-prefixes 10.240.0.0/16 -o none
	$(AZCLI) network vnet subnet create -g $(GROUP) --vnet-name $(VNET) --name podnet --address-prefixes 10.241.0.0/16 -o none

overlay-net-up:
	$(AZCLI) network vnet create -g $(GROUP) -l $(REGION) --name $(VNET) --address-prefixes 10.0.0.0/8 -o none
	$(AZCLI) network vnet subnet create -g $(GROUP) --vnet-name $(VNET) --name nodenet --address-prefix 10.10.0.0/16 -o none


##@ AKS Clusters

byocni-up: swift-byocni-up ## Alias to swift-byocni-up
cilium-up: swift-cilium-up ## Alias to swift-cilium-up
up: swift-up ## Alias to swift-up

overlay-up: rg-up overlay-net-up ## Brings up an Overlay AzCNI cluster
	$(AZCLI) aks create -n $(CLUSTER) -g $(GROUP) -l $(REGION) \
		--node-count 3 \
		--network-plugin azure \
		--network-plugin-mode overlay \
		--pod-cidr 192.168.0.0/16 \
		--vnet-subnet-id /subscriptions/$(SUB)/resourceGroups/$(GROUP)/providers/Microsoft.Network/virtualNetworks/$(VNET)/subnets/nodenet \
		--generate-ssh-keys \
		--yes
	@$(MAKE) set-kubeconf

swift-byocni-up: rg-up net-up ## Bring up a SWIFT BYO CNI cluster
	$(AZCLI) aks create -n $(CLUSTER) -g $(GROUP) -l $(REGION) \
		--node-count 3 \
		--network-plugin none \
		--vnet-subnet-id /subscriptions/$(SUB)/resourceGroups/$(GROUP)/providers/Microsoft.Network/virtualNetworks/$(VNET)/subnets/nodenet \
		--pod-subnet-id /subscriptions/$(SUB)/resourceGroups/$(GROUP)/providers/Microsoft.Network/virtualNetworks/$(VNET)/subnets/podnet \
		--generate-ssh-keys \
		--yes
	@$(MAKE) set-kubeconf

swift-cilium-up: rg-up net-up ## Bring up a SWIFT Cilium cluster
	$(AZCLI) aks create -n $(CLUSTER) -g $(GROUP) -l $(REGION) \
		--node-count 3 \
		--network-plugin azure \
		--enable-cilium-dataplane \
		--aks-custom-headers AKSHTTPCustomFeatures=Microsoft.ContainerService/CiliumDataplanePreview \
		--vnet-subnet-id /subscriptions/$(SUB)/resourceGroups/$(GROUP)/providers/Microsoft.Network/virtualNetworks/$(VNET)/subnets/nodenet \
		--pod-subnet-id /subscriptions/$(SUB)/resourceGroups/$(GROUP)/providers/Microsoft.Network/virtualNetworks/$(VNET)/subnets/podnet \
		--generate-ssh-keys \
		--yes
	@$(MAKE) set-kubeconf

swift-up: rg-up net-up ## Bring up a SWIFT AzCNI cluster
	$(AZCLI) aks create -n $(CLUSTER) -g $(GROUP) -l $(REGION) \
		--node-count 3 \
		--network-plugin azure \
		--vnet-subnet-id /subscriptions/$(SUB)/resourceGroups/$(GROUP)/providers/Microsoft.Network/virtualNetworks/$(VNET)/subnets/nodenet \
		--pod-subnet-id /subscriptions/$(SUB)/resourceGroups/$(GROUP)/providers/Microsoft.Network/virtualNetworks/$(VNET)/subnets/podnet \
		--generate-ssh-keys \
		--yes
	@$(MAKE) set-kubeconf

down: ## Delete the cluster
	$(AZCLI) aks delete -g $(GROUP) -n $(CLUSTER) --yes
	@$(MAKE) unset-kubeconf
	@$(MAKE) rg-down
