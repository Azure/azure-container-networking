#!/usr/bin/env bash
set -e
trap 'echo "[ERROR] Failed during Private Endpoint or DNS setup." >&2' ERR

SUBSCRIPTION_ID=$1
LOCATION=$2
RG=$3
SA1_NAME=$4 

VNET_A1="cx_vnet_v1"
VNET_A2="cx_vnet_v2"
VNET_A3="cx_vnet_v3"
SUBNET_PE_A1="pe"
PE_NAME="${SA1_NAME}-pe"
PRIVATE_DNS_ZONE="privatelink.blob.core.windows.net"

verify_dns_zone() {
  local rg="$1"; local zone="$2"
  if az network private-dns zone show -g "$rg" -n "$zone" &>/dev/null; then
    echo "[OK] Verified DNS zone $zone exists."
  else
    echo "[ERROR] DNS zone $zone not found!" >&2
    exit 1
  fi
}

verify_dns_link() {
  local rg="$1"; local zone="$2"; local link="$3"
  if az network private-dns link vnet show -g "$rg" --zone-name "$zone" -n "$link" &>/dev/null; then
    echo "[OK] Verified DNS link $link exists."
  else
    echo "[ERROR] DNS link $link not found!" >&2
    exit 1
  fi
}

verify_private_endpoint() {
  local rg="$1"; local name="$2"
  if az network private-endpoint show -g "$rg" -n "$name" &>/dev/null; then
    echo "[OK] Verified Private Endpoint $name exists."
  else
    echo "[ERROR] Private Endpoint $name not found!" >&2
    exit 1
  fi
}

echo "Creating Private DNS zone: $PRIVATE_DNS_ZONE"
az network private-dns zone create -g "$RG" -n "$PRIVATE_DNS_ZONE" --output none \
  && echo "[OK] DNS zone $PRIVATE_DNS_ZONE created."
verify_dns_zone "$RG" "$PRIVATE_DNS_ZONE"

for VNET in "$VNET_A1" "$VNET_A2" "$VNET_A3"; do
  LINK_NAME="${VNET}-link"
  echo "Linking DNS zone $PRIVATE_DNS_ZONE to VNet $VNET"
  az network private-dns link vnet create \
    -g "$RG" -n "$LINK_NAME" \
    --zone-name "$PRIVATE_DNS_ZONE" \
    --virtual-network "$VNET" \
    --registration-enabled false \
    --output none \
    && echo "[OK] Linked DNS zone to $VNET."
  verify_dns_link "$RG" "$PRIVATE_DNS_ZONE" "$LINK_NAME"
done

echo "Linking DNS zone to AKS cluster VNets"
for CLUSTER in "aks-1" "aks-2"; do
  echo "Getting VNet for $CLUSTER"
  AKS_VNET_ID=$(az aks show -g "$RG" -n "$CLUSTER" --query "agentPoolProfiles[0].vnetSubnetId" -o tsv | cut -d'/' -f1-9)
  
  if [ -z "$AKS_VNET_ID" ]; then
    echo "[WARNING] Could not get VNet for $CLUSTER, skipping DNS link"
    continue
  fi
  
  LINK_NAME="${CLUSTER}-vnet-link"
  echo "Linking DNS zone to $CLUSTER VNet"
  az network private-dns link vnet create \
    -g "$RG" -n "$LINK_NAME" \
    --zone-name "$PRIVATE_DNS_ZONE" \
    --virtual-network "$AKS_VNET_ID" \
    --registration-enabled false \
    --output none \
    && echo "[OK] Linked DNS zone to $CLUSTER VNet."
  verify_dns_link "$RG" "$PRIVATE_DNS_ZONE" "$LINK_NAME"
done

echo "Creating Private Endpoint for Storage Account: $SA1_NAME"
SA1_ID=$(az storage account show -g "$RG" -n "$SA1_NAME" --query id -o tsv)
DNS_ZONE_ID=$(az network private-dns zone show -g "$RG" -n "$PRIVATE_DNS_ZONE" --query id -o tsv)

az network private-endpoint create \
  -g "$RG" -n "$PE_NAME" -l "$LOCATION" \
  --vnet-name "$VNET_A1" --subnet "$SUBNET_PE_A1" \
  --private-connection-resource-id "$SA1_ID" \
  --group-id blob \
  --connection-name "${PE_NAME}-conn" \
  --output none \
  && echo "[OK] Private Endpoint $PE_NAME created for $SA1_NAME."
verify_private_endpoint "$RG" "$PE_NAME"

echo "Creating Private DNS Zone Group to register DNS record"
az network private-endpoint dns-zone-group create \
  -g "$RG" \
  --endpoint-name "$PE_NAME" \
  --name "default" \
  --private-dns-zone "$DNS_ZONE_ID" \
  --zone-name "blob" \
  --output none \
  && echo "[OK] DNS Zone Group created, DNS record will be auto-registered."

echo "All Private DNS and Endpoint resources created and verified successfully."
