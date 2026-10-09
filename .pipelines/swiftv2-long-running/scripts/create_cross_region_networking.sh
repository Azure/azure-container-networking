#!/usr/bin/env bash
set -euo pipefail
trap 'echo "[ERROR] Failed during cross-region networking setup." >&2' ERR

SUBSCRIPTION_ID=$1
SOURCE_LOCATION=$2
SOURCE_RG=$3
STAGE_LABEL=$4
PEER_LOCATION=$5
PEER_RG=$6
PIPELINE_NAME=${7:-"swiftv2-long-running"}

SCRIPT_DIR=$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd)
# shellcheck source=.pipelines/swiftv2-long-running/scripts/cross_region_common.sh
source "$SCRIPT_DIR/cross_region_common.sh"

require_cross_region_commands
az account set --subscription "$SUBSCRIPTION_ID"

if [[ "$SOURCE_LOCATION" == "$PEER_LOCATION" ]]; then
    echo "[ERROR] Source and peer locations must differ." >&2
    exit 1
fi

SOURCE_CUSTOMER_VNET="cx_vnet_v1"
SOURCE_CUSTOMER_VNET_ID=$(az network vnet show \
    --resource-group "$SOURCE_RG" \
    --name "$SOURCE_CUSTOMER_VNET" \
    --subscription "$SUBSCRIPTION_ID" \
    --query id -o tsv)
SOURCE_CUSTOMER_SUBNET_ID=$(az network vnet subnet show \
    --resource-group "$SOURCE_RG" \
    --vnet-name "$SOURCE_CUSTOMER_VNET" \
    --name lr \
    --subscription "$SUBSCRIPTION_ID" \
    --query id -o tsv)
SOURCE_AKS_SUBNET_ID=$(az aks show \
    --resource-group "$SOURCE_RG" \
    --name aks-1 \
    --subscription "$SUBSCRIPTION_ID" \
    --query "agentPoolProfiles[0].vnetSubnetId" -o tsv)
if [[ -z "$SOURCE_CUSTOMER_VNET_ID" || -z "$SOURCE_AKS_SUBNET_ID" ]]; then
    echo "[ERROR] Could not discover Region A customer or AKS networking." >&2
    exit 1
fi

SOURCE_AKS_VNET_ID=${SOURCE_AKS_SUBNET_ID%/subnets/*}
SOURCE_AKS_VNET_NAME=${SOURCE_AKS_VNET_ID##*/}
SOURCE_INFRA_SUBNET_ID=$(az network vnet subnet show \
    --resource-group "$SOURCE_RG" \
    --vnet-name "$SOURCE_AKS_VNET_NAME" \
    --name podnet \
    --subscription "$SUBSCRIPTION_ID" \
    --query id -o tsv)

mapfile -t SOURCE_CIDRS < <(
    {
        az network vnet show \
            --ids "$SOURCE_CUSTOMER_VNET_ID" \
            --query "addressSpace.addressPrefixes[]" -o tsv
        az network vnet show \
            --ids "$SOURCE_AKS_VNET_ID" \
            --query "addressSpace.addressPrefixes[]" -o tsv
    } | sed '/^$/d'
)

validate_cross_region_cidrs \
    "$CROSS_REGION_CUSTOMER_VNET_CIDR" \
    "$CROSS_REGION_INFRA_VNET_CIDR" \
    "${SOURCE_CIDRS[@]}"

if az group show \
    --name "$PEER_RG" \
    --subscription "$SUBSCRIPTION_ID" \
    --output none 2>/dev/null; then
    EXISTING_PEER_RG_LOCATION=$(az group show \
        --name "$PEER_RG" \
        --subscription "$SUBSCRIPTION_ID" \
        --query location -o tsv)
    if [[ "$EXISTING_PEER_RG_LOCATION" != "$PEER_LOCATION" ]]; then
        echo "[ERROR] Peer resource group $PEER_RG is in $EXISTING_PEER_RG_LOCATION, expected $PEER_LOCATION." >&2
        exit 1
    fi
fi

echo "==> Creating peer resource group $PEER_RG in $PEER_LOCATION"
az group create \
    --name "$PEER_RG" \
    --location "$PEER_LOCATION" \
    --subscription "$SUBSCRIPTION_ID" \
    --output none
