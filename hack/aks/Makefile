include deploy.mk

.DEFAULT_GOAL: help

# construct containerized azcli command
KUBECFG = $(HOME)/.kube
SSH     = $(HOME)/.ssh
AZCFG   = $(HOME)/.azure
AZIMG   = mcr.microsoft.com/azure-cli
AZCLI   ?= docker run --rm -v $(AZCFG):/root/.azure -v $(KUBECFG):/root/.kube -v $(SSH):/root/.ssh -v $(PWD):/root/tmpsrc $(AZIMG) az

# overrideable defaults
AUTOUPGRADE          ?= patch
K8S_VER              ?= 1.33
NODE_COUNT           ?= 2
NODE_COUNT_WIN	     ?= $(NODE_COUNT)
NODEUPGRADE          ?= NodeImage
OS				     ?= linux # Used to signify if you want to bring up a windows nodePool on byocni clusters
OS_SKU               ?= Ubuntu
OS_SKU_WIN           ?= Windows2022
REGION               ?= westus2
VM_SIZE	             ?= Standard_B2s
VM_SIZE_WIN          ?= Standard_B2s
IP_TAG               ?= FirstPartyUsage=/NonProd
IP_PREFIX            ?= serviceTaggedIp
PUBLIC_IP_ID         ?= /subscriptions/$(SUB)/resourceGroups/$(GROUP)/providers/Microsoft.Network/publicIPAddresses
PUBLIC_IPv4          ?= $(PUBLIC_IP_ID)/$(IP_PREFIX)-$(CLUSTER)-v4
PUBLIC_IPv6          ?= $(PUBLIC_IP_ID)/$(IP_PREFIX)-$(CLUSTER)-v6
KUBE_PROXY_JSON_PATH ?= ./kube-proxy.json
LTS					 ?= false
ACNS				 ?= false


# overrideable variables
SUB        ?= $(AZURE_SUBSCRIPTION)
CLUSTER    ?= $(USER)-$(REGION)
GROUP      ?= $(CLUSTER)
VNET       ?= $(CLUSTER)

# Long Term Support (LTS)
ifeq ($(LTS),true)
	LTS_ARGS=--k8s-support-plan AKSLongTermSupport --tier premium
else
	LTS_ARGS=
endif

# Only use with *cilium* recipes
# Advanced Container Networking Services (ACNS)
ifeq ($(ACNS),true)
	ACNS_ARGS=--enable-acns
else
	ACNS_ARGS=
endif

# Common az aks create fields
COMMON_AKS_FIELDS = $(AZCLI) aks create -n $(CLUSTER) -g $(GROUP) -l $(REGION) \
	--auto-upgrade-channel $(AUTOUPGRADE) \
	--node-os-upgrade-channel $(NODEUPGRADE) \
	--kubernetes-version $(K8S_VER) \
	--node-count $(NODE_COUNT) \
	--node-vm-size $(VM_SIZE) \
	--no-ssh-key \
	--os-sku $(OS_SKU) \
	$(LTS_ARGS) $(ACNS_ARGS)
POD_CIDR = 192.168.0.0/16

##@ Help

help:  ## Display this help
	@awk 'BEGIN {FS = ":.*##"; printf "\nUsage:\n  make \033[36m<target>\033[0m\n"} /^[0-9a-zA-Z_-]+:.*?##/ { printf "  \033[36m%-15s\033[0m %s\n", $$1, $$2 } /^##@/ { printf "\n\033[1m%s\033[0m\n", substr($$0, 5) } ' $(MAKEFILE_LIST)


##@ Utilities

azlogin: ## Login and set account to $SUB
	@$(AZCLI) login
	@$(AZCLI) account set -s $(SUB)

azcfg: ## Set the $AZCLI to use aks-preview
	@$(AZCLI) extension add --name aks-preview --yes
	@$(AZCLI) extension update --name aks-preview

