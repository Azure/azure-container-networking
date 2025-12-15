#!/usr/bin/env bash
set -e
trap 'echo "[ERROR] Failed while creating VNets or subnets. Check Azure CLI logs above." >&2' ERR

SUB_ID=$1
LOCATION=$2
RG=$3
BUILD_ID=$4
DELEGATOR_APP_NAME=$5
DELEGATOR_RG=$6
DELEGATOR_SUB=$7
DELEGATOR_BASE_URL=${8:-"http://localhost:8080"}  # Default to localhost:8080 if not provided

VNAMES=( "cx_vnet_v1" "cx_vnet_v2" "cx_vnet_v3" "cx_vnet_v4" )
VCIDRS=( "172.16.0.0/16" "172.17.0.0/16" "172.18.0.0/16" "172.19.0.0/16" )
NODE_SUBNETS=( "172.16.0.0/24" "172.17.0.0/24" "172.18.0.0/24" "172.19.0.0/24" )
EXTRA_SUBNETS_LIST=( "s1 s2 pe" "s1" "s1" "s1" )
EXTRA_CIDRS_LIST=( "172.16.1.0/24,172.16.2.0/24,172.16.3.0/24" \
                   "172.17.1.0/24" \
                   "172.18.1.0/24" \
                   "172.19.1.0/24" )
az account set --subscription "$SUB_ID"

verify_vnet() {
  local vnet="$1"
  echo "Verifying VNet: $vnet"
  if az network vnet show -g "$RG" -n "$vnet" &>/dev/null; then
    echo "[OK] Verified VNet $vnet exists."
  else
    echo "[ERROR] VNet $vnet not found!" >&2
    exit 1
  fi
}

verify_subnet() {
  local vnet="$1"; local subnet="$2"
  echo "Verifying subnet: $subnet in $vnet"
  if az network vnet subnet show -g "$RG" --vnet-name "$vnet" -n "$subnet" &>/dev/null; then
    echo "[OK] Verified subnet $subnet exists in $vnet."
  else
    echo "[ERROR] Subnet $subnet not found in $vnet!" >&2
    exit 1
  fi
}

create_vnet_subets() { 
  local vnet="$1"
  local vnet_cidr="$2"
  local node_subnet_cidr="$3"
  local extra_subnets="$4"
  local extra_cidrs="$5"

  echo "Creating VNet: $vnet with CIDR: $vnet_cidr"
  az network vnet create -g "$RG" -l "$LOCATION" --name "$vnet" --address-prefixes "$vnet_cidr" -o none

  IFS=' ' read -r -a extra_subnet_array <<< "$extra_subnets"
  IFS=',' read -r -a extra_cidr_array <<< "$extra_cidrs"

  for i in "${!extra_subnet_array[@]}"; do
    subnet_name="${extra_subnet_array[$i]}"
    subnet_cidr="${extra_cidr_array[$i]}"
    echo "Creating extra subnet: $subnet_name with CIDR: $subnet_cidr"
    
    # Only delegate pod subnets (not private endpoint subnets)
    if [[ "$subnet_name" != "pe" ]]; then
      az network vnet subnet create -g "$RG" \
         --vnet-name "$vnet" --name "$subnet_name" \
         --delegations Microsoft.SubnetDelegator/msfttestclients \
         --address-prefixes "$subnet_cidr" -o none
    else
      az network vnet subnet create -g "$RG" \
         --vnet-name "$vnet" --name "$subnet_name" \
         --address-prefixes "$subnet_cidr" -o none
    fi
  done
}

delegate_subnet() {
    local vnet="$1"
    local subnet="$2"
    local max_attempts=7
    local attempt=1
    
    echo "Delegating subnet: $subnet in VNet: $vnet to Subnet Delegator"
    subnet_id=$(az network vnet subnet show -g "$RG" --vnet-name "$vnet" -n "$subnet" --query id -o tsv)
    modified_custsubnet="${subnet_id//\//%2F}"
    
    responseFile="delegate_response.txt"
    cmd_delegator_curl="'curl -X PUT ${DELEGATOR_BASE_URL}/DelegatedSubnet/$modified_custsubnet'"
    cmd_containerapp_exec="az containerapp exec -n $DELEGATOR_APP_NAME -g $DELEGATOR_RG --subscription $DELEGATOR_SUB --command $cmd_delegator_curl"
    
    while [ $attempt -le $max_attempts ]; do
        echo "Attempt $attempt of $max_attempts..."
        script --quiet -c "$cmd_containerapp_exec" "$responseFile"
        
        if grep -qF "success" "$responseFile"; then
            echo "Subnet Delegator registered the subnet"
            rm -f "$responseFile"
            return 0
        else
            echo "Subnet Delegator failed to register the subnet (attempt $attempt)"
            cat "$responseFile"
            if [ $attempt -lt $max_attempts ]; then
                echo "Retrying in 5 seconds..."
                sleep 5
            fi
        fi
        
        ((attempt++))
    done
    
    echo "[ERROR] Failed to delegate subnet after $max_attempts attempts"
    rm -f "$responseFile"
    exit 1
}

for i in "${!VNAMES[@]}"; do
    VNET=${VNAMES[$i]}
    VNET_CIDR=${VCIDRS[$i]}
    NODE_SUBNET_CIDR=${NODE_SUBNETS[$i]}
    EXTRA_SUBNETS=${EXTRA_SUBNETS_LIST[$i]}
    EXTRA_SUBNET_CIDRS=${EXTRA_CIDRS_LIST[$i]}

    create_vnet_subets "$VNET" "$VNET_CIDR" "$NODE_SUBNET_CIDR" "$EXTRA_SUBNETS" "$EXTRA_SUBNET_CIDRS"
    verify_vnet "$VNET"  
    for PODSUBNET in $EXTRA_SUBNETS; do
        verify_subnet "$VNET" "$PODSUBNET"
        if [[ "$PODSUBNET" != "pe" ]]; then
            delegate_subnet "$VNET" "$PODSUBNET"
        fi
    done
done

echo "All VNets and subnets created and verified successfully."