#!/usr/bin/env bash
set -e
trap 'echo "[ERROR] Failed during NSG creation or rule setup." >&2' ERR

SUBSCRIPTION_ID=$1
RG=$2
LOCATION=$3

VNET_A1="cx_vnet_a1"
SUBNET1_PREFIX="10.10.1.0/24"
SUBNET2_PREFIX="10.10.2.0/24"
NSG_NAME="${VNET_A1}-nsg"

echo "==> Creating Network Security Group: $NSG_NAME"
az network nsg create -g "$RG" -n "$NSG_NAME" -l "$LOCATION" --output none \
  && echo "[OK] NSG '$NSG_NAME' created."

echo "==> Creating NSG rule to DENY traffic from Subnet1 ($SUBNET1_PREFIX) to Subnet2 ($SUBNET2_PREFIX)"
az network nsg rule create \
  -g "$RG" \
  --nsg-name "$NSG_NAME" \
  -n deny-subnet1-to-subnet2 \
  --priority 100 \
  --source-address-prefixes "$SUBNET1_PREFIX" \
  --destination-address-prefixes "$SUBNET2_PREFIX" \
  --direction Inbound \
  --access Deny \
  --protocol "*" \
  --description "Deny all traffic from Subnet1 to Subnet2" \
  --output none \
  && echo "[OK] Deny rule from Subnet1 → Subnet2 created."

echo "==> Creating NSG rule to DENY traffic from Subnet2 ($SUBNET2_PREFIX) to Subnet1 ($SUBNET1_PREFIX)"
az network nsg rule create \
  -g "$RG" \
  --nsg-name "$NSG_NAME" \
  -n deny-subnet2-to-subnet1 \
  --priority 200 \
  --source-address-prefixes "$SUBNET2_PREFIX" \
  --destination-address-prefixes "$SUBNET1_PREFIX" \
  --direction Inbound \
  --access Deny \
  --protocol "*" \
  --description "Deny all traffic from Subnet2 to Subnet1" \
  --output none \
  && echo "[OK] Deny rule from Subnet2 → Subnet1 created."

echo "NSG '$NSG_NAME' created successfully with bidirectional isolation between Subnet1 and Subnet2."