ip:
	$(AZCLI) network public-ip create --name $(IP_PREFIX)-$(CLUSTER)-$(IPVERSION) \
		--resource-group $(GROUP) \
		--allocation-method Static \
		--ip-tags $(IP_TAG) \
		--location $(REGION) \
		--sku Standard \
		--tier Regional \
		--version IP$(IPVERSION)

ipv4:
	@$(MAKE) ip IPVERSION=v4

ipv6:
	@$(MAKE) ip IPVERSION=v6


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
	@echo K8S_VER=$(K8S_VER)
	@echo LTS_ARGS=$(if $(LTS_ARGS),$(LTS_ARGS),$(LTS))
	@echo ACNS_ARGS=$(if $(ACNS_ARGS),$(ACNS_ARGS),$(ACNS))
	@echo COMMON_AKS_FIELDS=$(COMMON_AKS_FIELDS)


##@ SWIFT Infra

rg-up: ## Create resource group
	@$(AZCLI) group create --location $(REGION) --name $(GROUP)

rg-down: ## Delete resource group
	$(AZCLI) group delete -g $(GROUP) --yes

swift-net-up: ## Create vnet, nodenet and podnet subnets
	$(AZCLI) network vnet create -g $(GROUP) -l $(REGION) --name $(VNET) --address-prefixes 10.0.0.0/8 -o none
	$(AZCLI) network vnet subnet create -g $(GROUP) --vnet-name $(VNET) --name nodenet --address-prefixes 10.240.0.0/16 -o none
	$(AZCLI) network vnet subnet create -g $(GROUP) --vnet-name $(VNET) --name podnet --address-prefixes 10.241.0.0/16 -o none

vnetscale-swift-net-up: ## Create vnet, nodenet and podnet subnets for vnet scale
	$(AZCLI) network vnet create -g $(GROUP) -l $(REGION) --name $(VNET) --address-prefixes 10.0.0.0/8 -o none
	$(AZCLI) network vnet subnet create -g $(GROUP) --vnet-name $(VNET) --name nodenet --address-prefixes 10.240.0.0/16 -o none
	$(AZCLI) network vnet subnet create -g $(GROUP) --vnet-name $(VNET) --name podnet --address-prefixes 10.40.0.0/13 -o none

overlay-net-up: ## Create vnet, nodenet subnets
	$(AZCLI) network vnet create -g $(GROUP) -l $(REGION) --name $(VNET) --address-prefixes 10.0.0.0/8 -o none
	$(AZCLI) network vnet subnet create -g $(GROUP) --vnet-name $(VNET) --name nodenet --address-prefix 10.10.0.0/16 -o none

##@ AKS Clusters

byocni-up: swift-byocni-up ## Alias to swift-byocni-up
cilium-up: swift-cilium-up ## Alias to swift-cilium-up
up: swift-up ## Alias to swift-up


nodesubnet-byocni-nokubeproxy-up: rg-up ipv4 overlay-net-up ## Brings up an NodeSubnet BYO CNI cluster without kube-proxy
	$(COMMON_AKS_FIELDS) \
		--max-pods 250 \
		--load-balancer-outbound-ips $(PUBLIC_IPv4) \
		--network-plugin none \
		--vnet-subnet-id /subscriptions/$(SUB)/resourceGroups/$(GROUP)/providers/Microsoft.Network/virtualNetworks/$(VNET)/subnets/nodenet \
		--kube-proxy-config $(KUBE_PROXY_JSON_PATH) \
		--aks-custom-headers AKSHTTPCustomFeatures=Microsoft.ContainerService/Ubuntu2404Preview \
		--yes
	@$(MAKE) set-kubeconf

overlay-byocni-up: rg-up ipv4 overlay-net-up ## Brings up an Overlay BYO CNI cluster
	$(COMMON_AKS_FIELDS) \
		--load-balancer-outbound-ips $(PUBLIC_IPv4) \
		--network-plugin none \
		--network-plugin-mode overlay \
		--pod-cidr $(POD_CIDR) \
		--vnet-subnet-id /subscriptions/$(SUB)/resourceGroups/$(GROUP)/providers/Microsoft.Network/virtualNetworks/$(VNET)/subnets/nodenet \
		--yes
	@$(MAKE) set-kubeconf