PEER_RG_ID=$(az group show \
    --name "$PEER_RG" \
    --subscription "$SUBSCRIPTION_ID" \
    --query id -o tsv)
az tag update \
    --resource-id "$PEER_RG_ID" \
    --operation Merge \
    --tags \
        "swiftv2-cross-region-scenario=$STAGE_LABEL" \
        "swiftv2-cross-region-source=$SOURCE_LOCATION" \
        "swiftv2-cross-region-peer=$PEER_LOCATION" \
    --output none
ensure_cross_region_resource_group_policy "$SUBSCRIPTION_ID" "$PEER_RG" "$PIPELINE_NAME"

ensure_vnet() {
    local name="$1"
    local cidr="$2"
    local actual_location actual_cidrs

    if az network vnet show \
        --resource-group "$PEER_RG" \
        --name "$name" \
        --subscription "$SUBSCRIPTION_ID" \
        --output none 2>/dev/null; then
        actual_location=$(az network vnet show \
            --resource-group "$PEER_RG" \
            --name "$name" \
            --subscription "$SUBSCRIPTION_ID" \
            --query location -o tsv)
        actual_cidrs=$(az network vnet show \
            --resource-group "$PEER_RG" \
            --name "$name" \
            --subscription "$SUBSCRIPTION_ID" \
            --query "join(',', addressSpace.addressPrefixes)" -o tsv)
        if [[ "$actual_location" != "$PEER_LOCATION" || "$actual_cidrs" != "$cidr" ]]; then
            echo "[ERROR] VNet $name has incompatible location or address space." >&2
            exit 1
        fi
        echo "VNet $name already exists."
        return 0
    fi

    az network vnet create \
        --resource-group "$PEER_RG" \
        --name "$name" \
        --location "$PEER_LOCATION" \
        --subscription "$SUBSCRIPTION_ID" \
        --address-prefixes "$cidr" \
        --output none
}

ensure_public_ip_and_nat() {
    local public_ip="$1"
    local nat_gateway="$2"
    local public_ip_id actual_location actual_sku actual_allocation
    local nat_location nat_sku nat_idle_timeout nat_public_ip

    if ! az network public-ip show \
        --resource-group "$PEER_RG" \
        --name "$public_ip" \
        --subscription "$SUBSCRIPTION_ID" \
        --output none 2>/dev/null; then
        az network public-ip create \
            --resource-group "$PEER_RG" \
            --name "$public_ip" \
            --location "$PEER_LOCATION" \
            --subscription "$SUBSCRIPTION_ID" \
            --sku Standard \
            --allocation-method Static \
            --output none
    fi

    IFS='|' read -r actual_location actual_sku actual_allocation < <(
        az network public-ip show \
            --resource-group "$PEER_RG" \
            --name "$public_ip" \
            --subscription "$SUBSCRIPTION_ID" \
            --query "join('|', [location, sku.name, publicIPAllocationMethod])" -o tsv
    )
    if [[ "$actual_location" != "$PEER_LOCATION" ||
        "$actual_sku" != "Standard" ||
        "$actual_allocation" != "Static" ]]; then
        echo "[ERROR] Public IP $public_ip has incompatible location, SKU, or allocation." >&2
        exit 1
    fi
    public_ip_id=$(az network public-ip show \
        --resource-group "$PEER_RG" \
        --name "$public_ip" \
        --subscription "$SUBSCRIPTION_ID" \
        --query id -o tsv)

    if ! az network nat gateway show \
        --resource-group "$PEER_RG" \
        --name "$nat_gateway" \
        --subscription "$SUBSCRIPTION_ID" \
        --output none 2>/dev/null; then
        az network nat gateway create \
            --resource-group "$PEER_RG" \
            --name "$nat_gateway" \
            --location "$PEER_LOCATION" \
            --subscription "$SUBSCRIPTION_ID" \
            --public-ip-addresses "$public_ip" \
            --idle-timeout 10 \
            --output none
    fi

    IFS='|' read -r nat_location nat_sku nat_idle_timeout nat_public_ip < <(
        az network nat gateway show \
            --resource-group "$PEER_RG" \
            --name "$nat_gateway" \
            --subscription "$SUBSCRIPTION_ID" \
            --query "join('|', [location, sku.name, to_string(idleTimeoutInMinutes), publicIpAddresses[0].id])" -o tsv
    )
    if [[ "$nat_location" != "$PEER_LOCATION" ||
        "$nat_sku" != "Standard" ||
        "$nat_idle_timeout" != "10" ||
        "${nat_public_ip,,}" != "${public_ip_id,,}" ]]; then
        echo "[ERROR] NAT gateway $nat_gateway has incompatible location, SKU, timeout, or public IP." >&2
        exit 1
    fi
}

