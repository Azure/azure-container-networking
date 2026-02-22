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
  
  # Export KUBECONFIG so l1vhwindows.sh's internal kubectl commands use the correct cluster
  export KUBECONFIG="${original_dir}/kubeconfig-${cluster_name}.yaml"
  
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

label_single_node() {
  local kubeconfig_file=$1
  local node_name=$2
  local nic_label=$3

  echo "Applying labels to node ${node_name} immediately after join..."

  # Get a managed node as source for podnetwork labels
  local source_node
  source_node=$(kubectl --kubeconfig "$kubeconfig_file" get nodes --selector='!kubernetes.azure.com/managed' -o jsonpath='{.items[0].metadata.name}')

  # Find the actual k8s node name (VMSS name + instance suffix like 000000)
  local k8s_nodes
  k8s_nodes=($(kubectl --kubeconfig "$kubeconfig_file" get nodes -o jsonpath='{.items[*].metadata.name}' | tr ' ' '\n' | grep "^${node_name}" || true))

  if [[ ${#k8s_nodes[@]} -eq 0 ]]; then
    echo "Warning: No nodes found matching ${node_name}, skipping immediate labeling"
    return
  fi

  local LABEL_KEYS=(
    "kubernetes.azure.com/podnetwork-type"
    "kubernetes.azure.com/podnetwork-subscription"
    "kubernetes.azure.com/podnetwork-resourcegroup"
    "kubernetes.azure.com/podnetwork-name"
    "kubernetes.azure.com/podnetwork-subnet"
    "kubernetes.azure.com/podnetwork-multi-tenancy-enabled"
    "kubernetes.azure.com/podnetwork-delegationguid"
    "kubernetes.azure.com/podnetwork-swiftv2-enabled"
    "kubernetes.azure.com/cluster"
  )

  for k8s_node in "${k8s_nodes[@]}"; do
    echo "Labeling $k8s_node with workload-type and nic-capacity..."
    kubectl --kubeconfig "$kubeconfig_file" label node "$k8s_node" \
      "workload-type=swiftv2-l1vh-accelnet-byon" \
      "nic-capacity=${nic_label}" \
      --overwrite

    echo "Copying podnetwork labels from $source_node to $k8s_node..."
    for label_key in "${LABEL_KEYS[@]}"; do
      local escaped_key
      escaped_key=$(echo "$label_key" | sed 's/\//\\\//g; s/\./\\./g')
      local val
      val=$(kubectl --kubeconfig "$kubeconfig_file" get node "$source_node" -o jsonpath="{.metadata.labels['${escaped_key}']}")
      if [[ -n "$val" ]]; then
        kubectl --kubeconfig "$kubeconfig_file" label node "$k8s_node" "${label_key}=${val}" --overwrite
      fi
    done
    echo "[OK] Labels applied to $k8s_node"
  done
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
    # Label node immediately so CNS/NNC can start configuring it while other VMSSes are being created
    nic_label="high-nic"
    if [[ "$base_node_name" == *"acld"* ]]; then
      nic_label="low-nic"
    fi
    label_single_node "$kubeconfig_file" "$node_name" "$nic_label"
    tip_offset=$((tip_offset + 1))
  done
  
  # Final verification pass — ensure all labels are correct
  label_vmss_nodes "$cluster_name" "$cluster_prefix"

  bash ${BUILD_SOURCE_DIR}/Networking-Aquarius/.pipelines/singularity-runner/byon/parse.sh -k ./kubeconfig-${cluster_name}.yaml -p ${BUILD_SOURCE_DIR}/Networking-Aquarius/.pipelines/singularity-runner/byon/pws.ps1

  cluster_index=$((cluster_index + 1))
done

echo "VMSS deployment completed successfully for both clusters."