ifeq ($(OS),windows)
	$(MAKE) windows-nodepool-up
endif

overlay-byocni-nokubeproxy-up: rg-up ipv4 overlay-net-up ## Brings up an Overlay BYO CNI cluster without kube-proxy
	$(COMMON_AKS_FIELDS) \
		--load-balancer-outbound-ips $(PUBLIC_IPv4) \
		--network-plugin none \
		--network-plugin-mode overlay \
		--pod-cidr $(POD_CIDR) \
		--vnet-subnet-id /subscriptions/$(SUB)/resourceGroups/$(GROUP)/providers/Microsoft.Network/virtualNetworks/$(VNET)/subnets/nodenet \
		--kube-proxy-config $(KUBE_PROXY_JSON_PATH) \
		--aks-custom-headers AKSHTTPCustomFeatures=Microsoft.ContainerService/Ubuntu2404Preview \
		--yes
	@$(MAKE) set-kubeconf

overlay-cilium-up: rg-up ipv4 overlay-net-up ## Brings up an Overlay Cilium cluster
	$(COMMON_AKS_FIELDS) \
		--load-balancer-outbound-ips $(PUBLIC_IPv4) \
		--network-plugin azure \
		--network-dataplane cilium \
		--network-plugin-mode overlay \
		--pod-cidr $(POD_CIDR) \
		--vnet-subnet-id /subscriptions/$(SUB)/resourceGroups/$(GROUP)/providers/Microsoft.Network/virtualNetworks/$(VNET)/subnets/nodenet \
		--yes
	@$(MAKE) set-kubeconf

ebpf-overlay-cilium-up: rg-up ipv4 overlay-net-up ## Brings up an EBPF Overlay Cilium cluster
	$(COMMON_AKS_FIELDS) \
		--load-balancer-outbound-ips $(PUBLIC_IPv4) \
		--network-plugin azure \
		--network-dataplane cilium \
		--network-plugin-mode overlay \
		--pod-cidr $(POD_CIDR) \
		--vnet-subnet-id /subscriptions/$(SUB)/resourceGroups/$(GROUP)/providers/Microsoft.Network/virtualNetworks/$(VNET)/subnets/nodenet \
		--aks-custom-headers AKSHTTPCustomFeatures=Microsoft.ContainerService/AdvancedNetworkingPerformancePreview \
		--enable-acns \
		--acns-datapath-acceleration-mode BpfVeth \
		--yes
	@$(MAKE) set-kubeconf

overlay-up: rg-up ipv4 overlay-net-up ## Brings up an Overlay AzCNI cluster
	$(COMMON_AKS_FIELDS) \
		--load-balancer-outbound-ips $(PUBLIC_IPv4) \
		--network-plugin azure \
		--network-plugin-mode overlay \
		--pod-cidr $(POD_CIDR) \
		--vnet-subnet-id /subscriptions/$(SUB)/resourceGroups/$(GROUP)/providers/Microsoft.Network/virtualNetworks/$(VNET)/subnets/nodenet \
		--yes
	@$(MAKE) set-kubeconf
ifeq ($(OS),windows)
	$(MAKE) windows-nodepool-up
endif

swift-byocni-up: rg-up ipv4 swift-net-up ## Bring up a SWIFT BYO CNI cluster
	$(COMMON_AKS_FIELDS) \
		--load-balancer-outbound-ips $(PUBLIC_IPv4) \
		--network-plugin none \
		--vnet-subnet-id /subscriptions/$(SUB)/resourceGroups/$(GROUP)/providers/Microsoft.Network/virtualNetworks/$(VNET)/subnets/nodenet \
		--pod-subnet-id /subscriptions/$(SUB)/resourceGroups/$(GROUP)/providers/Microsoft.Network/virtualNetworks/$(VNET)/subnets/podnet \
		--yes
ifeq ($(OS),windows)
	@$(MAKE) windows-swift-nodepool-up
