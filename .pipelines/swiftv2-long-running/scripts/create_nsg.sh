#!/usr/bin/env bash
set -e

VNET_A1="delegated_vnet_a1"
S1_PREFIX="10.10.1.0/24"
S2_PREFIX="10.10.2.0/24"
NSG_NAME="${VNET_A1}-nsg"

az network nsg create -g "$RG" -n "$NSG_NAME" --output none
az network nsg rule create -g "$RG"
