#!/bin/bash

check_vmss_exists() {
  local resource_group=$1
  local vmss_name=$2
  
  echo "Checking status for VMSS '${vmss_name}'"
  local node_exists
  node_exists=$(az vmss show --resource-group "$resource_group" --name "$vmss_name" --query "name" -o tsv 2>/dev/null)
  if [[ -z "$node_exists" ]]; then
    echo "##vso[task.logissue type=error]VMSS '$vmss_name' does not exist."
    return 1
  else
    echo "Successfully verified VMSS exists: $vmss_name"
    return 0
  fi
}

check_kubeconfig_exists_in_keyvault() {
  local keyvault_name=$1
  local secret_name=$2
  local subscription=$3
  
  echo "Checking if kubeconfig secret '${secret_name}' exists in Key Vault '${keyvault_name}'"
  local secret_exists
  secret_exists=$(az keyvault secret show \
    --vault-name "$keyvault_name" \
    --name "$secret_name" \
    --subscription "$subscription" \
    --query "id" -o tsv 2>/dev/null || echo "")
  
  if [[ -z "$secret_exists" ]]; then
    echo "Kubeconfig secret '$secret_name' does not exist in Key Vault"
    return 1
  else
    echo "Kubeconfig secret '$secret_name' exists in Key Vault"
    return 0
  fi
}

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

check_if_nodes_joined_cluster() {
  local cluster_name=$1
  local node_name=$2
  local kubeconfig_file=$3
  local expected_nodes=$4
  
  echo "Checking if nodes from VMSS '${node_name}' have joined cluster..."
  
  for ((retry=1; retry<=15; retry++)); do
    nodes=($(kubectl --kubeconfig "$kubeconfig_file" get nodes -o jsonpath='{.items[*].metadata.name}' | tr ' ' '\n' | grep "^${node_name}" || true))
    echo "Found ${#nodes[@]} nodes: ${nodes[*]}"
    
    if [ ${#nodes[@]} -ge $expected_nodes ]; then
      echo "Found ${#nodes[@]} nodes from VMSS ${node_name}: ${nodes[*]}"
      return 0
    else
      if [ $retry -eq 15 ]; then
        echo "##vso[task.logissue type=error]Timeout waiting for nodes from VMSS ${node_name} to join the cluster"
        kubectl --kubeconfig "$kubeconfig_file" get nodes -o wide || true
        return 1
      fi
      echo "Retry $retry: Waiting for nodes to join... (${#nodes[@]}/$expected_nodes joined)"
      sleep 30
    fi
  done
}

wait_for_nodes_ready() {
  local cluster_name=$1
  local node_name=$2
  local kubeconfig_file="./kubeconfig-${cluster_name}.yaml"
  local expected_nodes=$3
  
  echo "Waiting for nodes from VMSS '${node_name}' to join cluster and become ready..."
  if ! check_if_nodes_joined_cluster "$cluster_name" "$node_name" "$kubeconfig_file" "$expected_nodes"; then
    exit 1
  fi

  echo "Checking if nodes are ready..."
  for ((ready_retry=1; ready_retry<=7; ready_retry++)); do
    echo "Ready check attempt $ready_retry of 7"
    all_ready=true
    
    for nodename in "${nodes[@]}"; do
      ready=$(kubectl --kubeconfig "$kubeconfig_file" get node "$nodename" -o jsonpath='{.status.conditions[?(@.type=="Ready")].status}' 2>/dev/null || echo "False")
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

get_ssh_public_key() {
  local secret_name=$1
  local keyvault_name=$2
  local subscription=$3
  
  echo "Fetching SSH public key from Key Vault..." >&2
  local ssh_public_key
  ssh_public_key=$(az keyvault secret show \
    --name "$secret_name" \
    --vault-name "$keyvault_name" \
    --subscription "$subscription" \
    --query value -o tsv 2>/dev/null || echo "")

  if [[ -z "$ssh_public_key" ]]; then
    echo "##vso[task.logissue type=error]Failed to retrieve SSH public key from Key Vault" >&2
    exit 1
  fi
  
  echo "$ssh_public_key"
}

copy_managed_node_labels_to_byon() {
  local kubeconfig_file=$1
  
  SOURCE_NODE=$(kubectl --kubeconfig "$kubeconfig_file" get nodes --selector='!kubernetes.azure.com/managed' -o jsonpath='{.items[0].metadata.name}')
  LABEL_KEYS=(
  "kubernetes\.azure\.com\/podnetwork-type"
  "kubernetes\.azure\.com\/podnetwork-subscription"
  "kubernetes\.azure\.com\/podnetwork-resourcegroup"
  "kubernetes\.azure\.com\/podnetwork-name"
  "kubernetes\.azure\.com\/podnetwork-subnet"
  "kubernetes\.azure\.com\/podnetwork-multi-tenancy-enabled"
  "kubernetes\.azure\.com\/podnetwork-delegationguid"
  "kubernetes\.azure\.com\/podnetwork-swiftv2-enabled"
  "kubernetes\.azure\.com\/cluster"
  )
  
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