ensure_agent_nsg() {
    local nsg="$1"
    local allowed_source="$2"

    if ! az network nsg show \
        --resource-group "$PEER_RG" \
        --name "$nsg" \
        --subscription "$SUBSCRIPTION_ID" \
        --output none 2>/dev/null; then
        az network nsg create \
            --resource-group "$PEER_RG" \
            --name "$nsg" \
            --location "$PEER_LOCATION" \
            --subscription "$SUBSCRIPTION_ID" \
            --output none
    fi

    local actual_location
    actual_location=$(az network nsg show \
        --resource-group "$PEER_RG" \
        --name "$nsg" \
        --subscription "$SUBSCRIPTION_ID" \
        --query location -o tsv)
    if [[ "$actual_location" != "$PEER_LOCATION" ]]; then
        echo "[ERROR] NSG $nsg is in $actual_location, expected $PEER_LOCATION." >&2
        exit 1
    fi

    az network nsg rule create \
        --resource-group "$PEER_RG" \
        --nsg-name "$nsg" \
        --name AllowMatchingRegionA \
        --subscription "$SUBSCRIPTION_ID" \
        --priority 100 \
        --direction Inbound \
        --access Allow \
        --protocol Tcp \
        --source-address-prefixes "$allowed_source" \
        --source-port-ranges '*' \
        --destination-address-prefixes '*' \
        --destination-port-ranges 8080 \
        --output none

    az network nsg rule create \
        --resource-group "$PEER_RG" \
        --nsg-name "$nsg" \
        --name DenyOtherTcp8080 \
        --subscription "$SUBSCRIPTION_ID" \
        --priority 110 \
        --direction Inbound \
        --access Deny \
        --protocol Tcp \
        --source-address-prefixes '*' \
        --source-port-ranges '*' \
        --destination-address-prefixes '*' \
        --destination-port-ranges 8080 \
        --output none
}

ensure_agent_subnet() {
    local vnet="$1"
    local cidr="$2"
    local nsg="$3"
    local nat_gateway="$4"

    if ! az network vnet subnet show \
        --resource-group "$PEER_RG" \
        --vnet-name "$vnet" \
        --name "$CROSS_REGION_AGENT_SUBNET" \
        --subscription "$SUBSCRIPTION_ID" \
        --output none 2>/dev/null; then
        az network vnet subnet create \
            --resource-group "$PEER_RG" \
            --vnet-name "$vnet" \
            --name "$CROSS_REGION_AGENT_SUBNET" \
            --subscription "$SUBSCRIPTION_ID" \
            --address-prefixes "$cidr" \
            --delegations Microsoft.ContainerInstance/containerGroups \
            --network-security-group "$nsg" \
            --nat-gateway "$nat_gateway" \
            --output none
    else
        local actual_cidr
        actual_cidr=$(az network vnet subnet show \
            --resource-group "$PEER_RG" \
            --vnet-name "$vnet" \
            --name "$CROSS_REGION_AGENT_SUBNET" \
            --subscription "$SUBSCRIPTION_ID" \
            --query addressPrefix -o tsv)
        if [[ "$actual_cidr" != "$cidr" ]]; then
            echo "[ERROR] Agent subnet in $vnet has incompatible address prefix $actual_cidr." >&2
            exit 1
        fi
        az network vnet subnet update \
            --resource-group "$PEER_RG" \
            --vnet-name "$vnet" \
            --name "$CROSS_REGION_AGENT_SUBNET" \
            --subscription "$SUBSCRIPTION_ID" \
            --delegations Microsoft.ContainerInstance/containerGroups \
            --network-security-group "$nsg" \
            --nat-gateway "$nat_gateway" \
            --output none
    fi
}

