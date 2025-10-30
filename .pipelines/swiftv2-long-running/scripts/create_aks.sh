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
DEFAULT_NODE_COUNT=1                               
COMMON_TAGS="fastpathenabled=true RGOwner=LongRunningTestPipelines stampcreatorserviceinfo=true"

wait_for_provisioning() {                      # Helper for safe retry/wait for provisioning states (basic)
  local rg="$1" clusterName="$2"                     
  echo "Waiting for AKS '$clusterName' in RG '$rg' to reach Succeeded/Failed (polling)..."
  while :; do
    state=$(az aks show --resource-group "$rg" --name "$clusterName" --query provisioningState -o tsv 2>/dev/null || true)
    if [ -z "$state" ]; then
      sleep 3
      continue
    fi
    case "$state" in
      Succeeded|Succeeded*) echo "Provisioning state: $state"; break ;;
      Failed|Canceled|Rejected) echo "Provisioning finished with state: $state"; break ;;
      *) printf "."; sleep 6 ;;
    esac
  done
}


for i in $(seq 1 "$CLUSTER_COUNT"); do
  echo "=============================="
  echo " Working on cluster set #$i"
  echo "=============================="
  
  CLUSTER_NAME="${CLUSTER_PREFIX}-${i}"
  echo "Creating AKS cluster '$CLUSTER_NAME' in RG '$RG'"

  make -C ./hack/aks azcfg AZCLI=az REGION=$LOCATION

  make -C ./hack/aks swiftv2-podsubnet-cluster-up \
  AZCLI=az REGION=$LOCATION \
  SUB=$SUBSCRIPTION_ID \
  GROUP=$RG \
  CLUSTER=$CLUSTER_NAME \
  NODE_COUNT=$DEFAULT_NODE_COUNT \
  VM_SIZE=$VM_SKU_DEFAULT \

  echo " - waiting for AKS provisioning state..."
  wait_for_provisioning "$RG" "$CLUSTER_NAME"

  echo "Adding multi-tenant nodepool ' to '$CLUSTER_NAME'"
  make -C ./hack/aks linux-swiftv2-nodepool-up \
  AZCLI=az REGION=$LOCATION \
  GROUP=$RG \
  VM_SIZE=$VM_SKU_HIGHNIC \
  CLUSTER=$CLUSTER_NAME \
  SUB=$SUBSCRIPTION_ID \

done
echo "All done. Created $CLUSTER_COUNT cluster set(s)."
