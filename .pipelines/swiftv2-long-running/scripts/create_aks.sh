#!/usr/bin/env bash
set -euo pipefail
trap 'echo "[ERROR] Failed during Resource group or AKS cluster creation." >&2' ERR
SUBSCRIPTION_ID=$1
LOCATION=$2
RG=$3
VM_SKU_DEFAULT=$4
VM_SKU_HIGHNIC=$5
DELEGATOR_APP_NAME=$6
DELEGATOR_RG=$7
DELEGATOR_SUB=$8
DELEGATOR_BASE_URL=${9:-"http://localhost:8080"}

CLUSTER_COUNT=2
PODS_PER_NODE=7
CLUSTER_PREFIX="aks"


stamp_vnet() {
    local vnet_id="$1"

    responseFile="response.txt"
    modified_vnet="${vnet_id//\//%2F}"
    cmd_stamp_curl="'curl -v -X PUT ${DELEGATOR_BASE_URL}/VirtualNetwork/$modified_vnet/stampcreatorservicename'"
    cmd_containerapp_exec="az containerapp exec -n $DELEGATOR_APP_NAME -g $DELEGATOR_RG --subscription $DELEGATOR_SUB --command $cmd_stamp_curl"
    
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
  local max_attempts=40
  local attempt=0
  
  while [[ $attempt -lt $max_attempts ]]; do
    state=$(az aks show --resource-group "$rg" --name "$clusterName" --query provisioningState -o tsv 2>/dev/null || true)
    echo "Attempt $((attempt+1))/$max_attempts - Provisioning state: $state"
    
    if [[ "$state" =~ Succeeded ]]; then
      echo "Provisioning succeeded"
      return 0
    fi
    if [[ "$state" =~ Failed|Canceled ]]; then
      echo "Provisioning finished with state: $state"
      return 1
    fi
    
    attempt=$((attempt+1))
    sleep 15
  done
  
  echo "Timeout waiting for AKS cluster provisioning after $((max_attempts * 15)) seconds"
  return 1
}

for i in $(seq 1 "$CLUSTER_COUNT"); do
    echo "Creating cluster #$i..."

    CLUSTER_NAME="${CLUSTER_PREFIX}-${i}"

    make -C ./hack/aks azcfg AZCLI=az REGION=$LOCATION
    make -C ./hack/aks swiftv2-podsubnet-cluster-up \
      AZCLI=az REGION=$LOCATION \
      SUB=$SUBSCRIPTION_ID \
      GROUP=$RG \
      CLUSTER=$CLUSTER_NAME \
      VM_SIZE=$VM_SKU_DEFAULT
    wait_for_provisioning "$RG" "$CLUSTER_NAME"

    vnet_id=$(az network vnet show -g "$RG" --name "$CLUSTER_NAME" --query id -o tsv)
    stamp_vnet "$vnet_id"

    make -C ./hack/aks linux-swiftv2-nodepool-up \
      AZCLI=az REGION=$LOCATION \
      GROUP=$RG \
      VM_SIZE=$VM_SKU_HIGHNIC \
      PODS_PER_NODE=$PODS_PER_NODE \
      CLUSTER=$CLUSTER_NAME \
      SUB=$SUBSCRIPTION_ID

    az aks get-credentials -g "$RG" -n "$CLUSTER_NAME" --admin --overwrite-existing \
      --file "/tmp/${CLUSTER_NAME}.kubeconfig"
    
    echo "Labeling all nodes in $CLUSTER_NAME with workload-type=swiftv2-linux"
    kubectl --kubeconfig "/tmp/${CLUSTER_NAME}.kubeconfig" label nodes --all workload-type=swiftv2-linux --overwrite
    
    echo "Labeling default nodepool (nodepool1) nodes with nic-capacity=low-nic"
    kubectl --kubeconfig "/tmp/${CLUSTER_NAME}.kubeconfig" label nodes -l agentpool=nodepool1 nic-capacity=low-nic --overwrite
    
    echo "Labeling nplinux nodepool nodes with nic-capacity=high-nic"
    kubectl --kubeconfig "/tmp/${CLUSTER_NAME}.kubeconfig" label nodes -l agentpool=nplinux nic-capacity=high-nic --overwrite
done

echo "All clusters complete."