endif
	@$(MAKE) set-kubeconf

swift-byocni-nokubeproxy-up: rg-up ipv4 swift-net-up ## Bring up a SWIFT BYO CNI cluster without kube-proxy, add managed identity and public ip
	$(COMMON_AKS_FIELDS) \
		--load-balancer-outbound-ips $(PUBLIC_IPv4) \
		--network-plugin none \
		--vnet-subnet-id /subscriptions/$(SUB)/resourceGroups/$(GROUP)/providers/Microsoft.Network/virtualNetworks/$(VNET)/subnets/nodenet \
		--pod-subnet-id /subscriptions/$(SUB)/resourceGroups/$(GROUP)/providers/Microsoft.Network/virtualNetworks/$(VNET)/subnets/podnet \
		--kube-proxy-config $(KUBE_PROXY_JSON_PATH) \
		--aks-custom-headers AKSHTTPCustomFeatures=Microsoft.ContainerService/Ubuntu2404Preview \
		--yes
	@$(MAKE) set-kubeconf

swift-cilium-up: rg-up ipv4 swift-net-up ## Bring up a SWIFT Cilium cluster
	$(COMMON_AKS_FIELDS) \
		--load-balancer-outbound-ips $(PUBLIC_IPv4) \
		--network-plugin azure \
		--network-dataplane cilium \
		--aks-custom-headers AKSHTTPCustomFeatures=Microsoft.ContainerService/CiliumDataplanePreview \
		--vnet-subnet-id /subscriptions/$(SUB)/resourceGroups/$(GROUP)/providers/Microsoft.Network/virtualNetworks/$(VNET)/subnets/nodenet \
		--pod-subnet-id /subscriptions/$(SUB)/resourceGroups/$(GROUP)/providers/Microsoft.Network/virtualNetworks/$(VNET)/subnets/podnet \
		--yes
	@$(MAKE) set-kubeconf

swift-up: rg-up ipv4 swift-net-up ## Bring up a SWIFT AzCNI cluster
	$(COMMON_AKS_FIELDS) \
		--load-balancer-outbound-ips $(PUBLIC_IPv4) \
		--network-plugin azure \
		--vnet-subnet-id /subscriptions/$(SUB)/resourceGroups/$(GROUP)/providers/Microsoft.Network/virtualNetworks/$(VNET)/subnets/nodenet \
		--pod-subnet-id /subscriptions/$(SUB)/resourceGroups/$(GROUP)/providers/Microsoft.Network/virtualNetworks/$(VNET)/subnets/podnet \
		--yes
	@$(MAKE) set-kubeconf

swiftv2-multitenancy-cluster-up: rg-up ipv4
	$(AZCLI) aks create -n $(CLUSTER) -g $(GROUP) -l $(REGION) \
		--network-plugin azure \
		--network-plugin-mode overlay \
		--kubernetes-version $(K8S_VER) \
		--nodepool-name "mtapool" \
		--node-vm-size $(VM_SIZE) \
		--node-count 2 \
		--load-balancer-outbound-ips $(PUBLIC_IPv4) \
		--nodepool-tags fastpathenabled=true \
		--no-ssh-key \
		$(LTS_ARGS) \
		--yes
	@$(MAKE) set-kubeconf

swiftv2-dummy-cluster-up: rg-up ipv4 swift-net-up ## Bring up a SWIFT AzCNI cluster
	$(AZCLI) aks create -n $(CLUSTER) -g $(GROUP) -l $(REGION) \
		--network-plugin azure \
		--vnet-subnet-id /subscriptions/$(SUB)/resourceGroups/$(GROUP)/providers/Microsoft.Network/virtualNetworks/$(VNET)/subnets/nodenet \
		--pod-subnet-id /subscriptions/$(SUB)/resourceGroups/$(GROUP)/providers/Microsoft.Network/virtualNetworks/$(VNET)/subnets/podnet \
		--tags stampcreatorserviceinfo=true \
		--load-balancer-outbound-ips $(PUBLIC_IPv4) \
		--no-ssh-key \
		--yes
	@$(MAKE) set-kubeconf