ensure_private_endpoint_subnet() {
    local vnet="$1"
    local cidr="$2"

    if ! az network vnet subnet show \
        --resource-group "$PEER_RG" \
        --vnet-name "$vnet" \
        --name "$CROSS_REGION_PE_SUBNET" \
        --subscription "$SUBSCRIPTION_ID" \
        --output none 2>/dev/null; then
        az network vnet subnet create \
            --resource-group "$PEER_RG" \
            --vnet-name "$vnet" \
            --name "$CROSS_REGION_PE_SUBNET" \
            --subscription "$SUBSCRIPTION_ID" \
            --address-prefixes "$cidr" \
            --disable-private-endpoint-network-policies true \
            --output none
    else
        local actual_cidr
        actual_cidr=$(az network vnet subnet show \
            --resource-group "$PEER_RG" \
            --vnet-name "$vnet" \
            --name "$CROSS_REGION_PE_SUBNET" \
            --subscription "$SUBSCRIPTION_ID" \
            --query addressPrefix -o tsv)
        if [[ "$actual_cidr" != "$cidr" ]]; then
            echo "[ERROR] Private Endpoint subnet in $vnet has incompatible address prefix $actual_cidr." >&2
            exit 1
        fi
        az network vnet subnet update \
            --resource-group "$PEER_RG" \
            --vnet-name "$vnet" \
            --name "$CROSS_REGION_PE_SUBNET" \
            --subscription "$SUBSCRIPTION_ID" \
            --disable-private-endpoint-network-policies true \
            --output none
    fi
}

ensure_peering() {
    local local_rg="$1"
    local local_vnet="$2"
    local remote_vnet_id="$3"
    local peering_name="$4"
    local existing_remote

    if az network vnet peering show \
        --resource-group "$local_rg" \
        --vnet-name "$local_vnet" \
        --name "$peering_name" \
        --subscription "$SUBSCRIPTION_ID" \
        --output none 2>/dev/null; then
        existing_remote=$(az network vnet peering show \
            --resource-group "$local_rg" \
            --vnet-name "$local_vnet" \
            --name "$peering_name" \
            --subscription "$SUBSCRIPTION_ID" \
            --query remoteVirtualNetwork.id -o tsv)
        if [[ "${existing_remote,,}" != "${remote_vnet_id,,}" ]]; then
            echo "[ERROR] Peering $peering_name points to unexpected VNet $existing_remote." >&2
            exit 1
        fi
        az network vnet peering update \
            --resource-group "$local_rg" \
            --vnet-name "$local_vnet" \
            --name "$peering_name" \
            --subscription "$SUBSCRIPTION_ID" \
            --allow-vnet-access true \
            --output none
        echo "Peering $peering_name already exists."
        return 0
    fi

    az network vnet peering create \
        --resource-group "$local_rg" \
        --vnet-name "$local_vnet" \
        --name "$peering_name" \
        --subscription "$SUBSCRIPTION_ID" \
        --remote-vnet "$remote_vnet_id" \
        --allow-vnet-access \
        --output none
}

wait_for_peering() {
    local resource_group="$1"
    local vnet="$2"
    local peering="$3"
    local attempt state

    for attempt in $(seq 1 30); do
        state=$(az network vnet peering show \
            --resource-group "$resource_group" \
            --vnet-name "$vnet" \
            --name "$peering" \
            --subscription "$SUBSCRIPTION_ID" \
            --query peeringState -o tsv)
        if [[ "$state" == "Connected" ]]; then
            echo "Peering $peering is Connected."
            return 0
        fi
        echo "Waiting for peering $peering (state: $state, attempt $attempt/30)..."
        sleep 10
    done

    echo "[ERROR] Peering $peering did not reach Connected state." >&2
    return 1
}

ensure_vnet "$CROSS_REGION_CUSTOMER_VNET" "$CROSS_REGION_CUSTOMER_VNET_CIDR"
ensure_vnet "$CROSS_REGION_INFRA_VNET" "$CROSS_REGION_INFRA_VNET_CIDR"

