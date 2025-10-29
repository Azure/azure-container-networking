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

verify_nsg() {
  local rg="$1"; local name="$2"
  echo "==> Verifying NSG: $name"
  if az network nsg show -g "$rg" -n "$name" &>/dev/null; then
    echo "[OK] Verified NSG $name exists."
  else
    echo "[ERROR] NSG $name not found!" >&2
    exit 1
  fi
}

verify_nsg_rule() {
  local rg="$1"; local nsg="$2"; local rule="$3"
  echo "==> Verifying NSG rule: $rule in $nsg"
  if az network nsg rule show -g "$rg" --nsg-name "$nsg" -n "$rule" &>/dev/null; then
    echo "[OK] Verified NSG rule $rule exists in $nsg."
  else
    echo "[ERROR] NSG rule $rule not found in $nsg!" >&2
    exit 1
  fi
}

verify_subnet_nsg_association() {
  local rg="$1"; local vnet="$2"; local subnet="$3"; local nsg="$4"
  echo "==> Verifying NSG association on subnet $subnet..."
  local associated_nsg
  associated_nsg=$(az network vnet subnet show -g "$rg" --vnet-name "$vnet" -n "$subnet" --query "networkSecurityGroup.id" -o tsv 2>/dev/null || echo "")
  if [[ "$associated_nsg" == *"$nsg"* ]]; then
    echo "[OK] Verified subnet $subnet is associated with NSG $nsg."
  else
    echo "[ERROR] Subnet $subnet is NOT associated with NSG $nsg!" >&2
    exit 1
  fi
}

# -------------------------------
#  1. Create NSG
# -------------------------------
echo "==> Creating Network Security Group: $NSG_NAME"
az network nsg create -g "$RG" -n "$NSG_NAME" -l "$LOCATION" --output none \
  && echo "[OK] NSG '$NSG_NAME' created."
verify_nsg "$RG" "$NSG_NAME"

# -------------------------------
#  2. Create NSG Rules
# -------------------------------
echo "==> Creating NSG rule to DENY traffic from Subnet1 ($SUBNET1_PREFIX) to Subnet2 ($SUBNET2_PREFIX)"
az network nsg rule create \
  --resource-group "$RG" \
  --nsg-name "$NSG_NAME" \
  --name deny-subnet1-to-subnet2 \
  --priority 100 \
  --source-address-prefixes "$SUBNET1_PREFIX" \
  --destination-address-prefixes "$SUBNET2_PREFIX" \
  --direction Inbound \
  --access Deny \
  --protocol "*" \
  --description "Deny all traffic from Subnet1 to Subnet2" \
  --output none \
  && echo "[OK] Deny rule from Subnet1 → Subnet2 created."

verify_nsg_rule "$RG" "$NSG_NAME" "deny-subnet1-to-subnet2"

echo "==> Creating NSG rule to DENY traffic from Subnet2 ($SUBNET2_PREFIX) to Subnet1 ($SUBNET1_PREFIX)"
az network nsg rule create \
  --resource-group "$RG" \
  --nsg-name "$NSG_NAME" \
  --name deny-subnet2-to-subnet1 \
  --priority 200 \
  --source-address-prefixes "$SUBNET2_PREFIX" \
  --destination-address-prefixes "$SUBNET1_PREFIX" \
  --direction Inbound \
  --access Deny \
  --protocol "*" \
  --description "Deny all traffic from Subnet2 to Subnet1" \
  --output none \
  && echo "[OK] Deny rule from Subnet2 → Subnet1 created."

verify_nsg_rule "$RG" "$NSG_NAME" "deny-subnet2-to-subnet1"

# -------------------------------
#  3. Associate NSG with Subnets
# -------------------------------
for SUBNET in s1 s2; do
  echo "==> Associating NSG $NSG_NAME with subnet $SUBNET"
  az network vnet subnet update \
    --name "$SUBNET" \
    --vnet-name "$VNET_A1" \
    --resource-group "$RG" \
    --network-security-group "$NSG_NAME" \
    --output none
  verify_subnet_nsg_association "$RG" "$VNET_A1" "$SUBNET" "$NSG_NAME"
done

echo "NSG '$NSG_NAME' created successfully with bidirectional isolation between Subnet1 and Subnet2."

