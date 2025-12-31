#!/bin/bash
set -e

# Arguments
RESOURCE_GROUP=$1

# Environment variables expected from pipeline:
# - SSH_PUBLIC_KEY_SECRET_NAME
# - CLUSTER_KUBECONFIG_KEYVAULT_NAME
# - KEY_VAULT_RESOURCE_GROUP
# - KEY_VAULT_SUBSCRIPTION

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
      --value "$(cat $kubeconfig_file)" \
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
  az deployment group create -n "sat${node_name}" \
    --resource-group "$RESOURCE_GROUP" \
    --template-file $(Build.SourcesDirectory)/Networking-Aquarius/.pipelines/singularity-runner/byon/linux.bicep \
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
    >> "$log_file" 2>&1

  echo "Displaying logs for node ${node_name}:"
  cat "$log_file"

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
  
  echo "Waiting for nodes from VMSS '${node_name}' to join cluster and become ready..."
  local expected_nodes=2
  
  # check if BYO node has joined cluster.
  for ((retry=1; retry<=15; retry++)); do
    nodes=($(kubectl --kubeconfig "./kubeconfig-${cluster_name}" get nodes -l kubernetes.azure.com/vmss-name="${node_name}" -o jsonpath='{.items[*].metadata.name}' 2>/dev/null || echo ""))
    if [ ${#nodes[@]} -ge $expected_nodes ]; then
      echo "Found ${#nodes[@]} nodes from VMSS ${node_name}: ${nodes[*]}"
      break
    else
      if [ $retry -eq 30 ]; then
        echo "##vso[task.logissue type=error]Timeout waiting for nodes from VMSS ${node_name} to join the cluster"
        exit 1
      fi
      echo "Retry $retry: Waiting for nodes to join... (${#nodes[@]}/$expected_nodes joined)"
      sleep 30
    fi
  done


  for nodename in "${nodes[@]}"; do
    ready=$(kubectl --kubeconfig "./kubeconfig-${cluster_name}" get node "$nodename" -o jsonpath='{.status.conditions[?(@.type=="Ready")].status}' 2>/dev/null || echo "False")
    if [ "$ready" != "True" ]; then
      echo "##vso[task.logissue type=error] Node ${node_name} is not ready"
      exit 1
    fi
  done
}

label_vmss_nodes() {
  local cluster_name=$1
  local kubeconfig_file="./kubeconfig-${cluster_name}"
  
  echo "Labeling BYON nodes in ${cluster_name} with workload-type=swiftv2-linux-byon"
  kubectl --kubeconfig "$kubeconfig_file" label nodes -l kubernetes.azure.com/managed=false workload-type=swiftv2-linux-byon --overwrite

  echo "Labeling ${cluster_name} linux-default nodes with nic-capacity=low-nic"
  kubectl --kubeconfig "$kubeconfig_file" label nodes -l kubernetes.azure.com/vmss-name="${cluster_name}-linux-default" nic-capacity=low-nic --overwrite || true

  echo "Labeling ${cluster_name} linux-highnic nodes with nic-capacity=high-nic"
  kubectl --kubeconfig "$kubeconfig_file" label nodes -l kubernetes.azure.com/vmss-name="${cluster_name}-linux-highnic" nic-capacity=high-nic --overwrite || true
  
  SOURCE_NODE=$(kubectl --kubeconfig "$kubeconfig_file" get nodes --selector='!kubernetes.azure.com/managed' -o jsonpath='{.items[0].metadata.name}')
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

check_nnc(){
  max_retries=15
           retry_interval=60
           for ((retry=1; retry<=max_retries; retry++)); do
              echo "Attempt $retry of $max_retries for NNC status check"
              sleep $retry_interval
              failed=0
              nodes=($(kubectl get nodes -l kubernetes.azure.com/managed=false -o jsonpath='{.items[*].metadata.name}'))
              for node in "${nodes[@]}"; do
                 echo "Checking unmanaged node $node"
                 nnc_exists=$(kubectl get nnc -A -o jsonpath="{.items[?(@.metadata.name=='${node}')].metadata.name}")
                 if [[ "$nnc_exists" == "$node" ]]; then
                       allocated_ips=$(kubectl get nnc -A -o jsonpath="{.items[?(@.metadata.name=='$node')].status.assignedIPCount}")
                       echo "Allocated IPs for $node: $allocated_ips"
                       if [[ "$allocated_ips" -gt 0 ]]; then
                          echo "$node has $allocated_ips allocated IPs"
                       else
                          echo "No allocated IPs found for $node."
                          failed=2
                       fi
                 else
                       echo "No NNC found for $node"
                       failed=1
                 fi
              done

              if [[ "$failed" -eq 0 ]]; then
                 echo "All NNCs are created and allocated ips."
                 exit 0
              elif [[ "$retry" -eq "$max_retries" ]]; then
                 echo "NNC check failed after $max_retries attempts."
                 if [[ "$failed" -eq 1 ]]; then
                    step="NNC creation failed"
                 elif [[ "$failed" -eq 2 ]]; then
                    step="NNC IP allocation failed"
                 fi
              fi
           done

}

upload_kubeconfig "aks-1"
upload_kubeconfig "aks-2"

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

echo "Creating VMSS nodes for cluster aks-1..."
create_and_check_vmss "aks-1" "linux-highnic" "Standard_D16s_v3" "7"
wait_for_nodes_ready "aks-1" "aks-1-linux-highnic"
create_and_check_vmss "aks-1" "linux-default" "Standard_D4s_v3" "2"
wait_for_nodes_ready "aks-1" "aks-1-linux-default"

echo "Creating VMSS nodes for cluster aks-2..."
create_and_check_vmss "aks-2" "linux-highnic" "Standard_D16s_v3" "7"
wait_for_nodes_ready "aks-2" "aks-2-linux-highnic"
create_and_check_vmss "aks-2" "linux-default" "Standard_D4s_v3" "2"
wait_for_nodes_ready "aks-2" "aks-2-linux-default"

label_vmss_nodes "aks-1"
label_vmss_nodes "aks-2"

echo "VMSS deployment completed successfully for both clusters."
