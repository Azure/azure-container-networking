#!/usr/bin/env bash
set -e
trap 'echo "[ERROR] Failed while creating VNets or subnets. Check Azure CLI logs above." >&2' ERR

SUB_ID=$1
LOCATION=$2
RG=$3
BUILD_ID=$4

# --- VNet definitions ---
# Create customer vnets for two customers A and B.
VNAMES=( "cx_vnet_a1" "cx_vnet_a2" "cx_vnet_a3" "cx_vnet_b1" )
VCIDRS=( "10.10.0.0/16" "10.11.0.0/16" "10.12.0.0/16" "10.13.0.0/16" )
NODE_SUBNETS=( "10.10.0.0/24" "10.11.0.0/24" "10.12.0.0/24" "10.13.0.0/24" )
EXTRA_SUBNETS_LIST=( "s1 s2 pe" "s1" "s1" "s1" )
EXTRA_CIDRS_LIST=( "10.10.1.0/24,10.10.2.0/24,10.10.3.0/24" \
                   "10.11.1.0/24" \
                   "10.12.1.0/24" \
                   "10.13.1.0/24" )
az account set --subscription "$SUB_ID"

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

# --- Loop over VNets ---
for i in "${!VNAMES[@]}"; do
    VNET=${VNAMES[$i]}
    VNET_CIDR=${VCIDRS[$i]}
    NODE_SUBNET_CIDR=${NODE_SUBNETS[$i]}
    EXTRA_SUBNETS=${EXTRA_SUBNETS_LIST[$i]}
    EXTRA_SUBNET_CIDRS=${EXTRA_CIDRS_LIST[$i]}

    # Create VNet + subnets
    make -C ./hack/aks swift-delegated-subnet-up \
      AZCLI=az REGION=$LOCATION GROUP=$RG VNET=$VNET \
      VNET_CIDR=$VNET_CIDR NODE_SUBNET_CIDR=$NODE_SUBNET_CIDR \
      EXTRA_SUBNETS="$EXTRA_SUBNETS" EXTRA_SUBNET_CIDRS="$EXTRA_SUBNET_CIDRS" \
      && echo "Created $VNET with subnets $EXTRA_SUBNETS"

    verify_vnet "$RG" "$VNET"   # Verify VNet

    # Loop over extra subnets to verify and create dummy clusters to delegate the pod subnets.
    for PODSUBNET in $EXTRA_SUBNETS; do
        verify_subnet "$RG" "$VNET" "$PODSUBNET"
        cluster_name="${BUILD_ID}-${VNET}-${PODSUBNET}"
        make -C ./hack/aks swiftv2-dummy-cluster-subnet-delegator-up \
            AZCLI=az CLUSTER=$cluster_name GROUP=$RG REGION=$LOCATION \
            SUB=$SUB_ID VNET=$VNET POD_SUBNET=$PODSUBNET \
            && echo "Created dummy cluster for $VNET subnet $PODSUBNET"
    done
done

echo "All VNets and subnets created and verified successfully."