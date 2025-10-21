#!/usr/bin/env bash
set -e
trap 'echo "[ERROR] Failed during NSG creation." >&2' ERR

SUBSCRIPTION_ID=$1
RG=$2
LOCATION=${3:-centraluseuap}

VNET_A1="cx_vnet_a1"
NSG_NAME="${VNET_A1}-nsg"

echo "==> Creating Network Security Group: $NSG_NAME"
az network nsg create -g "$RG" -n "$NSG_NAME" -l "$LOCATION" --output none \
  && echo "NSG $NSG_NAME created."

echo "==> Adding NSG rules"

# Allow SSH from any
az network nsg rule create \
  -g "$RG" \
  --nsg-name "$NSG_NAME" \
  -n allow-ssh \
  --priority 100 \
  --source-address-prefixes "*" \
  --destination-port-ranges 22 \
  --direction Inbound \
  --access Allow \
  --protocol Tcp \
  --description "Allow SSH access" \
  --output none \
    && echo "Rule allow-ssh created."

# Allow internal VNet traffic
az network nsg rule create \
  -g "$RG" \
  --nsg-name "$NSG_NAME" \
  -n allow-vnet \
  --priority 200 \
  --source-address-prefixes VirtualNetwork \
  --destination-address-prefixes VirtualNetwork \
  --direction Inbound \
  --access Allow \
  --protocol "*" \
  --description "Allow VNet internal traffic" \
  --output none \
    && echo "Rule allow-vnet created."

# Allow AKS API traffic
az network nsg rule create \
  -g "$RG" \
  --nsg-name "$NSG_NAME" \
  -n allow-aks-controlplane \
  --priority 300 \
  --source-address-prefixes AzureCloud \
  --destination-port-ranges 443 \
  --direction Inbound \
  --access Allow \
  --protocol Tcp \
  --description "Allow AKS control plane traffic" \
  --output none \
    && echo "Rule allow-aks-controlplane created."

echo "NSG '$NSG_NAME' created successfully with rules."
