#!/bin/bash
set -e

RESOURCE_GROUP=$1
BUILD_SOURCE_DIR=$2
BICEP_TEMPLATE_PATH="${BUILD_SOURCE_DIR}/Networking-Aquarius/.pipelines/singularity-runner/byon/linux.bicep"

upload_kubeconfig() {
  local cluster_name=$1
  local kubeconfig_file="./kubeconfig-${cluster_name}"
  local secret_name="${RESOURCE_GROUP}-${cluster_name}-kubeconfig"

  echo "Fetching AKS credentials for cluster: ${cluster_name}"
  az aks get-credentials \
    --resource-group "$RESOURCE_GROUP" \
    --name "$cluster_name" \
    --file "$kubeconfig_file" \
    --overwrite-existing

  echo "Storing kubeconfig for ${cluster_name} in Azure Key Vault..."
  if [[ -f "$kubeconfig_file" ]]; then
    az keyvault secret set \
      --vault-name "$CLUSTER_KUBECONFIG_KEYVAULT_NAME" \
      --name "$secret_name" \
      --value "$(cat "$kubeconfig_file")" \
      --subscription "$KEY_VAULT_SUBSCRIPTION" \
      >> /dev/null

    if [[ $? -eq 0 ]]; then
      echo "Successfully stored kubeconfig in Key Vault secret: $secret_name"
    else
      echo "##vso[task.logissue type=error]Failed to store kubeconfig for ${cluster_name} in Key Vault"
      exit 1
    fi
  else
    echo "##vso[task.logissue type=error]Kubeconfig file not found at: $kubeconfig_file"
    exit 1
  fi
}

create_and_check_vmss() {
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

  echo "Checking status for VMSS '${node_name}'"
  local node_exists
  node_exists=$(az vmss show --resource-group "$RESOURCE_GROUP" --name "$node_name" --query "name" -o tsv 2>/dev/null)
  if [[ -z "$node_exists" ]]; then
    echo "##vso[task.logissue type=error]VMSS '$node_name' does not exist."
    exit 1
  else
    echo "Successfully created VMSS: $node_name"
  fi
}

