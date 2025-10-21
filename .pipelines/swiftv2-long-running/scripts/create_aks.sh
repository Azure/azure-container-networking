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

# AKS clusters
for CLUSTER in "aks-cluster-a" "aks-cluster-b"; do
  echo "==> Creating AKS cluster: $CLUSTER"
  az aks create -g "$RG" -n "$CLUSTER" -l "$LOCATION" \
    --network-plugin azure --node-count 1 \
    --node-vm-size "$VM_SKU_DEFAULT" \
    --enable-managed-identity --generate-ssh-keys \
    --load-balancer-sku standard --yes

  echo "==> Adding high-NIC nodepool to $CLUSTER"
  az aks nodepool add -g "$RG" -n highnic \
    --cluster-name "$CLUSTER" --node-count 2 \
    --node-vm-size "$VM_SKU_HIGHNIC" --mode User
done
