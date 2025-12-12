#!/bin/bash
set -euo pipefail
set -x

source "$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd)"/toolbox.sh

function main() {
	cluster_name="$1"
    subnet_name="$2"
    dummy_cluster_name="$3"
    cluster_kubeconfig_keyvault_name="$4"
    cluster_kubeconfig_secret_name="$5"
    ssh_public_key_secret_name="$6
    cluster_kubeconfig="$(mktemp)"


	az acr login -n acndev
	location=$(get_underlay_location "./azureconfig.yaml")
	
	deploy_nodes "$dummy_cluster_name" \
    "$subnet_name" \
    "$(clean_cluster_name $cluster_name)" \
    "$location" \
    "$cluster_kubeconfig_keyvault_name" \
    "$cluster_kubeconfig_secret_name" \
    "$ssh_public_key_secret_name" \
    "$cluster_kubeconfig"

  # If we reach here, the deployment was successful
  echo "Nodes deployed successfully."
  return 0
}

function deploy_nodes() {
  dummy_cluster_name="$1"
  subnet_name="$2"
  cluster_resource_group="$3"
  cluster_location="$4"
  kubeconfig_kv_name="$5"
  kubeconfig_secret_name="$6"
  ssh_public_key_secret_name="$7"
  kubeconfig_file="$8"

  node_resource_group=$(az aks show --name "$dummy_cluster_name" --resource-group "$cluster_resource_group" --query nodeResourceGroup -o tsv | tr -d '\r\n' | xargs)
  echo "Raw value of node resource group: '$node_resource_group'"
  vnet_name=$(az network vnet list --resource-group "$node_resource_group" --query '[].name' -o tsv)
  SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"

  az identity create --name aksbootstrap --resource-group "$cluster_resource_group" --location "$cluster_location"
  identity_principal_id=$(az identity show --resource-group "$cluster_resource_group" --name aksbootstrap --query principalId -o tsv)
  sed "s|__OBJECT_ID__|$identity_principal_id|g" "$SCRIPT_DIR/bootstrap-role.yaml" | kubectl apply --kubeconfig="$kubeconfig_file" -f -

  echo "Finding resource group for Key Vault: $kubeconfig_kv_name"
  kv_resource_group=$(az keyvault show --name "$kubeconfig_kv_name" --query resourceGroup -o tsv)
  echo "Key Vault resource group: $kv_resource_group"

  echo "Fetching SSH public key from Key Vault..."
  ssh_public_key=$(az keyvault secret show \
    --name "$ssh_public_key_secret_name" \
    --vault-name "$kubeconfig_kv_name" \
    --query value -o tsv 2>/dev/null || echo "")

  if [[ -z "$ssh_public_key" ]]; then
    echo "##vso[task.logissue type=error]SSH public key secret is empty or inaccessible."
    return 1
  else
    echo "SSH public key retrieved successfully from Key Vault"
  fi

  node_names=("linone" "lintwo")
  for node_name in "${node_names[@]}"; do
    extension_name="NodeJoin-$node_name"
    echo "Creating Linux VMSS Node $node_name"
    az deployment group create \
      --name "sat$node_name" \
      --resource-group "$cluster_resource_group" \
      --template-file "$SCRIPT_DIR/../../singularity-runner/byon/linux.bicep" \
      --parameters \
        name="$node_name" \
        sshPublicKey="$ssh_public_key" \
        vnetrgname="$node_resource_group" \
        vnetname="$vnet_name" \
        subnetname="$subnet_name" \
        extensionName="$extension_name" \
        clusterKubeconfigKeyvaultName="$kubeconfig_kv_name" \
        clusterKubeconfigSecretName="$kubeconfig_secret_name" \
        keyVaultSubscription="$(get_underlay_subscription "./azureconfig.yaml")" \
        keyVaultResourceGroup="$kv_resource_group"
  done
  wait
}


main $@