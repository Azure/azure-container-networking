#!/usr/bin/env bash
set -e
trap 'echo "[ERROR] Failed during Private Endpoint or DNS setup." >&2' ERR

SUBSCRIPTION_ID=$1
LOCATION=$2
RG=$3
SA1_NAME=$4  # Storage account 1

VNET_A1="cx_vnet_a1"
VNET_A2="cx_vnet_a2"
VNET_A3="cx_vnet_a3"
SUBNET_PE_A1="pe"
PE_NAME="${SA1_NAME}-pe"
PRIVATE_DNS_ZONE="privatelink.blob.core.windows.net"

# 1. Create Private DNS zone
echo "==> Creating Private DNS zone: $PRIVATE_DNS_ZONE"
az network private-dns zone create -g "$RG" -n "$PRIVATE_DNS_ZONE" --output none \
  && echo "[OK] DNS zone $PRIVATE_DNS_ZONE created."

# 2. Link DNS zone to VNet
echo "==> Linking DNS zone $PRIVATE_DNS_ZONE to VNet $VNET_A1"
az network private-dns link vnet create \
  -g "$RG" -n "${VNET_A1}-link" \
  --zone-name "$PRIVATE_DNS_ZONE" \
  --virtual-network "$VNET_A1" \
  --registration-enabled false \
  && echo "[OK] Linked DNS zone to $VNET_A1."

az network private-dns link vnet create \
  -g "$RG" -n "${VNET_A2}-link" \
  --zone-name "$PRIVATE_DNS_ZONE" \
  --virtual-network "$VNET_A2" \
  --registration-enabled false \
  && echo "[OK] Linked DNS zone to $VNET_A2."

az network private-dns link vnet create \
  -g "$RG" -n "${VNET_A3}-link" \
  --zone-name "$PRIVATE_DNS_ZONE" \
  --virtual-network "$VNET_A3" \
  --registration-enabled false \
  && echo "[OK] Linked DNS zone to $VNET_A3."


# 3. Create Private Endpoint
echo "==> Creating Private Endpoint for Storage Account: $SA1_NAME"
SA1_ID=$(az storage account show -g "$RG" -n "$SA1_NAME" --query id -o tsv)
az network private-endpoint create \
  -g "$RG" -n "$PE_NAME" -l "$LOCATION" \
  --vnet-name "$VNET_A1" --subnet "$SUBNET_PE_A1" \
  --private-connection-resource-id "$SA1_ID" \
  --group-id blob \
  --connection-name "${PE_NAME}-conn" \
  --output none \
  && echo "[OK] Private Endpoint $PE_NAME created for $SA1_NAME."
