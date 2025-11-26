#!/usr/bin/env bash
set -euo pipefail
trap 'echo "[ERROR] Failed during Resource group or AKS cluster creation." >&2' ERR
SUBSCRIPTION_ID=$1
LOCATION=$2
RG=$3
VM_SKU_DEFAULT=$4
VM_SKU_HIGHNIC=$5

CLUSTER_COUNT=2
CLUSTER_PREFIX="aks"


stamp_vnet() {
    local vnet_id="$1"

    responseFile="response.txt"
    modified_vnet="${vnet_id//\//%2F}"
    cmd_stamp_curl="'curl -v -X PUT http://localhost:8080/VirtualNetwork/$modified_vnet/stampcreatorservicename'"
    cmd_containerapp_exec="az containerapp exec -n subnetdelegator-westus-u3h4j -g subnetdelegator-westus --subscription 9b8218f9-902a-4d20-a65c-e98acec5362f --command $cmd_stamp_curl"
    
    max_retries=10
    sleep_seconds=15
    retry_count=0

    while [[ $retry_count -lt $max_retries ]]; do
        script --quiet -c "$cmd_containerapp_exec" "$responseFile"
        if grep -qF "200 OK" "$responseFile"; then
            echo "Subnet Delegator successfully stamped the vnet"
            return 0
        else
            echo "Subnet Delegator failed to stamp the vnet, attempt $((retry_count+1))"
            cat "$responseFile"
            retry_count=$((retry_count+1))
            sleep "$sleep_seconds"
        fi
    done

    echo "Failed to stamp the vnet even after $max_retries attempts"
    exit 1
}

wait_for_provisioning() {
  local rg="$1" clusterName="$2"
  echo "Waiting for AKS '$clusterName' in RG '$rg'..."
  while :; do
    state=$(az aks show --resource-group "$rg" --name "$clusterName" --query provisioningState -o tsv 2>/dev/null || true)
    if [[ "$state" =~ Succeeded ]]; then
      echo "Provisioning state: $state"
      break
    fi
    if [[ "$state" =~ Failed|Canceled ]]; then
      echo "Provisioning finished with state: $state"
      break
    fi
    sleep 6
  done
}


#########################################
# Main script starts here
#########################################

for i in $(seq 1 "$CLUSTER_COUNT"); do
    echo "Creating cluster #$i..."

    CLUSTER_NAME="${CLUSTER_PREFIX}-${i}"

    make -C ./hack/aks azcfg AZCLI=az REGION=$LOCATION

    # Create cluster with SkipAutoDeleteTill tag for persistent infrastructure
    make -C ./hack/aks swiftv2-podsubnet-cluster-up \
      AZCLI=az REGION=$LOCATION \
      SUB=$SUBSCRIPTION_ID \
      GROUP=$RG \
      CLUSTER=$CLUSTER_NAME \
      VM_SIZE=$VM_SKU_DEFAULT
    
    # Add SkipAutoDeleteTill tag to cluster (2032-12-31 for long-term persistence)
    az aks update -g "$RG" -n "$CLUSTER_NAME" --tags SkipAutoDeleteTill=2032-12-31 || echo "Warning: Failed to add tag to cluster"

    wait_for_provisioning "$RG" "$CLUSTER_NAME"

    vnet_id=$(az network vnet show -g "$RG" --name "$CLUSTER_NAME" --query id -o tsv)
    echo "Found VNET: $vnet_id"
    
    # Add SkipAutoDeleteTill tag to AKS VNet
    az network vnet update --ids "$vnet_id" --set tags.SkipAutoDeleteTill=2032-12-31 || echo "Warning: Failed to add tag to vnet"

    stamp_vnet "$vnet_id"

    make -C ./hack/aks linux-swiftv2-nodepool-up \
      AZCLI=az REGION=$LOCATION \
      GROUP=$RG \
      VM_SIZE=$VM_SKU_HIGHNIC \
      CLUSTER=$CLUSTER_NAME \
      SUB=$SUBSCRIPTION_ID

    az aks get-credentials -g "$RG" -n "$CLUSTER_NAME" --admin --overwrite-existing \
      --file "/tmp/${CLUSTER_NAME}.kubeconfig"
    
    # Label all nodes with workload-type and nic-capacity labels
    echo "==> Labeling all nodes in $CLUSTER_NAME with workload-type=swiftv2-linux"
    kubectl --kubeconfig "/tmp/${CLUSTER_NAME}.kubeconfig" label nodes --all workload-type=swiftv2-linux --overwrite
    echo "[OK] All nodes labeled with workload-type=swiftv2-linux"
    
    # Label default nodepool (nodepool1) with low-nic capacity
    echo "==> Labeling default nodepool (nodepool1) nodes with nic-capacity=low-nic"
    kubectl --kubeconfig "/tmp/${CLUSTER_NAME}.kubeconfig" label nodes -l agentpool=nodepool1 nic-capacity=low-nic --overwrite
    echo "[OK] Default nodepool nodes labeled with nic-capacity=low-nic"
    
    # Label nplinux nodepool with high-nic capacity
    echo "==> Labeling nplinux nodepool nodes with nic-capacity=high-nic"
    kubectl --kubeconfig "/tmp/${CLUSTER_NAME}.kubeconfig" label nodes -l agentpool=nplinux nic-capacity=high-nic --overwrite
    echo "[OK] nplinux nodepool nodes labeled with nic-capacity=high-nic"
done

echo "All clusters complete."