ensure_public_ip_and_nat "$CROSS_REGION_CUSTOMER_NAT_PIP" "$CROSS_REGION_CUSTOMER_NAT"
ensure_public_ip_and_nat "$CROSS_REGION_INFRA_NAT_PIP" "$CROSS_REGION_INFRA_NAT"

SOURCE_CUSTOMER_CIDR=$(az network vnet subnet show \
    --ids "$SOURCE_CUSTOMER_SUBNET_ID" \
    --query addressPrefix -o tsv)
SOURCE_INFRA_CIDR=$(az network vnet subnet show \
    --ids "$SOURCE_INFRA_SUBNET_ID" \
    --query addressPrefix -o tsv)

ensure_agent_nsg "$CROSS_REGION_CUSTOMER_NSG" "$SOURCE_CUSTOMER_CIDR"
ensure_agent_nsg "$CROSS_REGION_INFRA_NSG" "$SOURCE_INFRA_CIDR"

ensure_agent_subnet \
    "$CROSS_REGION_CUSTOMER_VNET" \
    "$CROSS_REGION_CUSTOMER_AGENT_CIDR" \
    "$CROSS_REGION_CUSTOMER_NSG" \
    "$CROSS_REGION_CUSTOMER_NAT"
ensure_agent_subnet \
    "$CROSS_REGION_INFRA_VNET" \
    "$CROSS_REGION_INFRA_AGENT_CIDR" \
    "$CROSS_REGION_INFRA_NSG" \
    "$CROSS_REGION_INFRA_NAT"
ensure_private_endpoint_subnet "$CROSS_REGION_CUSTOMER_VNET" "$CROSS_REGION_CUSTOMER_PE_CIDR"
ensure_private_endpoint_subnet "$CROSS_REGION_INFRA_VNET" "$CROSS_REGION_INFRA_PE_CIDR"

PEER_CUSTOMER_VNET_ID=$(az network vnet show \
    --resource-group "$PEER_RG" \
    --name "$CROSS_REGION_CUSTOMER_VNET" \
    --subscription "$SUBSCRIPTION_ID" \
    --query id -o tsv)
PEER_INFRA_VNET_ID=$(az network vnet show \
    --resource-group "$PEER_RG" \
    --name "$CROSS_REGION_INFRA_VNET" \
    --subscription "$SUBSCRIPTION_ID" \
    --query id -o tsv)

CUSTOMER_A_TO_B="cr-${STAGE_LABEL}-to-${PEER_LOCATION}-customer"
CUSTOMER_B_TO_A="cr-${PEER_LOCATION}-to-${STAGE_LABEL}-customer"
INFRA_A_TO_B="cr-${STAGE_LABEL}-to-${PEER_LOCATION}-infra"
INFRA_B_TO_A="cr-${PEER_LOCATION}-to-${STAGE_LABEL}-infra"

ensure_peering "$SOURCE_RG" "$SOURCE_CUSTOMER_VNET" "$PEER_CUSTOMER_VNET_ID" "$CUSTOMER_A_TO_B"
ensure_peering "$PEER_RG" "$CROSS_REGION_CUSTOMER_VNET" "$SOURCE_CUSTOMER_VNET_ID" "$CUSTOMER_B_TO_A"
ensure_peering "$SOURCE_RG" "$SOURCE_AKS_VNET_NAME" "$PEER_INFRA_VNET_ID" "$INFRA_A_TO_B"
ensure_peering "$PEER_RG" "$CROSS_REGION_INFRA_VNET" "$SOURCE_AKS_VNET_ID" "$INFRA_B_TO_A"

wait_for_peering "$SOURCE_RG" "$SOURCE_CUSTOMER_VNET" "$CUSTOMER_A_TO_B"
wait_for_peering "$PEER_RG" "$CROSS_REGION_CUSTOMER_VNET" "$CUSTOMER_B_TO_A"
wait_for_peering "$SOURCE_RG" "$SOURCE_AKS_VNET_NAME" "$INFRA_A_TO_B"
wait_for_peering "$PEER_RG" "$CROSS_REGION_INFRA_VNET" "$INFRA_B_TO_A"

echo "Cross-region networking is ready for $STAGE_LABEL ($SOURCE_LOCATION -> $PEER_LOCATION)."
