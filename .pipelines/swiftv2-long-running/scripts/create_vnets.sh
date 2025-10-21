#!/usr/bin/env bash
set -e
trap 'echo "[ERROR] Failed while creating VNets or subnets. Check Azure CLI logs above." >&2' ERR

SUBSCRIPTION_ID=$1
LOCATION=$2
RG=$3

az account set --subscription "$SUBSCRIPTION_ID"

# VNets and subnets
VNET_A1="cx_vnet_a1"
VNET_A2="cx_vnet_a2"
VNET_A3="cx_vnet_a3"
VNET_B1="cx_vnet_b1"

A1_S1="10.10.1.0/24"
A1_S2="10.10.2.0/24"
A1_PE="10.10.100.0/24"

A2_MAIN="10.11.1.0/24"
A2_PE="10.11.100.0/24"

A3_MAIN="10.12.1.0/24"
A3_PE="10.12.100.0/24"

B1_MAIN="10.20.1.0/24"

# A1
az network vnet create -g "$RG" -n "$VNET_A1" --address-prefix 10.10.0.0/16 --subnet-name s1 --subnet-prefix "$A1_S1" -l "$LOCATION" --output none \
 && echo "Created $VNET_A1 with subnet s1"
az network vnet subnet create -g "$RG" --vnet-name "$VNET_A1" -n s2 --address-prefix "$A1_S2" --output none \
 && echo "Created $VNET_A1 with subnet s2"
az network vnet subnet create -g "$RG" --vnet-name "$VNET_A1" -n pe --address-prefix "$A1_PE" --output none \
 && echo "Created $VNET_A1 with subnet pe"

# A2
az network vnet create -g "$RG" -n "$VNET_A2" --address-prefix 10.11.0.0/16 --subnet-name s-A2 --subnet-prefix "$A2_MAIN" -l "$LOCATION" --output none \
 && echo "Created $VNET_A2 with subnet s-A2"
az network vnet subnet create -g "$RG" --vnet-name "$VNET_A2" -n pe --address-prefix "$A2_PE" --output none \
 && echo "Created $VNET_A2 with subnet pe"

# A3
az network vnet create -g "$RG" -n "$VNET_A3" --address-prefix 10.12.0.0/16 --subnet-name s-A3 --subnet-prefix "$A3_MAIN" -l "$LOCATION" --output none \
 && echo "Created $VNET_A3 with subnet s-A3"
az network vnet subnet create -g "$RG" --vnet-name "$VNET_A3" -n pe --address-prefix "$A3_PE" --output none \
 && echo "Created $VNET_A3 with subnet pe"

# B1
az network vnet create -g "$RG" -n "$VNET_B1" --address-prefix 10.20.0.0/16 --subnet-name s-B1 --subnet-prefix "$B1_MAIN" -l "$LOCATION" --output none \
 && echo "Created $VNET_B1 with subnet s-B1"

echo "All VNets and subnets created successfully."
