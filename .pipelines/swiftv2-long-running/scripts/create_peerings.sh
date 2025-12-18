#!/usr/bin/env bash
set -e
trap 'echo "[ERROR] Failed during VNet peering creation." >&2' ERR

RG=$1
VNET_A1="cx_vnet_v1"
VNET_A2="cx_vnet_v2"
VNET_A3="cx_vnet_v3"
VNET_B1="cx_vnet_v4"

verify_peering() {
  local rg="$1"; local vnet="$2"; local peering="$3"
  if az network vnet peering show -g "$rg" --vnet-name "$vnet" -n "$peering" --query "peeringState" -o tsv | grep -q "Connected"; then
    echo "[OK] Peering $peering on $vnet is Connected."
  else
    echo "[ERROR] Peering $peering on $vnet not found or not Connected!" >&2
    exit 1
  fi
}

peer_two_vnets() {
  local rg="$1"; local v1="$2"; local v2="$3"; local name12="$4"; local name21="$5"
  az network vnet peering create -g "$rg" -n "$name12" --vnet-name "$v1" --remote-vnet "$v2" --allow-vnet-access --output none \
    && echo "Created peering $name12"
  az network vnet peering create -g "$rg" -n "$name21" --vnet-name "$v2" --remote-vnet "$v1" --allow-vnet-access --output none \
    && echo "Created peering $name21"

  verify_peering "$rg" "$v1" "$name12"
  verify_peering "$rg" "$v2" "$name21"
}

peer_two_vnets "$RG" "$VNET_A1" "$VNET_A2" "A1-to-A2" "A2-to-A1"
peer_two_vnets "$RG" "$VNET_A2" "$VNET_A3" "A2-to-A3" "A3-to-A2"
peer_two_vnets "$RG" "$VNET_A1" "$VNET_A3" "A1-to-A3" "A3-to-A1"
echo "VNet peerings created and verified."