swiftv2-podsubnet-cluster-up: ipv4 swift-net-up ## Bring up a SWIFTv2 PodSubnet cluster
	$(COMMON_AKS_FIELDS) \
		--network-plugin azure \
		--node-vm-size $(VM_SIZE) \
		--vnet-subnet-id /subscriptions/$(SUB)/resourceGroups/$(GROUP)/providers/Microsoft.Network/virtualNetworks/$(VNET)/subnets/nodenet \
		--pod-subnet-id /subscriptions/$(SUB)/resourceGroups/$(GROUP)/providers/Microsoft.Network/virtualNetworks/$(VNET)/subnets/podnet \
		--nodepool-tags fastpathenabled=true aks-nic-enable-multi-tenancy=true \
		--tags stampcreatorserviceinfo=true \
		--aks-custom-headers AKSHTTPCustomFeatures=Microsoft.ContainerService/NetworkingMultiTenancyPreview \
		--load-balancer-outbound-ips $(PUBLIC_IPv4) \
		--yes
	@$(MAKE) set-kubeconf

# The below Vnet Scale clusters are currently only in private preview and available with Kubernetes 1.28
# These AKS clusters can only be created in a limited subscription listed here:
# https://dev.azure.com/msazure/CloudNativeCompute/_git/aks-rp?path=/resourceprovider/server/microsoft.com/containerservice/flags/network_flags.go&version=GBmaster&line=134&lineEnd=135&lineStartColumn=1&lineEndColumn=1&lineStyle=plain&_a=contents
vnetscale-swift-byocni-up: rg-up ipv4 vnetscale-swift-net-up ## Bring up a Vnet Scale SWIFT BYO CNI cluster
	$(COMMON_AKS_FIELDS) \
		--load-balancer-outbound-ips $(PUBLIC_IPv4) \
		--network-plugin none \
		--vnet-subnet-id /subscriptions/$(SUB)/resourceGroups/$(GROUP)/providers/Microsoft.Network/virtualNetworks/$(VNET)/subnets/nodenet \
		--pod-subnet-id /subscriptions/$(SUB)/resourceGroups/$(GROUP)/providers/Microsoft.Network/virtualNetworks/$(VNET)/subnets/podnet \
		--pod-ip-allocation-mode StaticBlock \
		--yes
	@$(MAKE) set-kubeconf

vnetscale-swift-byocni-nokubeproxy-up: rg-up ipv4 vnetscale-swift-net-up ## Bring up a Vnet Scale SWIFT BYO CNI cluster without kube-proxy
	$(COMMON_AKS_FIELDS) \
		--load-balancer-outbound-ips $(PUBLIC_IPv4) \
		--network-plugin none \
		--vnet-subnet-id /subscriptions/$(SUB)/resourceGroups/$(GROUP)/providers/Microsoft.Network/virtualNetworks/$(VNET)/subnets/nodenet \
		--pod-subnet-id /subscriptions/$(SUB)/resourceGroups/$(GROUP)/providers/Microsoft.Network/virtualNetworks/$(VNET)/subnets/podnet \
		--kube-proxy-config $(KUBE_PROXY_JSON_PATH) \
		--pod-ip-allocation-mode StaticBlock \
		--aks-custom-headers AKSHTTPCustomFeatures=Microsoft.ContainerService/Ubuntu2404Preview \
		--yes
	@$(MAKE) set-kubeconf

vnetscale-swift-cilium-up: rg-up ipv4 vnetscale-swift-net-up ## Bring up a Vnet Scale SWIFT Cilium cluster
	$(COMMON_AKS_FIELDS) \
		--load-balancer-outbound-ips $(PUBLIC_IPv4) \
		--network-plugin azure \
		--network-dataplane cilium \
		--aks-custom-headers AKSHTTPCustomFeatures=Microsoft.ContainerService/CiliumDataplanePreview \
		--vnet-subnet-id /subscriptions/$(SUB)/resourceGroups/$(GROUP)/providers/Microsoft.Network/virtualNetworks/$(VNET)/subnets/nodenet \
		--pod-subnet-id /subscriptions/$(SUB)/resourceGroups/$(GROUP)/providers/Microsoft.Network/virtualNetworks/$(VNET)/subnets/podnet \
		--pod-ip-allocation-mode StaticBlock \
		--yes
	@$(MAKE) set-kubeconf

