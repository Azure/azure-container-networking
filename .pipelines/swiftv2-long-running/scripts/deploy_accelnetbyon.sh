#!/bin/bash
set -e

RESOURCE_GROUP=$1
REGION=$2
BUILD_SOURCE_DIR=$3
SUBSCRIPTION_ID=$(az account show --query id -o tsv)
BICEP_TEMPLATE_PATH="${BUILD_SOURCE_DIR}/Networking-Aquarius/.pipelines/singularity-runner/byon/linux.bicep"

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
source "${SCRIPT_DIR}/byon_helper.sh"

cluster_names="aks-1 aks-2"
tip_session_ids=(8fe6f0f9-1476-4c8c-8945-c16300e557a9 8fe6f0f9-1476-4c8c-8945-c16300e557a9 dc72458d-a95d-4913-9deb-e4f4d20ca4dd dc72458d-a95d-4913-9deb-e4f4d20ca4dd 744922f0-1651-409f-9475-18ae34602446 744922f0-1651-409f-9475-18ae34602446 f64e86da-c832-460b-995f-0e9fe2601d99 f64e86da-c832-460b-995f-0e9fe2601d99)

# Define VMSS configurations: node_prefix, sku, nic_count, node_count
vmss_configs=(
  "accelnet-highnic:Standard_D16s_v3:7:2"
  "accelnet-default:Standard_D8s_v3:2:2"
)

cluster_index=0
for cluster_name in $cluster_names; do
  az aks get-credentials --resource-group $RESOURCE_GROUP --name $cluster_name --file ./kubeconfig-${cluster_name}.yaml --overwrite-existing -a || exit 1
  bash .pipelines/singularity-runner/byon/parse.sh -k ./kubeconfig-${cluster_name}.yaml -p .pipelines/singularity-runner/byon/pws.ps1

  echo "Creating L1VH Accelnet BYON for cluster: $cluster_name"
  tip_base_index=$((cluster_index * 4))
  tip_offset=0
  
  for config in "${vmss_configs[@]}"; do
    IFS=':' read -r node_prefix vmss_sku nic_count node_count <<< "$config"
    for ((i=0; i<node_count; i++)); do
      node_name="${cluster_name}-${node_prefix}${i}"
      tip_index=$((tip_base_index + tip_offset))
      tip_session_id="${tip_session_ids[$tip_index]}"
      
      echo "Creating VMSS: $node_name with SKU: $vmss_sku, NICs: $nic_count, TIP: $tip_session_id (index: $tip_index)"
      create_l1vh_vmss "$cluster_name" "$node_name" "$vmss_sku" "$nic_count" "$tip_session_id"
      wait_for_nodes_ready "$cluster_name" "$node_name"
      tip_offset=$((tip_offset + 1))
    done
  done
  
  cluster_index=$((cluster_index + 1))
done


create_l1vh_vmss() {
  local cluster_name=$1
  local node_name=$2
  local vmss_sku=$3
  local nic_count=$4
  local TIP_ARG1=$5

  bash .pipelines/singularity-runner/byon/l1vhwindows.sh \
    -l $REGION \
    -r $RESOURCE_GROUP \
    -s $SUBSCRIPTION_ID \
    -v "$node_name" \
    -e "nodenet" \
    -t "$TIP_ARG1" \
    -n "$RESOURCE_GROUP" \
    -i "$cluster_name" \
    -z "$vmss_sku" \
    -y "singularity-standalone-testing" \
    -q "vmssstandalonepwd" \
    -p "vmbiceppwd" \
    -x "l1vhstandalonestorage" \
    > ./script1.log 2>&1
}

label_vmss_nodes() {
  local cluster_name=$1
  local kubeconfig_file="./kubeconfig-${cluster_name}"
  
  echo "Labeling BYON nodes in ${cluster_name} with workload-type=swiftv2-l1vh-accelnet-byon"
  kubectl --kubeconfig "$kubeconfig_file" label nodes -l kubernetes.azure.com/managed=false,kubernetes.io/os=linux workload-type=swiftv2-l1vh-accelnet-byon --overwrite

  echo "Labeling ${cluster_name}-accelnet-default nodes with nic-capacity=low-nic"
  kubectl --kubeconfig "$kubeconfig_file" get nodes -o name | grep "${cluster_name}-accelnet-default" | xargs -I {} kubectl --kubeconfig "$kubeconfig_file" label {} nic-capacity=low-nic --overwrite || true

  echo "Labeling ${cluster_name}-accelnet-highnic nodes with nic-capacity=high-nic"
  kubectl --kubeconfig "$kubeconfig_file" get nodes -o name | grep "${cluster_name}-accelnet-highnic" | xargs -I {} kubectl --kubeconfig "$kubeconfig_file" label {} nic-capacity=high-nic --overwrite || true
  
  copy_managed_node_labels_to_byon "$kubeconfig_file"
}