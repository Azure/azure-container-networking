.DEFAULT_GOAL: help

# construct containerized azcli command
KUBECFG = $(HOME)/.kube
SSH 	= $(HOME)/.ssh
AZCFG 	= $(HOME)/.azure
AZIMG 	= mcr.microsoft.com/azure-cli
AZCLI 	= docker run -v $(AZCFG):/root/.azure -v $(KUBECFG):/root/.kube -v $(SSH):/root/.ssh $(AZIMG)

# overrideable variables
USER 	?= $(whoami)
SUB 	?= $(AZURE_SUBSCRIPTION) # must be defined in calling context
REGION 	?= centraluseuap
CLUSTER ?= $(USER)-swift-$(REGION)
GROUP 	?= $(CLUSTER)
VNET 	?= $(CLUSTER)

##@ Help

help:  ## Display this help
	@awk 'BEGIN {FS = ":.*##"; printf "\nUsage:\n  make \033[36m<target>\033[0m\n"} /^[a-zA-Z_-]+:.*?##/ { printf "  \033[36m%-15s\033[0m %s\n", $$1, $$2 } /^##@/ { printf "\n\033[1m%s\033[0m\n", substr($$0, 5) } ' $(MAKEFILE_LIST)


##@ Utilities 

azcfg:
	@$(AZCLI) az account set -s $(SUB)
	@$(AZCLI) az group create --location $(REGION) --name $(GROUP)
	@$(AZCLI) az configure --defaults group=$(GROUP)
	@$(AZCLI) az extension add --name aks-preview --yes


##@ SWIFT

swift-vars: ## Show the env vars configured for the swift command
	@echo SUB=$(SUB)
	@echo GROUP=$(GROUP)
	@echo REGION=$(REGION)
	@echo VNET=$(VNET)
	@echo CLUSTER=$(CLUSTER)

swift-net-up: azcfg ## Create required SWIFT resources
	$(AZCLI) az network vnet create -g $(GROUP) --name $(VNET) --address-prefixes 10.0.0.0/8 -o none
	$(AZCLI) az network vnet subnet create -g $(GROUP) --vnet-name $(VNET) --name nodenet --address-prefixes 10.240.0.0/16 -o none
	$(AZCLI) az network vnet subnet create -g $(GROUP) --vnet-name $(VNET) --name podnet --address-prefixes 10.241.0.0/16 -o none

swift-up: azcfg swift-net-up ## Brings up a swift cluster $name in $sub/$region
	$(AZCLI) az aks create -n $(CLUSTER) -g $(GROUP) -l $(REGION) \
		--max-pods 250 \
		--node-count 3 \
		--network-plugin azure \
		--vnet-subnet-id /subscriptions/$(SUB)/resourceGroups/$(GROUP)/providers/Microsoft.Network/virtualNetworks/$(VNET)/subnets/nodenet \
		--pod-subnet-id /subscriptions/$(SUB)/resourceGroups/$(GROUP)/providers/Microsoft.Network/virtualNetworks/$(VNET)/subnets/podnet \
		--yes
	$(AZCLI) az aks get-credentials -n $(CLUSTER) -g $(GROUP)

swift-down: azcfg ## Deletes the swift cluster $name in $sub/$region 
	$(AZCLI) az aks delete -g $(GROUP) -n $(CLUSTER) --yes

swift-cleanup: azcfg ## Delete the $rg in $sub/$region
	$(AZCLI) az group delete -g $(GROUP) --yes
