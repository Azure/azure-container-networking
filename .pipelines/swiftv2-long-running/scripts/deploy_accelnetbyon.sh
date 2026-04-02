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
  local keep_nat=${5:-false}
  local original_dir=$(pwd)
  local log_file="${original_dir}/l1vh-script-${node_name}.log"

  echo "Calling l1vhwindows.sh for $node_name (keep_nat=$keep_nat)..."
  set +e
  
  # Change to Networking-Aquarius directory so relative paths work
  pushd ${BUILD_SOURCE_DIR}/Networking-Aquarius > /dev/null
  
  # Export KUBECONFIG so l1vhwindows.sh's internal kubectl commands use the correct cluster
  export KUBECONFIG="${original_dir}/kubeconfig-${cluster_name}.yaml"
  
  local keep_nat_flag=""
  if [[ "$keep_nat" == "true" ]]; then
    keep_nat_flag="-k"
  fi

  bash .pipelines/singularity-runner/byon/l1vhwindows.sh \
    -l $REGION \
    -r $RESOURCE_GROUP \
    -s $SUBSCRIPTION_ID \
    -v "$node_name" \
    -e "nodenet" \
    -n "$RESOURCE_GROUP" \
    -i "$cluster_name" \
    -z "$vmss_sku" \
    -y "${L1VH_KEYVAULT_NAME}" \
    -q "${L1VH_VMSS_PASSWORD_SECRET}" \
    -p "${L1VH_VM_BICEP_PASSWORD_SECRET}" \
    -x "${L1VH_STORAGE_SECRET}" \
    -d $nic_count \
    $keep_nat_flag \
    2>&1 | tee "$log_file"
  local exit_code=${PIPESTATUS[0]}
  
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
  local source_node
  source_node=$(kubectl --kubeconfig "$kubeconfig_file" get nodes --selector='!kubernetes.azure.com/managed' -o jsonpath='{.items[0].metadata.name}')
  local k8s_nodes
  k8s_nodes=($(kubectl --kubeconfig "$kubeconfig_file" get nodes -o jsonpath='{.items[*].metadata.name}' | tr ' ' '\n' | grep "^${node_name}" || true))

  if [[ ${#k8s_nodes[@]} -eq 0 ]]; then
    echo "Warning: No nodes found matching ${node_name}, skipping immediate labeling"
    return
  fi

  for k8s_node in "${k8s_nodes[@]}"; do
    echo "Labeling $k8s_node with workload-type and nic-capacity..."
    kubectl --kubeconfig "$kubeconfig_file" label node "$k8s_node" \
      "workload-type=swiftv2-l1vh-accelnet-byon" \
      "nic-capacity=${nic_label}" \
      --overwrite

    echo "Copying podnetwork labels from managed nodes to accelnet node $k8s_node..."
    copy_podnetwork_labels_to_node "$kubeconfig_file" "$source_node" "$k8s_node"
    echo "[OK] Labels applied to $k8s_node"
  done
}

declare -A cluster_prefixes=( ["aks-1"]="a1" ["aks-2"]="a2" )

for cluster_name in $cluster_names; do
  az identity show --name "aksbootstrap" --resource-group "$RESOURCE_GROUP" &>/dev/null || \
    az identity create --name "aksbootstrap" --resource-group "$RESOURCE_GROUP"
  az aks get-credentials --resource-group $RESOURCE_GROUP --name $cluster_name --file ./kubeconfig-${cluster_name}.yaml --overwrite-existing -a || exit 1
  
  upload_kubeconfig "$cluster_name"
  bash ${BUILD_SOURCE_DIR}/Networking-Aquarius/.pipelines/singularity-runner/byon/parse.sh -k ./kubeconfig-${cluster_name}.yaml -p ${BUILD_SOURCE_DIR}/Networking-Aquarius/.pipelines/singularity-runner/byon/pws.ps1
  echo "Applying RuntimeClass for cluster $cluster_name"
  kubectl apply -f "${SCRIPT_DIR}/runclass.yaml" --kubeconfig "./kubeconfig-${cluster_name}.yaml" || exit 1
  
  echo "Creating L1VH Accelnet BYON for cluster: $cluster_name"
  cluster_prefix="${cluster_prefixes[$cluster_name]}"
  
  total_configs=${#vmss_configs[@]}
  config_index=0
  for config in "${vmss_configs[@]}"; do
    IFS=':' read -r base_node_name vmss_sku nic_count <<< "$config"
    node_name="${cluster_prefix}${base_node_name}"
    config_index=$((config_index + 1))
    keep_nat="false"
    if [[ $config_index -eq $total_configs ]]; then
      keep_nat="true" # Pass -k (keep NAT) only on the last VMSS so NAT gateway stays attached for outbound connectivity
    fi
    echo "Creating VMSS: $node_name with SKU: $vmss_sku, NICs: $nic_count (keep_nat=$keep_nat)"
    # Skip creation if VMSS already exists and node is already in the cluster
    kubeconfig_file="./kubeconfig-${cluster_name}.yaml"
    if check_vmss_exists "$RESOURCE_GROUP" "$node_name" 2>/dev/null; then
      echo "VMSS '$node_name' already exists, checking if node joined cluster..."
      set +e
      existing_nodes=($(kubectl --kubeconfig "$kubeconfig_file" get nodes -o jsonpath='{.items[*].metadata.name}' | tr ' ' '\n' | grep "^${node_name}" 2>/dev/null))
      set -e
      if [[ ${#existing_nodes[@]} -ge 1 ]]; then
        echo "Node '$node_name' already in cluster (${#existing_nodes[@]} node(s) found). Skipping VMSS creation."
      else
        echo "VMSS exists but node not in cluster. Re-creating VMSS..."
        create_l1vh_vmss "$cluster_name" "$node_name" "$vmss_sku" "$nic_count" "$keep_nat"
        if ! check_if_nodes_joined_cluster "$cluster_name" "$node_name" "$kubeconfig_file" "1"; then
          echo "##vso[task.logissue type=error]Node $node_name did not join the cluster"
          exit 1
        fi
      fi
    else
      create_l1vh_vmss "$cluster_name" "$node_name" "$vmss_sku" "$nic_count" "$keep_nat"
      # Wait for node to join cluster (but not Ready — nodes need labels first for CNS/NNC setup)
      if ! check_if_nodes_joined_cluster "$cluster_name" "$node_name" "$kubeconfig_file" "1"; then
        echo "##vso[task.logissue type=error]Node $node_name did not join the cluster"
        exit 1
      fi
    fi
    
    nic_label="high-nic"
    if [[ "$base_node_name" == *"acld"* ]]; then
      nic_label="low-nic"
    fi
    label_single_node "$kubeconfig_file" "$node_name" "$nic_label"
  done
  
  bash ${BUILD_SOURCE_DIR}/Networking-Aquarius/.pipelines/singularity-runner/byon/parse.sh -k ./kubeconfig-${cluster_name}.yaml -p ${BUILD_SOURCE_DIR}/Networking-Aquarius/.pipelines/singularity-runner/byon/pws.ps1
done

echo "VMSS deployment completed successfully for both clusters."