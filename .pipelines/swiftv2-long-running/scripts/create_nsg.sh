#!/usr/bin/env bash
set -e
trap 'echo "[ERROR] Failed during NSG creation or rule setup." >&2' ERR

SUBSCRIPTION_ID=$1
RG=$2
LOCATION=$3

VNET_A1="cx_vnet_v1"

# Get actual subnet CIDR ranges dynamically
echo "==> Retrieving actual subnet address prefixes..."
SUBNET1_PREFIX=$(az network vnet subnet show -g "$RG" --vnet-name "$VNET_A1" -n s1 --query "addressPrefix" -o tsv)
SUBNET2_PREFIX=$(az network vnet subnet show -g "$RG" --vnet-name "$VNET_A1" -n s2 --query "addressPrefix" -o tsv)

echo "Subnet s1 CIDR: $SUBNET1_PREFIX"
echo "Subnet s2 CIDR: $SUBNET2_PREFIX"

if [[ -z "$SUBNET1_PREFIX" || -z "$SUBNET2_PREFIX" ]]; then
  echo "[ERROR] Failed to retrieve subnet address prefixes!" >&2
  exit 1
fi

# Wait 5 minutes for NSGs to be associated with subnets
echo "==> Waiting 5 minutes for NSG associations to complete..."
sleep 300

# Get NSG IDs associated with each subnet with retry logic
echo "==> Retrieving NSGs associated with subnets..."
max_retries=10
retry_count=0
retry_delay=30

while [[ $retry_count -lt $max_retries ]]; do
  NSG_S1_ID=$(az network vnet subnet show -g "$RG" --vnet-name "$VNET_A1" -n s1 --query "networkSecurityGroup.id" -o tsv 2>/dev/null || echo "")
  NSG_S2_ID=$(az network vnet subnet show -g "$RG" --vnet-name "$VNET_A1" -n s2 --query "networkSecurityGroup.id" -o tsv 2>/dev/null || echo "")
  
  if [[ -n "$NSG_S1_ID" && -n "$NSG_S2_ID" ]]; then
    echo "[OK] Successfully retrieved NSG associations for both subnets"
    break
  fi
  
  retry_count=$((retry_count + 1))
  if [[ $retry_count -lt $max_retries ]]; then
    echo "[RETRY $retry_count/$max_retries] NSG associations not ready yet. Waiting ${retry_delay}s before retry..."
    echo "  Subnet s1 NSG ID: ${NSG_S1_ID:-<empty>}"
    echo "  Subnet s2 NSG ID: ${NSG_S2_ID:-<empty>}"
    sleep $retry_delay
  else
    echo "[ERROR] Failed to retrieve NSG associations after $max_retries attempts!" >&2
    echo "  Subnet s1 NSG ID: ${NSG_S1_ID:-<empty>}" >&2
    echo "  Subnet s2 NSG ID: ${NSG_S2_ID:-<empty>}" >&2
    exit 1
  fi
done

# Extract NSG names from IDs
NSG_S1_NAME=$(basename "$NSG_S1_ID")
NSG_S2_NAME=$(basename "$NSG_S2_ID")

echo "Subnet s1 NSG: $NSG_S1_NAME"
echo "Subnet s2 NSG: $NSG_S2_NAME"

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

wait_for_nsg() {
  local rg="$1"; local name="$2"
  echo "==> Waiting for NSG $name to become available..."
  local max_attempts=30
  local attempt=0
  while [[ $attempt -lt $max_attempts ]]; do
    if az network nsg show -g "$rg" -n "$name" &>/dev/null; then
      local provisioning_state
      provisioning_state=$(az network nsg show -g "$rg" -n "$name" --query "provisioningState" -o tsv)
      if [[ "$provisioning_state" == "Succeeded" ]]; then
        echo "[OK] NSG $name is available (provisioningState: $provisioning_state)."
        return 0
      fi
      echo "Waiting... NSG $name provisioningState: $provisioning_state"
    fi
    attempt=$((attempt + 1))
    sleep 10
  done
  echo "[ERROR] NSG $name did not become available within the expected time!" >&2
  exit 1
}

# -------------------------------
#  1. Wait for NSG to be available
# -------------------------------
wait_for_nsg "$RG" "$NSG_S1_NAME"

