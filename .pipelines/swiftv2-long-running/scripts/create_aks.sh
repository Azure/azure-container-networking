#!/usr/bin/env bash
set -e

SUBSCRIPTION_ID=$1
LOCATION=$2
RG=$3
VM_SKU_DEFAULT=$4
VM_SKU_HIGHNIC=$5

echo "Subscription id: $SUBSCRIPTION_ID"
echo "Resource group: $RG"
echo "Location: $LOCATION"
echo "VM SKU (default): $VM_SKU_DEFAULT"
echo "VM SKU (high-NIC): $VM_SKU_HIGHNIC"
az account set --subscription "$SUBSCRIPTION_ID"

echo "==> Creating resource group: $RG"
az group create -n "$RG" -l "$LOCATION" --output none

# Enable parallel cluster creation
create_cluster() {
  local CLUSTER=$1
  echo "==> Creating AKS cluster: $CLUSTER"

  az aks create -g "$RG" -n "$CLUSTER" -l "$LOCATION" \
    --network-plugin azure --node-count 1 \
    --node-vm-size "$VM_SKU_DEFAULT" \
    --enable-managed-identity --generate-ssh-keys \
    --load-balancer-sku standard --yes --only-show-errors

  echo "==> Adding high-NIC nodepool to $CLUSTER"
  az aks nodepool add -g "$RG" -n highnic \
    --cluster-name "$CLUSTER" --node-count 2 \
    --node-vm-size "$VM_SKU_HIGHNIC" --mode User --only-show-errors

  echo "Finished AKS cluster: $CLUSTER"
}

# Run both clusters in parallel
create_cluster "aks-cluster-a" &
pid_a=$!

create_cluster "aks-cluster-b" &
pid_b=$!

# Wait for both to finish
wait $pid_a $pid_b

echo "AKS clusters created successfully!"
