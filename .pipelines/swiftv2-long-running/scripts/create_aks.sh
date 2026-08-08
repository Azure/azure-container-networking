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
STALE_NODE_THRESHOLD=1800  # seconds a Node may stay unreachable and non-Ready before it is considered unrecoverable

echo "Setting active subscription to $SUBSCRIPTION_ID"
az account set --subscription "$SUBSCRIPTION_ID"


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

# "kubectl wait --all" can never be satisfied by a Node whose kubelet has stopped
# reporting, so it burns its full timeout and then fails without saying why. Report
# those nodes up front and fail immediately: a node unreachable this long needs its
# backing VM repaired, and no amount of waiting will change that.
#
# Deliberately NOT deleting these Node objects. A stale Node usually means the VM
# behind it is deallocated or its scale set failed to provision, so removing the
# object would let the readiness gate pass while the cluster is silently short of
# nodes. Fail loudly and let the VM be repaired instead.
check_unreachable_nodes() {
  local kubeconfig="$1" clusterName="$2"
  local now unreachable

  now=$(date -u +%s)
  unreachable=$(kubectl --kubeconfig "$kubeconfig" get nodes -o json \
    | jq -r --argjson now "$now" --argjson threshold "$STALE_NODE_THRESHOLD" '
        .items[]
        | select(.spec.taints // [] | any(.key == "node.kubernetes.io/unreachable"))
        | .metadata.name as $name
        | .status.conditions[]
        | select(.type == "Ready")
        | select(.status != "True" and (.lastTransitionTime | fromdateiso8601) < ($now - $threshold))
        | "  - \($name) (Ready=\(.status) since \(.lastTransitionTime))"')

  if [[ -z "$unreachable" ]]; then
    return 0
  fi

  echo "##vso[task.logissue type=error]Cluster $clusterName has nodes that have been unreachable and non-Ready for more than ${STALE_NODE_THRESHOLD}s:"
  echo "$unreachable"
  echo "They cannot become Ready, so waiting for node readiness would only time out."
  echo "Check the backing scale sets for a failed provisioningState or missing instances, repair them, then re-run."
  return 1
}

for i in $(seq 1 "$CLUSTER_COUNT"); do
    echo "Creating cluster #$i..."

    CLUSTER_NAME="${CLUSTER_PREFIX}-${i}"

    # Check if cluster already exists and is healthy
    EXISTING_STATE=$(az aks show -g "$RG" -n "$CLUSTER_NAME" --query provisioningState -o tsv 2>/dev/null || true)
    if [[ "$EXISTING_STATE" == "Succeeded" ]]; then
      echo "Cluster $CLUSTER_NAME already exists (state: $EXISTING_STATE). Skipping creation."
    else
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
    fi

    # Add high-NIC nodepool if it doesn't exist
    NPLINUX_EXISTS=$(az aks nodepool show -g "$RG" --cluster-name "$CLUSTER_NAME" -n nplinux --query provisioningState -o tsv 2>/dev/null || true)
    if [[ -n "$NPLINUX_EXISTS" ]]; then
      echo "Nodepool nplinux already exists on $CLUSTER_NAME (state: $NPLINUX_EXISTS). Skipping."
    else
      make -C ./hack/aks linux-swiftv2-nodepool-up \
        AZCLI=az REGION=$LOCATION \
        GROUP=$RG \
        VM_SIZE=$VM_SKU_HIGHNIC \
        PODS_PER_NODE=$PODS_PER_NODE \
        CLUSTER=$CLUSTER_NAME \
        SUB=$SUBSCRIPTION_ID
    fi

    az aks get-credentials -g "$RG" -n "$CLUSTER_NAME" --admin --overwrite-existing \
      --file "/tmp/${CLUSTER_NAME}.kubeconfig"
    
    check_unreachable_nodes "/tmp/${CLUSTER_NAME}.kubeconfig" "$CLUSTER_NAME"

    echo "Waiting for all nodes in $CLUSTER_NAME to be Ready..."
    kubectl --kubeconfig "/tmp/${CLUSTER_NAME}.kubeconfig" wait --for=condition=Ready nodes --all --timeout=10m
done

echo "All clusters complete."