# Check if both subnets share the same NSG
if [[ "$NSG_S1_NAME" == "$NSG_S2_NAME" ]]; then
  echo "==> Both subnets share the same NSG: $NSG_S1_NAME"
  echo "==> Creating all NSG rules on shared NSG with unique priorities"
  
  # Rule 1: Deny Outbound traffic FROM Subnet1 TO Subnet2
  echo "==> Creating NSG rule on $NSG_S1_NAME to DENY OUTBOUND traffic from Subnet1 ($SUBNET1_PREFIX) to Subnet2 ($SUBNET2_PREFIX)"
  az network nsg rule create \
    --resource-group "$RG" \
    --nsg-name "$NSG_S1_NAME" \
    --name deny-s1-to-s2-outbound \
    --priority 100 \
    --source-address-prefixes "$SUBNET1_PREFIX" \
    --destination-address-prefixes "$SUBNET2_PREFIX" \
    --source-port-ranges "*" \
    --destination-port-ranges "*" \
    --direction Outbound \
    --access Deny \
    --protocol "*" \
    --description "Deny outbound traffic from Subnet1 to Subnet2" \
    --output none \
    && echo "[OK] Deny outbound rule from Subnet1 → Subnet2 created on $NSG_S1_NAME."
  
  verify_nsg_rule "$RG" "$NSG_S1_NAME" "deny-s1-to-s2-outbound"
  
  # Rule 2: Deny Inbound traffic FROM Subnet2 TO Subnet1
  echo "==> Creating NSG rule on $NSG_S1_NAME to DENY INBOUND traffic from Subnet2 ($SUBNET2_PREFIX) to Subnet1 ($SUBNET1_PREFIX)"
  az network nsg rule create \
    --resource-group "$RG" \
    --nsg-name "$NSG_S1_NAME" \
    --name deny-s2-to-s1-inbound \
    --priority 100 \
    --source-address-prefixes "$SUBNET2_PREFIX" \
    --destination-address-prefixes "$SUBNET1_PREFIX" \
    --source-port-ranges "*" \
    --destination-port-ranges "*" \
    --direction Inbound \
    --access Deny \
    --protocol "*" \
    --description "Deny inbound traffic from Subnet2 to Subnet1" \
    --output none \
    && echo "[OK] Deny inbound rule from Subnet2 → Subnet1 created on $NSG_S1_NAME."
  
  verify_nsg_rule "$RG" "$NSG_S1_NAME" "deny-s2-to-s1-inbound"
  
  # Rule 3: Deny Outbound traffic FROM Subnet2 TO Subnet1
  echo "==> Creating NSG rule on $NSG_S1_NAME to DENY OUTBOUND traffic from Subnet2 ($SUBNET2_PREFIX) to Subnet1 ($SUBNET1_PREFIX)"
  az network nsg rule create \
    --resource-group "$RG" \
    --nsg-name "$NSG_S1_NAME" \
    --name deny-s2-to-s1-outbound \
    --priority 110 \
    --source-address-prefixes "$SUBNET2_PREFIX" \
    --destination-address-prefixes "$SUBNET1_PREFIX" \
    --source-port-ranges "*" \
    --destination-port-ranges "*" \
    --direction Outbound \
    --access Deny \
    --protocol "*" \
    --description "Deny outbound traffic from Subnet2 to Subnet1" \
    --output none \
    && echo "[OK] Deny outbound rule from Subnet2 → Subnet1 created on $NSG_S1_NAME."
  
  verify_nsg_rule "$RG" "$NSG_S1_NAME" "deny-s2-to-s1-outbound"
  
  # Rule 4: Deny Inbound traffic FROM Subnet1 TO Subnet2
  echo "==> Creating NSG rule on $NSG_S1_NAME to DENY INBOUND traffic from Subnet1 ($SUBNET1_PREFIX) to Subnet2 ($SUBNET2_PREFIX)"
  az network nsg rule create \
    --resource-group "$RG" \
    --nsg-name "$NSG_S1_NAME" \
    --name deny-s1-to-s2-inbound \
    --priority 110 \
    --source-address-prefixes "$SUBNET1_PREFIX" \
    --destination-address-prefixes "$SUBNET2_PREFIX" \
    --source-port-ranges "*" \
    --destination-port-ranges "*" \
    --direction Inbound \
    --access Deny \
    --protocol "*" \
    --description "Deny inbound traffic from Subnet1 to Subnet2" \
    --output none \
    && echo "[OK] Deny inbound rule from Subnet1 → Subnet2 created on $NSG_S1_NAME."
  
  verify_nsg_rule "$RG" "$NSG_S1_NAME" "deny-s1-to-s2-inbound"
  
  echo "NSG rules applied successfully on shared NSG $NSG_S1_NAME with bidirectional isolation between Subnet1 and Subnet2."
