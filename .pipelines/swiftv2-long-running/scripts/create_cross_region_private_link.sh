#!/usr/bin/env bash
set -euo pipefail
trap 'echo "[ERROR] Failed during cross-region Private Link setup." >&2' ERR

SUBSCRIPTION_ID=$1
SOURCE_LOCATION=$2
SOURCE_RG=$3
STAGE_LABEL=$4
PEER_LOCATION=$5
PEER_RG=$6

SCRIPT_DIR=$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd)
# shellcheck source=.pipelines/swiftv2-long-running/scripts/cross_region_common.sh
source "$SCRIPT_DIR/cross_region_common.sh"

require_cross_region_commands
az account set --subscription "$SUBSCRIPTION_ID"

SOURCE_CUSTOMER_VNET_ID=$(az network vnet show \
    --resource-group "$SOURCE_RG" \
    --name cx_vnet_v1 \
    --subscription "$SUBSCRIPTION_ID" \
    --query id -o tsv)
SOURCE_AKS_NODE_SUBNET_ID=$(az aks show \
    --resource-group "$SOURCE_RG" \
    --name aks-1 \
    --subscription "$SUBSCRIPTION_ID" \
    --query "agentPoolProfiles[0].vnetSubnetId" -o tsv)
SOURCE_AKS_VNET_ID=${SOURCE_AKS_NODE_SUBNET_ID%/subnets/*}

if ! az network private-dns zone show \
    --resource-group "$SOURCE_RG" \
    --name "$CROSS_REGION_PRIVATE_DNS_ZONE" \
    --subscription "$SUBSCRIPTION_ID" \
    --output none 2>/dev/null; then
    az network private-dns zone create \
        --resource-group "$SOURCE_RG" \
        --name "$CROSS_REGION_PRIVATE_DNS_ZONE" \
        --subscription "$SUBSCRIPTION_ID" \
        --output none
fi

ensure_cross_region_private_dns_link \
    "$SUBSCRIPTION_ID" \
    "$SOURCE_RG" \
    "$CROSS_REGION_PRIVATE_DNS_ZONE" \
    "$SOURCE_CUSTOMER_VNET_ID" \
    "cr-customer-source-link"
ensure_cross_region_private_dns_link \
    "$SUBSCRIPTION_ID" \
    "$SOURCE_RG" \
    "$CROSS_REGION_PRIVATE_DNS_ZONE" \
    "$SOURCE_AKS_VNET_ID" \
    "cr-infra-source-link"

DNS_ZONE_ID=$(az network private-dns zone show \
    --resource-group "$SOURCE_RG" \
    --name "$CROSS_REGION_PRIVATE_DNS_ZONE" \
    --subscription "$SUBSCRIPTION_ID" \
    --query id -o tsv)

ensure_private_link_target() {
    local role="$1"
    local vnet_name="$2"
    local account_name endpoint_name account_id subnet_id existing_service_id existing_subnet_id
    local connection_state nic_id private_ip

    account_name=$(cross_region_storage_account_name \
        "$SUBSCRIPTION_ID" "$STAGE_LABEL" "$PEER_LOCATION" "$role")
    endpoint_name="${account_name}-pe"

    ensure_cross_region_storage_account \
        "$SUBSCRIPTION_ID" \
        "$PEER_RG" \
        "$PEER_LOCATION" \
        "$account_name" \
        "$role" \
        "$STAGE_LABEL"
    ensure_cross_region_test_blob \
        "$SUBSCRIPTION_ID" \
        "$PEER_RG" \
        "$account_name" \
        "Hello from cross-region Private Link ($role): $SOURCE_LOCATION to $PEER_LOCATION"

    az storage account update \
        --name "$account_name" \
        --resource-group "$PEER_RG" \
        --subscription "$SUBSCRIPTION_ID" \
        --public-network-access Disabled \
        --default-action Deny \
        --bypass None \
        --output none

    account_id=$(az storage account show \
        --name "$account_name" \
        --resource-group "$PEER_RG" \
        --subscription "$SUBSCRIPTION_ID" \
        --query id -o tsv)
    subnet_id=$(az network vnet subnet show \
        --resource-group "$PEER_RG" \
        --vnet-name "$vnet_name" \
        --name "$CROSS_REGION_PE_SUBNET" \
        --subscription "$SUBSCRIPTION_ID" \
        --query id -o tsv)

    if az network private-endpoint show \
        --resource-group "$PEER_RG" \
        --name "$endpoint_name" \
        --subscription "$SUBSCRIPTION_ID" \
        --output none 2>/dev/null; then
        existing_service_id=$(az network private-endpoint show \
            --resource-group "$PEER_RG" \
            --name "$endpoint_name" \
            --subscription "$SUBSCRIPTION_ID" \
            --query "privateLinkServiceConnections[0].privateLinkServiceId" -o tsv)
        existing_subnet_id=$(az network private-endpoint show \
            --resource-group "$PEER_RG" \
            --name "$endpoint_name" \
            --subscription "$SUBSCRIPTION_ID" \
            --query "subnet.id" -o tsv)
        if [[ "${existing_service_id,,}" != "${account_id,,}" || "${existing_subnet_id,,}" != "${subnet_id,,}" ]]; then
            echo "[ERROR] Private Endpoint $endpoint_name targets an unexpected resource or subnet." >&2
            return 1
        fi
    else
        az network private-endpoint create \
            --resource-group "$PEER_RG" \
            --name "$endpoint_name" \
            --location "$PEER_LOCATION" \
            --subscription "$SUBSCRIPTION_ID" \
            --subnet "$subnet_id" \
            --private-connection-resource-id "$account_id" \
            --group-id blob \
            --connection-name "${endpoint_name}-connection" \
            --output none
    fi

    connection_state=$(az network private-endpoint show \
        --resource-group "$PEER_RG" \
        --name "$endpoint_name" \
        --subscription "$SUBSCRIPTION_ID" \
        --query "privateLinkServiceConnections[0].privateLinkServiceConnectionState.status" -o tsv)
    if [[ "$connection_state" != "Approved" ]]; then
        echo "[ERROR] Private Endpoint $endpoint_name connection state is $connection_state, expected Approved." >&2
        return 1
    fi

    az network private-endpoint dns-zone-group create \
        --resource-group "$PEER_RG" \
        --endpoint-name "$endpoint_name" \
        --name default \
        --subscription "$SUBSCRIPTION_ID" \
        --private-dns-zone "$DNS_ZONE_ID" \
        --zone-name blob \
        --output none

    nic_id=$(az network private-endpoint show \
        --resource-group "$PEER_RG" \
        --name "$endpoint_name" \
        --subscription "$SUBSCRIPTION_ID" \
        --query "networkInterfaces[0].id" -o tsv)
    private_ip=$(az network nic show \
        --ids "$nic_id" \
        --query "ipConfigurations[0].privateIPAddress" -o tsv)
    if [[ -z "$private_ip" ]]; then
        echo "[ERROR] Private Endpoint $endpoint_name has no private IP." >&2
        return 1
    fi

    echo "Private Link target $account_name is ready at $private_ip."
}

ensure_private_link_target "private-link-customer" "$CROSS_REGION_CUSTOMER_VNET"
ensure_private_link_target "private-link-infra" "$CROSS_REGION_INFRA_VNET"

ensure_cross_region_resource_group_lock "$SUBSCRIPTION_ID" "$PEER_RG"

echo "Cross-region Private Link targets are ready in $PEER_LOCATION."
