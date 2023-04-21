.DEFAULT_GOAL: help

# construct containerized azcli command
KUBECFG = $(HOME)/.kube
SSH     = $(HOME)/.ssh
AZCFG   = $(HOME)/.azure
AZIMG   = mcr.microsoft.com/azure-cli
AZCLI   ?= docker run --rm -v $(AZCFG):/root/.azure -v $(KUBECFG):/root/.kube -v $(SSH):/root/.ssh $(AZIMG) az

# overrideable defaults
REGION     ?= westus2
OS_SKU     ?= Ubuntu
VM_SIZE	   ?= Standard_B2s
NODE_COUNT ?= 2

# overrideable variables
SUB        ?= $(AZURE_SUBSCRIPTION)
CLUSTER    ?= $(USER)-$(REGION)
GROUP      ?= $(CLUSTER)
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

vars: ## Show the input vars configured for the cluster commands
	@echo CLUSTER=$(CLUSTER)
	@echo GROUP=$(GROUP)
	@echo REGION=$(REGION)
	@echo SUB=$(SUB)
	@echo VNET=$(VNET)
	@echo OS_SKU=$(OS_SKU)
	@echo VM_SIZE=$(VM_SIZE)
	@echo NODE_COUNT=$(NODE_COUNT)
	@echo VMSS_NAME=$(VMSS_NAME)


##@ SWIFT Infra

rg-up: ## Create resource group 
	@$(AZCLI) group create --location $(REGION) --name $(GROUP)

rg-down: ## Delete resource group
	$(AZCLI) group delete -g $(GROUP) --yes

swift-net-up: ## Create vnet, nodenet and podnet subnets
	$(AZCLI) network vnet create -g $(GROUP) -l $(REGION) --name $(VNET) --address-prefixes 10.0.0.0/8 -o none
	$(AZCLI) network vnet subnet create -g $(GROUP) --vnet-name $(VNET) --name nodenet --address-prefixes 10.240.0.0/16 -o none
	$(AZCLI) network vnet subnet create -g $(GROUP) --vnet-name $(VNET) --name podnet --address-prefixes 10.241.0.0/16 -o none

overlay-net-up: ## Create vnet, nodenet subnets
	$(AZCLI) network vnet create -g $(GROUP) -l $(REGION) --name $(VNET) --address-prefixes 10.0.0.0/8 -o none
	$(AZCLI) network vnet subnet create -g $(GROUP) --vnet-name $(VNET) --name nodenet --address-prefix 10.10.0.0/16 -o none


##@ AKS Clusters

byocni-up: swift-byocni-up ## Alias to swift-byocni-up
cilium-up: swift-cilium-up ## Alias to swift-cilium-up
up: swift-up ## Alias to swift-up

overlay-byocni-up: rg-up overlay-net-up ## Brings up an Overlay BYO CNI cluster
	$(AZCLI) aks create -n $(CLUSTER) -g $(GROUP) -l $(REGION) \
		--node-count $(NODE_COUNT) \
		--node-vm-size $(VM_SIZE) \
		--load-balancer-sku basic \
		--network-plugin none \
		--network-plugin-mode overlay \
		--pod-cidr 192.168.0.0/16 \
		--vnet-subnet-id /subscriptions/$(SUB)/resourceGroups/$(GROUP)/providers/Microsoft.Network/virtualNetworks/$(VNET)/subnets/nodenet \
		--no-ssh-key \
		--yes
	@$(MAKE) set-kubeconf

overlay-cilium-up: rg-up overlay-net-up ## Brings up an Overlay AzCNI cluster
	$(AZCLI) aks create -n $(CLUSTER) -g $(GROUP) -l $(REGION) \
		--node-count $(NODE_COUNT) \
		--node-vm-size $(VM_SIZE) \
		--load-balancer-sku basic \
		--network-plugin azure \
		--network-dataplane cilium \
		--network-plugin-mode overlay \
		--pod-cidr 192.168.0.0/16 \
		--vnet-subnet-id /subscriptions/$(SUB)/resourceGroups/$(GROUP)/providers/Microsoft.Network/virtualNetworks/$(VNET)/subnets/nodenet \
		--no-ssh-key \
		--yes
	@$(MAKE) set-kubeconf