wait_for_nodes_ready() {
  local cluster_name=$1
  local node_name=$2
  local kubeconfig_file="./kubeconfig-${cluster_name}"
  
  echo "Waiting for nodes from VMSS '${node_name}' to join cluster and become ready..."
  local expected_nodes=2
  
  # Check if BYO nodes have joined cluster using VMSS name label
  for ((retry=1; retry<=15; retry++)); do
    nodes=($(kubectl --kubeconfig "$kubeconfig_file" get nodes -o jsonpath='{.items[*].metadata.name}' | tr ' ' '\n' | grep "^${node_name}" || true))
    echo "Found ${#nodes[@]} nodes: ${nodes[*]}"
    
    if [ ${#nodes[@]} -ge $expected_nodes ]; then
      echo "Found ${#nodes[@]} nodes from VMSS ${node_name}: ${nodes[*]}"
      break
    else
      if [ $retry -eq 15 ]; then
        echo "##vso[task.logissue type=error]Timeout waiting for nodes from VMSS ${node_name} to join the cluster"
        kubectl --kubeconfig "$kubeconfig_file" get nodes -o wide || true
        exit 1
      fi
      echo "Retry $retry: Waiting for nodes to join... (${#nodes[@]}/$expected_nodes joined)"
      sleep 30
    fi
  done

  echo "Checking if nodes are ready..."
  for ((ready_retry=1; ready_retry<=7; ready_retry++)); do
    echo "Ready check attempt $ready_retry of 7"
    all_ready=true
    
    for nodename in "${nodes[@]}"; do
      ready=$(kubectl --kubeconfig "./kubeconfig-${cluster_name}" get node "$nodename" -o jsonpath='{.status.conditions[?(@.type=="Ready")].status}' 2>/dev/null || echo "False")
      if [ "$ready" != "True" ]; then
        echo "Node $nodename is not ready yet (status: $ready)"
        all_ready=false
      else
        echo "Node $nodename is ready"
      fi
    done
    
    if [ "$all_ready" = true ]; then
      echo "All nodes from VMSS ${node_name} are ready"
      break
    else
      if [ $ready_retry -eq 7 ]; then
        echo "##vso[task.logissue type=error]Timeout: Nodes from VMSS ${node_name} are not ready after 7 attempts"
        kubectl --kubeconfig "$kubeconfig_file" get nodes -o wide || true
        exit 1
      fi
      echo "Waiting 30 seconds before retry..."
      sleep 30
    fi
  done
}

label_vmss_nodes() {
  local cluster_name=$1
  local kubeconfig_file="./kubeconfig-${cluster_name}"
  
  echo "Labeling BYON nodes in ${cluster_name} with workload-type=swiftv2-linux-byon"
  kubectl --kubeconfig "$kubeconfig_file" label nodes -l kubernetes.azure.com/managed=false workload-type=swiftv2-linux-byon --overwrite

  echo "Labeling ${cluster_name}-linux-default nodes with nic-capacity=low-nic"
  kubectl --kubeconfig "$kubeconfig_file" get nodes -o name | grep "${cluster_name}-linux-default" | xargs -I {} kubectl --kubeconfig "$kubeconfig_file" label {} nic-capacity=low-nic --overwrite || true

  echo "Labeling ${cluster_name}-linux-highnic nodes with nic-capacity=high-nic"
  kubectl --kubeconfig "$kubeconfig_file" get nodes -o name | grep "${cluster_name}-linux-highnic" | xargs -I {} kubectl --kubeconfig "$kubeconfig_file" label {} nic-capacity=high-nic --overwrite || true
  
  SOURCE_NODE=$(kubectl --kubeconfig "$kubeconfig_file" get nodes --selector='!kubernetes.azure.com/managed' -o jsonpath='{.items[0].metadata.name}')
  
  if [ -z "$SOURCE_NODE" ]; then
    echo "Error: No BYON nodes found to use as source for label copying"
    exit 1
  fi
  
  echo "Using node $SOURCE_NODE as source for label copying"
  
  LABEL_KEYS=(
  "kubernetes\.azure\.com\/podnetwork-type"
  "kubernetes\.azure\.com\/podnetwork-subscription"
  "kubernetes\.azure\.com\/podnetwork-resourcegroup"
  "kubernetes\.azure\.com\/podnetwork-name"
  "kubernetes\.azure\.com\/podnetwork-subnet"
  "kubernetes\.azure\.com\/podnetwork-multi-tenancy-enabled"
  "kubernetes\.azure\.com\/podnetwork-delegationguid"
  "kubernetes\.azure\.com\/cluster")
  
  nodes=($(kubectl --kubeconfig "$kubeconfig_file" get nodes -l kubernetes.azure.com/managed=false -o jsonpath='{.items[*].metadata.name}'))
      
  for NODENAME in "${nodes[@]}"; do
      for label_key in "${LABEL_KEYS[@]}"; do
        v=$(kubectl --kubeconfig "$kubeconfig_file" get nodes "$SOURCE_NODE" -o jsonpath="{.metadata.labels['$label_key']}")
        l=$(echo "$label_key" | sed 's/\\//g')
        echo "Labeling node $NODENAME with $l=$v"
        kubectl --kubeconfig "$kubeconfig_file" label node "$NODENAME" "$l=$v" --overwrite
      done
  done
}

echo "Fetching SSH public key from Key Vault..."
ssh_public_key=$(az keyvault secret show \
  --name "$SSH_PUBLIC_KEY_SECRET_NAME" \
  --vault-name "$CLUSTER_KUBECONFIG_KEYVAULT_NAME" \
  --subscription "$KEY_VAULT_SUBSCRIPTION" \
  --query value -o tsv 2>/dev/null || echo "")

if [[ -z "$ssh_public_key" ]]; then
  echo "##vso[task.logissue type=error]Failed to retrieve SSH public key from Key Vault"
  exit 1
fi

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
  create_and_check_vmss "$cluster_name" "linux-highnic" "Standard_D16s_v3" "7"
  wait_for_nodes_ready "$cluster_name" "$cluster_name-linux-highnic"

  create_and_check_vmss "$cluster_name" "linux-default" "Standard_D8s_v3" "2"
  wait_for_nodes_ready "$cluster_name" "$cluster_name-linux-default"

  label_vmss_nodes "$cluster_name"
done

echo "VMSS deployment completed successfully for both clusters."
