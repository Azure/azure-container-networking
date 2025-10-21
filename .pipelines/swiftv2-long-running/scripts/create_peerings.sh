#!/usr/bin/env bash
set -e

VNET_A1="delegated_vnet_a1"
VNET_A2="delegated_vnet_a2"
VNET_A3="delegated_vnet_a3"

peer_two_vnets() {
  local rg="$1"; local v1="$2"; local v2="$3"; local name12="$4"; local name21="$5"
  az network vnet peering create -g "$rg" -n "$name12" --vnet-name "$v1" --remote-vnet "$v2" --allow-vnet-access --output none
  az network vnet peering create -g "$rg" -n "$name21" --vnet-name "$v2" --remote-vnet "$v1" --allow-vnet-access --output none
}

peer_two_vnets "$RG" "$VNET_A1" "$VNET_A2" "A1-to-A2" "A2-to-A1"
peer_two_vnets "$RG" "$VNET_A2" "$VNET_A3" "A2-to-A3" "A3-to-A2"
peer_two_vnets "$RG" "$VNET_A1" "$VNET_A3" "A1-to-A3" "A3-to-A1"
echo "VNet peerings created successfully."