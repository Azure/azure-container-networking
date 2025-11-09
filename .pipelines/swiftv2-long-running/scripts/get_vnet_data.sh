#!/usr/bin/env bash
set -euo pipefail

# === FUNCTION: Get Subnet ARM ID ===
# Usage: get_subnet_arm_id <resource_group> <vnet_name> <subnet_name>
get_subnet_arm_id() {
    local rg="$1"
    local vnet="$2"
    local subnet="$3"

    az network vnet subnet show --resource-group "$rg" --vnet-name "$vnet" --name "$subnet" --query "id" -o tsv
}

# === FUNCTION: Get Subnet GUID (SAL token) ===
# Usage: get_subnet_guid <resource_group> <vnet_name> <subnet_name>
get_subnet_guid() {
    local rg="$1"
    local vnet="$2"
    local subnet="$3"

    local subnet_id
    subnet_id=$(get_subnet_arm_id "$rg" "$vnet" "$subnet")
    az resource show --ids "$subnet_id" --api-version 2023-09-01 --query "properties.serviceAssociationLinks[0].properties.subnetId" -o tsv
}

# === FUNCTION: Get VNET GUID ===
# Usage: get_vnet_guid <resource_group> <vnet_name>
# Extracts the GUID from the VNET resource ARM ID
get_vnet_guid() {
    local rg="$1"
    local vnet="$2"

    local vnet_id
    vnet_id=$(az network vnet show --resource-group "$rg" --name "$vnet" --query "id" -o tsv)
    echo "$vnet_id"
}
