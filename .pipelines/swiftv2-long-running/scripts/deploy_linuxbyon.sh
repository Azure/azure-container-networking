#!/bin/bash
set -e

RESOURCE_GROUP=$1
BUILD_SOURCE_DIR=$2
BICEP_TEMPLATE_PATH="${BUILD_SOURCE_DIR}/Networking-Aquarius/.pipelines/singularity-runner/byon/linux.bicep"

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
source "${SCRIPT_DIR}/byon_helper.sh"

create_linux_vmss() {
  local cluster_name=$1
  local node_type=$2
  local vmss_sku=$3
  local nic_count=$4
  local node_name="${cluster_name}-${node_type}"
  local log_file="./lin-script-${node_name}.log"
  local extension_name="NodeJoin-${node_name}"
  local kubeconfig_secret="${RESOURCE_GROUP}-${cluster_name}-kubeconfig"
               
  echo "Creating Linux VMSS Node '${node_name}' for cluster '${cluster_name}'"
  set +e
  az deployment group create -n "sat${node_name}" \
    --resource-group "$RESOURCE_GROUP" \
    --template-file "$BICEP_TEMPLATE_PATH" \
    --parameters vnetname="$cluster_name" \
                subnetname="nodenet" \
                name="$node_name" \
                sshPublicKey="$ssh_public_key" \
                vnetrgname="$RESOURCE_GROUP" \
                extensionName="$extension_name" \
                clusterKubeconfigKeyvaultName="$CLUSTER_KUBECONFIG_KEYVAULT_NAME" \
                clusterKubeconfigSecretName="$kubeconfig_secret" \
                keyVaultSubscription="$KEY_VAULT_SUBSCRIPTION" \
                vmsssku="$vmss_sku" \
                vmsscount=2 \
                delegatedNicsCount="$nic_count" \
    2>&1 | tee "$log_file"
  local deployment_exit_code=$?
  set -e

  if [[ $deployment_exit_code -ne 0 ]]; then
    echo "##vso[task.logissue type=error]Azure deployment failed for VMSS '$node_name' with exit code $deployment_exit_code"
    exit 1
  fi

  check_vmss_exists "$RESOURCE_GROUP" "$node_name" || exit 1
}

label_vmss_nodes() {
  local cluster_name=$1
  local kubeconfig_file="./kubeconfig-${cluster_name}"
  
  echo "Labeling BYON nodes in ${cluster_name} with workload-type=swiftv2-linux-byon"
  kubectl --kubeconfig "$kubeconfig_file" label nodes -l kubernetes.azure.com/managed=false,kubernetes.io/os=linux workload-type=swiftv2-linux-byon --overwrite

  echo "Labeling ${cluster_name}-linux-default nodes with nic-capacity=low-nic"
  kubectl --kubeconfig "$kubeconfig_file" get nodes -o name | grep "${cluster_name}-linux-default" | xargs -I {} kubectl --kubeconfig "$kubeconfig_file" label {} nic-capacity=low-nic --overwrite || true

  echo "Labeling ${cluster_name}-linux-highnic nodes with nic-capacity=high-nic"
  kubectl --kubeconfig "$kubeconfig_file" get nodes -o name | grep "${cluster_name}-linux-highnic" | xargs -I {} kubectl --kubeconfig "$kubeconfig_file" label {} nic-capacity=high-nic --overwrite || true
  
  copy_managed_node_labels_to_byon "$kubeconfig_file"
}

ssh_public_key=$(get_ssh_public_key "$SSH_PUBLIC_KEY_SECRET_NAME" "$CLUSTER_KUBECONFIG_KEYVAULT_NAME" "$KEY_VAULT_SUBSCRIPTION")
cluster_names="aks-1 aks-2"

for cluster_name in $cluster_names; do
  upload_kubeconfig "$cluster_name"
  echo "Installing CNI plugins for cluster $cluster_name"
  if ! helm install -n kube-system azure-cni-plugins ${BUILD_SOURCE_DIR}/Networking-Aquarius/.pipelines/singularity-runner/byon/chart/base \
        --set installCniPlugins.enabled=true \
        --kubeconfig "./kubeconfig-${cluster_name}"; then
    echo "##vso[task.logissue type=error]Failed to install CNI plugins for cluster ${cluster_name}"
    exit 1
  fi
  
  echo "Creating VMSS nodes for cluster $cluster_name..."
  create_linux_vmss "$cluster_name" "linux-highnic" "Standard_D16s_v3" "7"
  wait_for_nodes_ready "$cluster_name" "$cluster_name-linux-highnic"

  create_linux_vmss "$cluster_name" "linux-default" "Standard_D8s_v3" "2"
  wait_for_nodes_ready "$cluster_name" "$cluster_name-linux-default"

  label_vmss_nodes "$cluster_name"
done
echo "VMSS deployment completed successfully for both clusters."
