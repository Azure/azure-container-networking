#!/bin/bash
# Requires
# sufix1 - unique single digit whole number 1-9. Cannot match sufix2
# sufix2 - unique single digit whole number 1-9. Cannot match sufix1
# SUB - GUID for subscription
# clusterType - swift-byocni-nokubeproxy-up is primary atm, but leaving for testing later.
# Example command: clusterPrefix=<alais> sufix1=1 sufix2=2 SUB=<GUID> clusterType=swift-byocni-nokubeproxy-up ./cil-script.sh

sufixes="${sufix1} ${sufix2}"
install=helm
echo "sufixes ${sufixes}"

cd ../..
for unique in $sufixes; do
    make -C ./hack/aks $clusterType \
        AZCLI=az REGION=westus2 SUB=$SUB \
        CLUSTER=${clusterPrefix}-${unique} \
        KUBE_PROXY_JSON_PATH=./kube-proxy.json \
        NODE_COUNT=1 \
        VNET_PREFIX=10.${unique}.0.0/16 \
        NODE_SUBNET_PREFIX=10.${unique}.1.0/24 \
        POD_SUBNET_PREFIX=10.${unique}.2.0/24

    kubectl config use-context ${clusterPrefix}-${unique}

    if [ $install == "helm" ]; then
        helm upgrade --install -n kube-system cilium cilium/cilium \
        --version v1.16.1 \
        --set cluster.name=${clusterPrefix}-${unique} \
        --set azure.resourceGroup=${clusterPrefix}-${unique}-rg \
        --set cluster.id=${unique} \
        --set ipam.operator.clusterPoolIPv4PodCIDRList='{10.'${unique}'.2.0/24}' \
        --set hubble.enabled=false \
        --set envoy.enabled=false
    fi
done

cd hack/scripts

VNET_ID1=$(az network vnet show \
    --resource-group "${clusterPrefix}-${sufix1}-rg" \
    --name "${clusterPrefix}-${sufix1}-vnet" \
    --query id -o tsv)

VNET_ID2=$(az network vnet show \
    --resource-group "${clusterPrefix}-${sufix2}-rg" \
    --name "${clusterPrefix}-${sufix2}-vnet" \
    --query id -o tsv)

az network vnet peering create \
    -g "${clusterPrefix}-${sufix1}-rg" \
    --name "peering-${clusterPrefix}-${sufix1}-to-${clusterPrefix}-${sufix2}" \
    --vnet-name "${clusterPrefix}-${sufix1}-vnet" \
    --remote-vnet "${VNET_ID2}" \
    --allow-vnet-access

az network vnet peering create \
    -g "${clusterPrefix}-${sufix2}-rg" \
    --name "peering-${clusterPrefix}-${sufix2}-to-${clusterPrefix}-${sufix1}" \
    --vnet-name "${clusterPrefix}-${sufix2}-vnet" \
    --remote-vnet "${VNET_ID1}" \
    --allow-vnet-access


cilium clustermesh enable --context ${clusterPrefix}-${sufix1} --enable-kvstoremesh=true
cilium clustermesh enable --context ${clusterPrefix}-${sufix2} --enable-kvstoremesh=true


cilium clustermesh status --context ${clusterPrefix}-${sufix1} --wait
cilium clustermesh status --context ${clusterPrefix}-${sufix2} --wait

# # CA is passed between clusters in this step
cilium clustermesh connect --context ${clusterPrefix}-${sufix1} --destination-context ${clusterPrefix}-${sufix2}

# For 3+ clusters
# cilium clustermesh connect --context ${clusterPrefix}-${sufix1} --destination-context ${clusterPrefix}-${sufix2}  --connection-mode mesh
# These can be run in parallel in different bash shells
