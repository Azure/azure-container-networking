#!/usr/bin/env bash
set -e
trap 'echo "[ERROR] Failed during Private Endpoint or DNS setup." >&2' ERR

SUBSCRIPTION_ID=$1
LOCATION=$2
RG=$3
SA1_NAME=$4  # from previous script (storage account 1)
SA2_NAME=$5  # from previous script (storage account 2)
VNET_A1="cx_vnet_a1"

SUBNET_PE_A1="pe"
PE_NAME="${SA1_NAME}-pe"
PRIVATE_DNS_ZONE="privatelink.blob.core.windows.net"
LINK_NAME="${VNET_A1}-link"

echo "==> Creating Private DNS zone: $PRIVATE_DNS_ZONE"
az network private-dns zone create -g "$RG" -n "$PRIVATE_DNS_ZONE" --output none \
  && echo "[OK] DNS zone $PRIVATE_DNS_ZONE created."

echo "==> Linking DNS zone $PRIVATE_DNS_ZONE to VNet $VNET_A1"
az network private-dns link-vnet create \
  -g "$RG" -n "$LINK_NAME" \
  --zone-name "$PRIVATE_DNS_ZONE" \
  --virtual-network "$VNET_A1" \
  --registration-enabled false --output none \
  && echo "[OK] Linked DNS zone to $VNET_A1."

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

echo "==> Linking Private Endpoint to DNS zone"
NIC_ID=$(az network private-endpoint show -g "$RG" -n "$PE_NAME" --query 'networkInterfaces[0].id' -o tsv)
FQDN=$(az storage account show -g "$RG" -n "$SA1_NAME" --query 'primaryEndpoints.blob' -o tsv | sed 's#https://##; s#/##')
PRIVATE_IP=$(az network nic show --ids "$NIC_ID" --query 'ipConfigurations[0].privateIpAddress' -o tsv)

az network private-dns record-set a add-record \
  -g "$RG" -z "$PRIVATE_DNS_ZONE" -n "$FQDN" -a "$PRIVATE_IP" --output none \
  && echo "[OK] Added Private DNS record for $SA1_NAME → $PRIVATE_IP"

echo "Private Endpoint setup complete for $SA1_NAME (accessible only within VNet A1)."
