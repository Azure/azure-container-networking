#!/bin/bash
set -e

RESOURCE_GROUP=$1
REGION=$2
BUILD_SOURCE_DIR=$3
SUBSCRIPTION_ID=$(az account show --query id -o tsv)

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
source "${SCRIPT_DIR}/byon_helper.sh"

cluster_names="aks-1 aks-2"
tip_session_ids=(7356a39e-f8e5-40e0-815d-cd53f540d505 7356a39e-f8e5-40e0-815d-cd53f540d505 4aa2ca57-8dd0-4fbd-a961-bae4142dc33c 4aa2ca57-8dd0-4fbd-a961-bae4142dc33c 4e7a03fc-c541-4845-8805-73e45028f171 4e7a03fc-c541-4845-8805-73e45028f171 f64e86da-c832-460b-995f-0e9fe2601d99 f64e86da-c832-460b-995f-0e9fe2601d99)
vmss_configs=(
  "aclhigh1:Standard_D16s_v6:7"
  "aclhigh2:Standard_D16s_v6:7"
  "acldef1:Standard_D4s_v6:2"
  "acldef2:Standard_D4s_v6:2"
)

create_l1vh_vmss() {
  local cluster_name=$1
  local node_name=$2
  local vmss_sku=$3
  local nic_count=$4
  local TIP_ARG1=$5
  local original_dir=$(pwd)
  local log_file="${original_dir}/l1vh-script-${node_name}.log"

  echo "Calling l1vhwindows.sh for $node_name..."
  set +e
  
  # Change to Networking-Aquarius directory so relative paths work
  pushd ${BUILD_SOURCE_DIR}/Networking-Aquarius > /dev/null
  
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
    2>&1 | tee "$log_file"
  local exit_code=$?
  
  popd > /dev/null
  set -e
  
  if [[ $exit_code -ne 0 ]]; then
    echo "##vso[task.logissue type=error]L1VH VMSS creation failed for $node_name with exit code $exit_code"
    echo "Log file contents:"
    cat "$log_file" || true
    exit 1
  fi
  
  echo "L1VH script completed for $node_name"
  check_vmss_exists "$RESOURCE_GROUP" "$node_name" || exit 1
}

label_vmss_nodes() {
  local cluster_name=$1
  local kubeconfig_file="./kubeconfig-${cluster_name}.yaml"
  
  echo "Labeling BYON nodes in ${cluster_name} with workload-type=swiftv2-l1vh-accelnet-byon"
  kubectl --kubeconfig "$kubeconfig_file" label nodes -l kubernetes.azure.com/managed=false,kubernetes.io/os=linux workload-type=swiftv2-l1vh-accelnet-byon --overwrite

  echo "Labeling ${cluster_name}-accelnet-default nodes with nic-capacity=low-nic"
  kubectl --kubeconfig "$kubeconfig_file" get nodes -o name | grep "${cluster_name}-accelnet-default" | xargs -I {} kubectl --kubeconfig "$kubeconfig_file" label {} nic-capacity=low-nic --overwrite || true

  echo "Labeling ${cluster_name}-accelnet-highnic nodes with nic-capacity=high-nic"
  kubectl --kubeconfig "$kubeconfig_file" get nodes -o name | grep "${cluster_name}-accelnet-highnic" | xargs -I {} kubectl --kubeconfig "$kubeconfig_file" label {} nic-capacity=high-nic --overwrite || true
  
  copy_managed_node_labels_to_byon "$kubeconfig_file"
}

cluster_index=0
for cluster_name in $cluster_names; do
  az identity create --name "aksbootstrap" --resource-group $RESOURCE_GROUP
  az aks get-credentials --resource-group $RESOURCE_GROUP --name $cluster_name --file ./kubeconfig-${cluster_name}.yaml --overwrite-existing -a || exit 1
  bash ${BUILD_SOURCE_DIR}/Networking-Aquarius/.pipelines/singularity-runner/byon/parse.sh -k ./kubeconfig-${cluster_name}.yaml -p ${BUILD_SOURCE_DIR}/Networking-Aquarius/.pipelines/singularity-runner/byon/pws.ps1

  echo "Creating L1VH Accelnet BYON for cluster: $cluster_name"
  tip_base_index=$((cluster_index * 4))
  tip_offset=0
  
  for config in "${vmss_configs[@]}"; do
    IFS=':' read -r node_name vmss_sku nic_count <<< "$config"
    tip_index=$((tip_base_index + tip_offset))
    tip_session_id="${tip_session_ids[$tip_index]}"
    echo "Creating VMSS: $node_name with SKU: $vmss_sku, NICs: $nic_count, TIP: $tip_session_id (index: $tip_index)"
    create_l1vh_vmss "$cluster_name" "$node_name" "$vmss_sku" "$nic_count" "$tip_session_id"
    wait_for_nodes_ready "$cluster_name" "$node_name" "1"
    tip_offset=$((tip_offset + 1))
  done
  
  label_vmss_nodes "$cluster_name"
  cluster_index=$((cluster_index + 1))
done

echo "VMSS deployment completed successfully for both clusters."