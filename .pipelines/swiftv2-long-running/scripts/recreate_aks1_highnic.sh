#!/bin/bash
# Recreate aks-1-linux-highnic VMSS nodes in eastus2euap
# This script only creates the highnic VMSS for aks-1 and joins it to the cluster.
# It reuses the same logic as deploy_linuxbyon.sh but scoped to a single VMSS.
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

  # Delete existing VMSS instances if any (clean slate)
  echo "Cleaning up existing VMSS instances for '${node_name}'..."
  existing_ids=$(az vmss list-instances -g "$RESOURCE_GROUP" -n "$node_name" --query "[].instanceId" -o tsv 2>/dev/null || echo "")
  if [[ -n "$existing_ids" ]]; then
    echo "Deleting existing instances: $existing_ids"
    az vmss delete-instances -g "$RESOURCE_GROUP" -n "$node_name" --instance-ids $existing_ids 2>/dev/null || true
    echo "Waiting for instances to be deleted..."
    sleep 60
  fi

  echo "Creating Linux VMSS Node '${node_name}' for cluster '${cluster_name}'"
  set +e
  az deployment group create -n "recreate-${node_name}" \
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

label_nodes() {
  local cluster_name=$1
  local kubeconfig_file="./kubeconfig-${cluster_name}"

  echo "Labeling highnic BYON nodes in ${cluster_name}"
  kubectl --kubeconfig "$kubeconfig_file" get nodes -o name | grep "${cluster_name}-linux-highnic" | xargs -I {} kubectl --kubeconfig "$kubeconfig_file" label {} workload-type=swiftv2-linux-byon nic-capacity=high-nic --overwrite || true

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

  new_nodes=($(kubectl --kubeconfig "$kubeconfig_file" get nodes -o name | grep "${cluster_name}-linux-highnic"))

  for node_ref in "${new_nodes[@]}"; do
      NODENAME=$(echo "$node_ref" | sed 's|node/||')
      for label_key in "${LABEL_KEYS[@]}"; do
        v=$(kubectl --kubeconfig "$kubeconfig_file" get nodes "$SOURCE_NODE" -o jsonpath="{.metadata.labels['$label_key']}")
        l=$(echo "$label_key" | sed 's/\\//g')
        echo "Labeling node $NODENAME with $l=$v"
        kubectl --kubeconfig "$kubeconfig_file" label node "$NODENAME" "$l=$v" --overwrite
      done
  done
}

# --- Main ---
echo "=== Recreating aks-1-linux-highnic VMSS in ${RESOURCE_GROUP} ==="

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

# Only aks-1, only highnic
cluster_name="aks-1"

# Upload fresh kubeconfig to Key Vault
upload_kubeconfig "$cluster_name"

# Delete stale K8s node objects
echo "Cleaning stale K8s node objects..."
kubeconfig_file="./kubeconfig-${cluster_name}"
for stale_node in $(kubectl --kubeconfig "$kubeconfig_file" get nodes -o jsonpath='{.items[*].metadata.name}' | tr ' ' '\n' | grep "^${cluster_name}-linux-highnic" || true); do
  echo "Deleting stale node: $stale_node"
  kubectl --kubeconfig "$kubeconfig_file" delete node "$stale_node" --ignore-not-found=true || true
done

# Recreate the VMSS (this handles deletion + creation via Bicep)
create_and_check_vmss "$cluster_name" "linux-highnic" "Standard_D16s_v3" "7"

# Wait for nodes to join
wait_for_nodes_ready "$cluster_name" "${cluster_name}-linux-highnic"

# Label the new nodes
label_nodes "$cluster_name"

echo "=== aks-1-linux-highnic recreation complete ==="
kubectl --kubeconfig "$kubeconfig_file" get nodes -o wide | grep highnic
