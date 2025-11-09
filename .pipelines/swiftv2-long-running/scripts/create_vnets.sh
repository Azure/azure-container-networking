#!/usr/bin/env bash
set -e
trap 'echo "[ERROR] Failed while creating VNets or subnets. Check Azure CLI logs above." >&2' ERR

SUBSCRIPTION_ID=$1
LOCATION=$2
RG=$3
BUILD_ID=$4

# VNets and subnets
VNET_A1="cx_vnet_a1"
VNET_A2="cx_vnet_a2"
VNET_A3="cx_vnet_a3"
VNET_B1="cx_vnet_b1"

A2_MAIN="10.11.1.0/24"
A3_MAIN="10.12.1.0/24"
B1_MAIN="10.20.1.0/24"

az account set --subscription "$SUBSCRIPTION_ID"

# -------------------------------
# Verification functions
# -------------------------------
verify_vnet() {
  local rg="$1"; local vnet="$2"
  echo "==> Verifying VNet: $vnet"
  if az network vnet show -g "$rg" -n "$vnet" &>/dev/null; then
    echo "[OK] Verified VNet $vnet exists."
  else
    echo "[ERROR] VNet $vnet not found!" >&2
    exit 1
  fi
}

verify_subnet() {
  local rg="$1"; local vnet="$2"; local subnet="$3"
  echo "==> Verifying subnet: $subnet in $vnet"
  if az network vnet subnet show -g "$rg" --vnet-name "$vnet" -n "$subnet" &>/dev/null; then
    echo "[OK] Verified subnet $subnet exists in $vnet."
  else
    echo "[ERROR] Subnet $subnet not found in $vnet!" >&2
    exit 1
  fi
}

# -------------------------------
#  Create VNets and Subnets
# -------------------------------
# Create first vnet for customer "A". VnetA1
make -C ./hack/aks swift-delegated-subnet-up \
  AZCLI=az REGION=$LOCATION GROUP=$RG VNET=$VNET_A1 EXTRA_SUBNETS="s1 s2 pe" \
  && echo "Created $VNET_A1 with subnet pe"

verify_vnet "$RG" "$VNET_A1"
for sn in s1 s2 pe; do 
    verify_subnet "$RG" "$VNET_A1" "$sn"; 
    cluster_name="${BUILD_ID}-${VNET_A1}-${sn}"
    make -C ./hack/aks swiftv2-dummy-cluster-subnet-delegator-up \
        AZCLI=az CLUSTER=$cluster_name GROUP=$RG REGION=$LOCATION \
        SUB=$SUBSCRIPTION_ID VNET=$VNET_A1 POD_SUBNET=$sn \
        && echo "Created dummy cluster for $VNET_A1 subnet $sn"
done

# Create second vnet for customer "A". VnetA2
make -C ./hack/aks swift-delegated-subnet-up \
  AZCLI=az REGION=$LOCATION GROUP=$RG VNET=$VNET_A2 EXTRA_SUBNETS="s1" \
  && echo "Created $VNET_A2 with subnet s1"

verify_vnet "$RG" "$VNET_A2"
verify_subnet "$RG" "$VNET_A2" "s1"
cluster_name="${BUILD_ID}-${VNET_A2}-s1"
    make -C ./hack/aks swiftv2-dummy-cluster-subnet-delegator-up \
        AZCLI=az CLUSTER=$cluster_name GROUP=$RG REGION=$LOCATION \
        SUB=$SUBSCRIPTION_ID VNET=$VNET_A2 POD_SUBNET="s1" \
        && echo "Created dummy cluster for $VNET_A2 subnet s1"

# A3
az network vnet create -g "$RG" -n "$VNET_A3" --address-prefix 10.12.0.0/16 --subnet-name s1 --subnet-prefix "$A3_MAIN" -l "$LOCATION" --output none \
 && echo "Created $VNET_A3 with subnet s1"
verify_vnet "$RG" "$VNET_A3"
verify_subnet "$RG" "$VNET_A3" "s1"
cluster_name="${BUILD_ID}-${VNET_A3}-s1"
    make -C ./hack/aks swiftv2-dummy-cluster-subnet-delegator-up \
        AZCLI=az CLUSTER=$cluster_name GROUP=$RG REGION=$LOCATION \
        SUB=$SUBSCRIPTION_ID VNET=$VNET_A3 POD_SUBNET="s1" \
        && echo "Created dummy cluster for $VNET_A3 subnet s1"

# B1
az network vnet create -g "$RG" -n "$VNET_B1" --address-prefix 10.20.0.0/16 --subnet-name s1 --subnet-prefix "$B1_MAIN" -l "$LOCATION" --output none \
 && echo "Created $VNET_B1 with subnet s1"
verify_vnet "$RG" "$VNET_B1"
verify_subnet "$RG" "$VNET_B1" "s1"
cluster_name="${BUILD_ID}-${VNET_B1}-s1"
    make -C ./hack/aks swiftv2-dummy-cluster-subnet-delegator-up \
        AZCLI=az CLUSTER=$cluster_name GROUP=$RG REGION=$LOCATION \
        SUB=$SUBSCRIPTION_ID VNET=$VNET_B1 POD_SUBNET="s1" \
        && echo "Created dummy cluster for $VNET_B1 subnet s1"

echo " All VNets and subnets created and verified successfully."