vnetscale-swift-up: rg-up ipv4 vnetscale-swift-net-up ## Bring up a Vnet Scale SWIFT AzCNI cluster
	$(COMMON_AKS_FIELDS) \
		--load-balancer-outbound-ips $(PUBLIC_IPv4) \
		--network-plugin azure \
		--vnet-subnet-id /subscriptions/$(SUB)/resourceGroups/$(GROUP)/providers/Microsoft.Network/virtualNetworks/$(VNET)/subnets/nodenet \
		--pod-subnet-id /subscriptions/$(SUB)/resourceGroups/$(GROUP)/providers/Microsoft.Network/virtualNetworks/$(VNET)/subnets/podnet \
		--pod-ip-allocation-mode StaticBlock \
		--yes
	@$(MAKE) set-kubeconf

nodesubnet-cilium-up: rg-up ipv4 overlay-net-up ## Bring up a Nodesubnet Cilium cluster
	$(COMMON_AKS_FIELDS) \
		--load-balancer-outbound-ips $(PUBLIC_IPv4) \
		--network-plugin azure \
		--network-dataplane cilium \
		--vnet-subnet-id /subscriptions/$(SUB)/resourceGroups/$(GROUP)/providers/Microsoft.Network/virtualNetworks/$(VNET)/subnets/nodenet \
		--yes
	@$(MAKE) set-kubeconf

cniv1-up: rg-up ipv4 overlay-net-up ## Bring up a CNIv1 cluster
	$(COMMON_AKS_FIELDS) \
		--load-balancer-outbound-ips $(PUBLIC_IPv4) \
		--max-pods 250 \
		--network-plugin azure \
		--vnet-subnet-id /subscriptions/$(SUB)/resourceGroups/$(GROUP)/providers/Microsoft.Network/virtualNetworks/$(VNET)/subnets/nodenet \
		--yes
	@$(MAKE) set-kubeconf
ifeq ($(OS),windows)
	$(MAKE) windows-nodepool-up
endif

dualstack-overlay-up: rg-up ipv4 ipv6 overlay-net-up ## Brings up an dualstack Overlay cluster with Linux node only
	$(COMMON_AKS_FIELDS) \
		--load-balancer-outbound-ips $(PUBLIC_IPv4),$(PUBLIC_IPv6) \
		--network-plugin azure \
		--network-plugin-mode overlay \
		--subscription $(SUB) \
		--ip-families ipv4,ipv6 \
		--aks-custom-headers AKSHTTPCustomFeatures=Microsoft.ContainerService/AzureOverlayDualStackPreview \
		--yes
	@$(MAKE) set-kubeconf

dualstack-overlay-byocni-up: rg-up ipv4 ipv6 overlay-net-up ## Brings up an dualstack Overlay BYO CNI cluster
	$(COMMON_AKS_FIELDS) \
		--load-balancer-outbound-ips $(PUBLIC_IPv4),$(PUBLIC_IPv6) \
		--network-plugin none \
		--network-plugin-mode overlay \
		--subscription $(SUB) \
		--ip-families ipv4,ipv6 \
		--aks-custom-headers AKSHTTPCustomFeatures=Microsoft.ContainerService/AzureOverlayDualStackPreview \
		--yes
	@$(MAKE) set-kubeconf
ifeq ($(OS),windows)
	$(MAKE) windows-nodepool-up
endif