else
  echo "==> Subnets have different NSGs"
  echo "==> Subnet s1 NSG: $NSG_S1_NAME"
  echo "==> Subnet s2 NSG: $NSG_S2_NAME"
  
  wait_for_nsg "$RG" "$NSG_S2_NAME"
  
  # -------------------------------
  #  2. Create NSG Rules on Subnet1's NSG
  # -------------------------------
  # Rule 1: Deny Outbound traffic FROM Subnet1 TO Subnet2
  echo "==> Creating NSG rule on $NSG_S1_NAME to DENY OUTBOUND traffic from Subnet1 ($SUBNET1_PREFIX) to Subnet2 ($SUBNET2_PREFIX)"
  az network nsg rule create \
    --resource-group "$RG" \
    --nsg-name "$NSG_S1_NAME" \
    --name deny-s1-to-s2-outbound \
    --priority 100 \
    --source-address-prefixes "$SUBNET1_PREFIX" \
    --destination-address-prefixes "$SUBNET2_PREFIX" \
    --source-port-ranges "*" \
    --destination-port-ranges "*" \
    --direction Outbound \
    --access Deny \
    --protocol "*" \
    --description "Deny outbound traffic from Subnet1 to Subnet2" \
    --output none \
    && echo "[OK] Deny outbound rule from Subnet1 → Subnet2 created on $NSG_S1_NAME."
  
  verify_nsg_rule "$RG" "$NSG_S1_NAME" "deny-s1-to-s2-outbound"
  
  # Rule 2: Deny Inbound traffic FROM Subnet2 TO Subnet1 (for packets arriving at s1)
  echo "==> Creating NSG rule on $NSG_S1_NAME to DENY INBOUND traffic from Subnet2 ($SUBNET2_PREFIX) to Subnet1 ($SUBNET1_PREFIX)"
  az network nsg rule create \
    --resource-group "$RG" \
    --nsg-name "$NSG_S1_NAME" \
    --name deny-s2-to-s1-inbound \
    --priority 110 \
    --source-address-prefixes "$SUBNET2_PREFIX" \
    --destination-address-prefixes "$SUBNET1_PREFIX" \
    --source-port-ranges "*" \
    --destination-port-ranges "*" \
    --direction Inbound \
    --access Deny \
    --protocol "*" \
    --description "Deny inbound traffic from Subnet2 to Subnet1" \
    --output none \
    && echo "[OK] Deny inbound rule from Subnet2 → Subnet1 created on $NSG_S1_NAME."
  
  verify_nsg_rule "$RG" "$NSG_S1_NAME" "deny-s2-to-s1-inbound"
  
  # -------------------------------
  #  3. Create NSG Rules on Subnet2's NSG
  # -------------------------------
  # Rule 3: Deny Outbound traffic FROM Subnet2 TO Subnet1
  echo "==> Creating NSG rule on $NSG_S2_NAME to DENY OUTBOUND traffic from Subnet2 ($SUBNET2_PREFIX) to Subnet1 ($SUBNET1_PREFIX)"
  az network nsg rule create \
    --resource-group "$RG" \
    --nsg-name "$NSG_S2_NAME" \
    --name deny-s2-to-s1-outbound \
    --priority 100 \
    --source-address-prefixes "$SUBNET2_PREFIX" \
    --destination-address-prefixes "$SUBNET1_PREFIX" \
    --source-port-ranges "*" \
    --destination-port-ranges "*" \
    --direction Outbound \
    --access Deny \
    --protocol "*" \
    --description "Deny outbound traffic from Subnet2 to Subnet1" \
    --output none \
    && echo "[OK] Deny outbound rule from Subnet2 → Subnet1 created on $NSG_S2_NAME."
  
  verify_nsg_rule "$RG" "$NSG_S2_NAME" "deny-s2-to-s1-outbound"
  
  # Rule 4: Deny Inbound traffic FROM Subnet1 TO Subnet2 (for packets arriving at s2)
  echo "==> Creating NSG rule on $NSG_S2_NAME to DENY INBOUND traffic from Subnet1 ($SUBNET1_PREFIX) to Subnet2 ($SUBNET2_PREFIX)"
  az network nsg rule create \
    --resource-group "$RG" \
    --nsg-name "$NSG_S2_NAME" \
    --name deny-s1-to-s2-inbound \
    --priority 110 \
    --source-address-prefixes "$SUBNET1_PREFIX" \
    --destination-address-prefixes "$SUBNET2_PREFIX" \
    --source-port-ranges "*" \
    --destination-port-ranges "*" \
    --direction Inbound \
    --access Deny \
    --protocol "*" \
    --description "Deny inbound traffic from Subnet1 to Subnet2" \
    --output none \
    && echo "[OK] Deny inbound rule from Subnet1 → Subnet2 created on $NSG_S2_NAME."
  
  verify_nsg_rule "$RG" "$NSG_S2_NAME" "deny-s1-to-s2-inbound"
  
  echo "NSG rules applied successfully on $NSG_S1_NAME and $NSG_S2_NAME with bidirectional isolation between Subnet1 and Subnet2."
fi