overlay-up: rg-up overlay-net-up ## Brings up an Overlay AzCNI cluster
	$(AZCLI) aks create -n $(CLUSTER) -g $(GROUP) -l $(REGION) \
		--node-count $(NODE_COUNT) \
		--node-vm-size $(VM_SIZE) \
		--load-balancer-sku basic \
		--network-plugin azure \
		--network-plugin-mode overlay \
		--pod-cidr 192.168.0.0/16 \
		--vnet-subnet-id /subscriptions/$(SUB)/resourceGroups/$(GROUP)/providers/Microsoft.Network/virtualNetworks/$(VNET)/subnets/nodenet \
		--no-ssh-key \
		--yes
	@$(MAKE) set-kubeconf

swift-byocni-up: rg-up swift-net-up ## Bring up a SWIFT BYO CNI cluster
	$(AZCLI) aks create -n $(CLUSTER) -g $(GROUP) -l $(REGION) \
		--node-count $(NODE_COUNT) \
		--node-vm-size $(VM_SIZE) \
		--load-balancer-sku basic \
		--network-plugin none \
		--vnet-subnet-id /subscriptions/$(SUB)/resourceGroups/$(GROUP)/providers/Microsoft.Network/virtualNetworks/$(VNET)/subnets/nodenet \
		--pod-subnet-id /subscriptions/$(SUB)/resourceGroups/$(GROUP)/providers/Microsoft.Network/virtualNetworks/$(VNET)/subnets/podnet \
		--no-ssh-key \
		--os-sku $(OS_SKU) \
		--yes
	@$(MAKE) set-kubeconf

swift-cilium-up: rg-up swift-net-up ## Bring up a SWIFT Cilium cluster
	$(AZCLI) aks create -n $(CLUSTER) -g $(GROUP) -l $(REGION) \
		--node-count $(NODE_COUNT) \
		--node-vm-size $(VM_SIZE) \
		--load-balancer-sku basic \
		--network-plugin azure \
		--network-dataplane cilium \
		--aks-custom-headers AKSHTTPCustomFeatures=Microsoft.ContainerService/CiliumDataplanePreview \
		--vnet-subnet-id /subscriptions/$(SUB)/resourceGroups/$(GROUP)/providers/Microsoft.Network/virtualNetworks/$(VNET)/subnets/nodenet \
		--pod-subnet-id /subscriptions/$(SUB)/resourceGroups/$(GROUP)/providers/Microsoft.Network/virtualNetworks/$(VNET)/subnets/podnet \
		--no-ssh-key \
		--yes
	@$(MAKE) set-kubeconf

swift-up: rg-up swift-net-up ## Bring up a SWIFT AzCNI cluster
	$(AZCLI) aks create -n $(CLUSTER) -g $(GROUP) -l $(REGION) \
		--node-count $(NODE_COUNT) \
		--node-vm-size $(VM_SIZE) \
		--load-balancer-sku basic \
		--network-plugin azure \
		--vnet-subnet-id /subscriptions/$(SUB)/resourceGroups/$(GROUP)/providers/Microsoft.Network/virtualNetworks/$(VNET)/subnets/nodenet \
		--pod-subnet-id /subscriptions/$(SUB)/resourceGroups/$(GROUP)/providers/Microsoft.Network/virtualNetworks/$(VNET)/subnets/podnet \
		--no-ssh-key \
		--yes
	@$(MAKE) set-kubeconf

down: ## Delete the cluster
	$(AZCLI) aks delete -g $(GROUP) -n $(CLUSTER) --yes
	@$(MAKE) unset-kubeconf
	@$(MAKE) rg-down

restart-vmss: ## Restarts the nodes in the cluster
	$(AZCLI) vmss restart -g MC_${GROUP}_${CLUSTER}_${REGION} --name $(VMSS_NAME)