cilium-dualstack-up: rg-up ipv4 ipv6 overlay-net-up ## Brings up a Cilium Dualstack Overlay cluster with Linux node only
	$(COMMON_AKS_FIELDS) \
		--load-balancer-outbound-ips $(PUBLIC_IPv4),$(PUBLIC_IPv6) \
		--network-plugin azure \
		--network-plugin-mode overlay \
		--network-dataplane cilium \
		--subscription $(SUB) \
		--ip-families ipv4,ipv6 \
		--aks-custom-headers AKSHTTPCustomFeatures=Microsoft.ContainerService/AzureOverlayDualStackPreview \
		--yes
	@$(MAKE) set-kubeconf

dualstack-byocni-nokubeproxy-up: rg-up ipv4 ipv6 overlay-net-up ## Brings up a Dualstack overlay BYOCNI cluster with Linux node only and no kube-proxy
	$(COMMON_AKS_FIELDS) \
		--load-balancer-outbound-ips $(PUBLIC_IPv4),$(PUBLIC_IPv6) \
		--network-plugin none \
		--network-plugin-mode overlay \
		--subscription $(SUB) \
		--ip-families ipv4,ipv6 \
		--aks-custom-headers AKSHTTPCustomFeatures=Microsoft.ContainerService/AzureOverlayDualStackPreview,AKSHTTPCustomFeatures=Microsoft.ContainerService/Ubuntu2404Preview \
		--kube-proxy-config $(KUBE_PROXY_JSON_PATH) \
		--yes
	@$(MAKE) set-kubeconf

windows-nodepool-up: ## Add windows node pool
	AZCLI="az" GROUP="$(GROUP)" CLUSTER="$(CLUSTER)" sh ../scripts/wait-cluster-update.sh
	$(AZCLI) aks nodepool add -g $(GROUP) -n npwin \
		--node-count $(NODE_COUNT_WIN) \
		--node-vm-size $(VM_SIZE_WIN) \
		--cluster-name $(CLUSTER) \
		--os-type Windows \
		--os-sku $(OS_SKU_WIN) \
		--max-pods 250 \
		--subscription $(SUB)

windows-swift-nodepool-up: ## Add windows node pool
	$(AZCLI) aks nodepool add -g $(GROUP) -n npwin \
		--node-count $(NODE_COUNT_WIN) \
		--node-vm-size $(VM_SIZE_WIN) \
		--cluster-name $(CLUSTER) \
		--os-type Windows \
		--os-sku $(OS_SKU_WIN) \
		--max-pods 250 \
		--subscription $(SUB) \
		--pod-subnet-id /subscriptions/$(SUB)/resourceGroups/$(GROUP)/providers/Microsoft.Network/virtualNetworks/$(VNET)/subnets/podnet

linux-swiftv2-nodepool-up: ## Add linux node pool to swiftv2 cluster
	$(AZCLI) aks nodepool add -g $(GROUP) -n nplinux \
		--node-count $(NODE_COUNT) \
		--node-vm-size $(VM_SIZE) \
		--cluster-name $(CLUSTER) \
		--os-type Linux \
		--max-pods 250 \
		--subscription $(SUB) \
		--tags fastpathenabled=true aks-nic-enable-multi-tenancy=true stampcreatorserviceinfo=true aks-nic-secondary-count=${PODS_PER_NODE} \
		--aks-custom-headers AKSHTTPCustomFeatures=Microsoft.ContainerService/NetworkingMultiTenancyPreview \
		--pod-subnet-id /subscriptions/$(SUB)/resourceGroups/$(GROUP)/providers/Microsoft.Network/virtualNetworks/$(VNET)/subnets/podnet

down: ## Delete the cluster
	$(AZCLI) aks delete -g $(GROUP) -n $(CLUSTER) --yes
	@$(MAKE) unset-kubeconf
	@$(MAKE) rg-down

restart-vmss: ## Restarts the nodes in the cluster
	$(AZCLI) vmss restart -g MC_${GROUP}_${CLUSTER}_${REGION} --name $(VMSS_NAME)

scale-nodes: ## Scales the nodes in the cluster
	$(AZCLI) aks nodepool scale --resource-group $(GROUP) --cluster-name $(CLUSTER) --name $(NODEPOOL) --node-count $(NODE_COUNT)
