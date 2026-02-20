#!/bin/bash
set -e

RESOURCE_GROUP=$1
REGION=$2
BUILD_SOURCE_DIR=$3
SUBSCRIPTION_ID=$(az account show --query id -o tsv)

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
source "${SCRIPT_DIR}/byon_helper.sh"

cluster_names="aks-1 aks-2"
vmss_configs=(
  "aclh1:Internal_GPGen8MMv2_128id:7"
  "aclh2:Internal_GPGen8MMv2_128id:7"
  "acld1:Internal_GPGen8MMv2_128id:2"
  "acld2:Internal_GPGen8MMv2_128id:2"
)

create_l1vh_vmss() {
  local cluster_name=$1
  local node_name=$2
  local vmss_sku=$3
  local nic_count=$4
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
  local cluster_prefix=$2
  local kubeconfig_file="./kubeconfig-${cluster_name}.yaml"
  
  echo "Labeling BYON nodes in ${cluster_name} with workload-type=swiftv2-l1vh-accelnet-byon"
  kubectl --kubeconfig "$kubeconfig_file" label nodes -l kubernetes.azure.com/managed=false,kubernetes.io/os=windows workload-type=swiftv2-l1vh-accelnet-byon --overwrite

  echo "Labeling ${cluster_prefix}acld nodes with nic-capacity=low-nic"
  kubectl --kubeconfig "$kubeconfig_file" get nodes -o name | grep "${cluster_prefix}acld" | xargs -I {} kubectl --kubeconfig "$kubeconfig_file" label {} nic-capacity=low-nic --overwrite || true

  echo "Labeling ${cluster_prefix}aclh nodes with nic-capacity=high-nic"
  kubectl --kubeconfig "$kubeconfig_file" get nodes -o name | grep "${cluster_prefix}aclh" | xargs -I {} kubectl --kubeconfig "$kubeconfig_file" label {} nic-capacity=high-nic --overwrite || true
  
  copy_managed_node_labels_to_byon "$kubeconfig_file"

  # Verify that critical labels were applied to all BYON Windows nodes
  echo "Verifying labels on BYON Windows nodes..."
  local missing_labels=false
  local byon_nodes=($(kubectl --kubeconfig "$kubeconfig_file" get nodes -l kubernetes.azure.com/managed=false,kubernetes.io/os=windows -o jsonpath='{.items[*].metadata.name}'))
  for node in "${byon_nodes[@]}"; do
    cluster_label=$(kubectl --kubeconfig "$kubeconfig_file" get node "$node" -o jsonpath='{.metadata.labels.kubernetes\.azure\.com/cluster}' 2>/dev/null)
    mt_label=$(kubectl --kubeconfig "$kubeconfig_file" get node "$node" -o jsonpath='{.metadata.labels.kubernetes\.azure\.com/podnetwork-multi-tenancy-enabled}' 2>/dev/null)
    if [[ -z "$cluster_label" || -z "$mt_label" ]]; then
      echo "##vso[task.logissue type=error]Node $node is missing critical labels (cluster='$cluster_label', multi-tenancy='$mt_label')"
      missing_labels=true
    else
      echo "[OK] Node $node has cluster and podnetwork labels"
    fi
  done
  if [[ "$missing_labels" == "true" ]]; then
    echo "##vso[task.logissue type=error]Some BYON nodes are missing critical labels. NodeInfo/NNC will not be created."
    exit 1
  fi
}

cluster_index=0
# Define cluster prefixes for unique VMSS naming (a1 for aks-1, a2 for aks-2)
declare -A cluster_prefixes=( ["aks-1"]="a1" ["aks-2"]="a2" )

for cluster_name in $cluster_names; do
  az identity create --name "aksbootstrap" --resource-group $RESOURCE_GROUP
  az aks get-credentials --resource-group $RESOURCE_GROUP --name $cluster_name --file ./kubeconfig-${cluster_name}.yaml --overwrite-existing -a || exit 1
  
  upload_kubeconfig "$cluster_name"
  bash ${BUILD_SOURCE_DIR}/Networking-Aquarius/.pipelines/singularity-runner/byon/parse.sh -k ./kubeconfig-${cluster_name}.yaml -p ${BUILD_SOURCE_DIR}/Networking-Aquarius/.pipelines/singularity-runner/byon/pws.ps1
  
  echo "Applying RuntimeClass for cluster $cluster_name"
  kubectl apply -f "${SCRIPT_DIR}/runclass.yaml" --kubeconfig "./kubeconfig-${cluster_name}.yaml" || exit 1
  
  echo "Creating L1VH Accelnet BYON for cluster: $cluster_name"
  tip_base_index=$((cluster_index * 4))
  tip_offset=0
  cluster_prefix="${cluster_prefixes[$cluster_name]}"
  
  for config in "${vmss_configs[@]}"; do
    IFS=':' read -r base_node_name vmss_sku nic_count <<< "$config"
    node_name="${cluster_prefix}${base_node_name}"
    echo "Creating VMSS: $node_name with SKU: $vmss_sku, NICs: $nic_count"
    create_l1vh_vmss "$cluster_name" "$node_name" "$vmss_sku" "$nic_count"
    # Wait for node to join cluster (but not Ready — nodes need labels first for CNS/NNC setup)
    kubeconfig_file="./kubeconfig-${cluster_name}.yaml"
    if ! check_if_nodes_joined_cluster "$cluster_name" "$node_name" "$kubeconfig_file" "1"; then
      echo "##vso[task.logissue type=error]Node $node_name did not join the cluster"
      exit 1
    fi
    tip_offset=$((tip_offset + 1))
  done
  
  # Label nodes first — CNS/NNC require labels like kubernetes.azure.com/cluster
  # and podnetwork-multi-tenancy-enabled before they configure the node
  label_vmss_nodes "$cluster_name" "$cluster_prefix"

  bash ${BUILD_SOURCE_DIR}/Networking-Aquarius/.pipelines/singularity-runner/byon/parse.sh -k ./kubeconfig-${cluster_name}.yaml -p ${BUILD_SOURCE_DIR}/Networking-Aquarius/.pipelines/singularity-runner/byon/pws.ps1

  cluster_index=$((cluster_index + 1))
done

echo "VMSS deployment completed successfully for both clusters."