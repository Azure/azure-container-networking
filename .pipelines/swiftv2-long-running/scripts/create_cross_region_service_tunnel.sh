#!/usr/bin/env bash
set -euo pipefail
trap 'echo "[ERROR] Failed during cross-region Service Tunnel setup." >&2' ERR

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

SOURCE_CUSTOMER_SUBNET_ID=$(az network vnet subnet show \
    --resource-group "$SOURCE_RG" \
    --vnet-name cx_vnet_v1 \
    --name lr \
    --subscription "$SUBSCRIPTION_ID" \
    --query id -o tsv)
SOURCE_AKS_NODE_SUBNET_ID=$(az aks show \
    --resource-group "$SOURCE_RG" \
    --name aks-1 \
    --subscription "$SUBSCRIPTION_ID" \
    --query "agentPoolProfiles[0].vnetSubnetId" -o tsv)
SOURCE_AKS_VNET_ID=${SOURCE_AKS_NODE_SUBNET_ID%/subnets/*}
SOURCE_AKS_VNET_NAME=${SOURCE_AKS_VNET_ID##*/}
SOURCE_INFRA_SUBNET_ID=$(az network vnet subnet show \
    --resource-group "$SOURCE_RG" \
    --vnet-name "$SOURCE_AKS_VNET_NAME" \
    --name podnet \
    --subscription "$SUBSCRIPTION_ID" \
    --query id -o tsv)

ensure_global_storage_endpoint() {
    local subnet_id="$1"
    local endpoint
    local has_global=false
    local -a endpoints=()

    mapfile -t endpoints < <(
        az network vnet subnet show \
            --ids "$subnet_id" \
            --query "serviceEndpoints[].service" -o tsv
    )
    for endpoint in "${endpoints[@]}"; do
        if [[ "$endpoint" == "Microsoft.Storage" ]]; then
            echo "[ERROR] Subnet $subnet_id already uses incompatible Microsoft.Storage endpoint." >&2
            return 1
        fi
        if [[ "$endpoint" == "Microsoft.Storage.Global" ]]; then
            has_global=true
        fi
    done

    if [[ "$has_global" == "true" ]]; then
        echo "Subnet $subnet_id already has Microsoft.Storage.Global."
    else
        endpoints+=("Microsoft.Storage.Global")
        az network vnet subnet update \
            --ids "$subnet_id" \
            --service-endpoints "${endpoints[@]}" \
            --output none
    fi

    endpoint=$(az network vnet subnet show \
        --ids "$subnet_id" \
        --query "serviceEndpoints[?service=='Microsoft.Storage.Global'].service | [0]" -o tsv)
    if [[ "$endpoint" != "Microsoft.Storage.Global" ]]; then
        echo "[ERROR] Microsoft.Storage.Global was not configured on $subnet_id." >&2
        return 1
    fi
}

ensure_global_storage_endpoint "$SOURCE_CUSTOMER_SUBNET_ID"
ensure_global_storage_endpoint "$SOURCE_INFRA_SUBNET_ID"

SERVICE_ACCOUNT=$(cross_region_storage_account_name \
    "$SUBSCRIPTION_ID" "$STAGE_LABEL" "$PEER_LOCATION" "service-tunnel")
ensure_cross_region_storage_account \
    "$SUBSCRIPTION_ID" \
    "$PEER_RG" \
    "$PEER_LOCATION" \
    "$SERVICE_ACCOUNT" \
    "service-tunnel" \
    "$STAGE_LABEL"
ensure_cross_region_test_blob \
    "$SUBSCRIPTION_ID" \
    "$PEER_RG" \
    "$SERVICE_ACCOUNT" \
    "Hello from cross-region Service Tunnel: $SOURCE_LOCATION to $PEER_LOCATION"

ensure_storage_network_rule() {
    local subnet_id="$1"
    local existing

    existing=$(az storage account network-rule list \
        --resource-group "$PEER_RG" \
        --account-name "$SERVICE_ACCOUNT" \
        --subscription "$SUBSCRIPTION_ID" \
        --query "virtualNetworkRules[?virtualNetworkResourceId=='$subnet_id'].virtualNetworkResourceId | [0]" -o tsv)
    if [[ -n "$existing" ]]; then
        echo "Storage network rule already exists for $subnet_id."
        return 0
    fi

    az storage account network-rule add \
        --resource-group "$PEER_RG" \
        --account-name "$SERVICE_ACCOUNT" \
        --subscription "$SUBSCRIPTION_ID" \
        --subnet "$subnet_id" \
        --output none
}

ensure_storage_network_rule "$SOURCE_CUSTOMER_SUBNET_ID"
ensure_storage_network_rule "$SOURCE_INFRA_SUBNET_ID"

mapfile -t IP_RULES < <(
    az storage account network-rule list \
        --resource-group "$PEER_RG" \
        --account-name "$SERVICE_ACCOUNT" \
        --subscription "$SUBSCRIPTION_ID" \
        --query "ipRules[].ipAddressOrRange" -o tsv
)
for ip_rule in "${IP_RULES[@]}"; do
    echo "Removing unintended IP rule $ip_rule from $SERVICE_ACCOUNT."
    az storage account network-rule remove \
        --resource-group "$PEER_RG" \
        --account-name "$SERVICE_ACCOUNT" \
        --subscription "$SUBSCRIPTION_ID" \
        --ip-address "$ip_rule" \
        --output none
done

az storage account update \
    --name "$SERVICE_ACCOUNT" \
    --resource-group "$PEER_RG" \
    --subscription "$SUBSCRIPTION_ID" \
    --public-network-access Enabled \
    --default-action Deny \
    --bypass None \
    --output none

DEFAULT_ACTION=$(az storage account network-rule list \
    --resource-group "$PEER_RG" \
    --account-name "$SERVICE_ACCOUNT" \
    --subscription "$SUBSCRIPTION_ID" \
    --query defaultAction -o tsv)
RULE_COUNT=$(az storage account network-rule list \
    --resource-group "$PEER_RG" \
    --account-name "$SERVICE_ACCOUNT" \
    --subscription "$SUBSCRIPTION_ID" \
    --query "length(virtualNetworkRules)" -o tsv)
if [[ "$DEFAULT_ACTION" != "Deny" || "$RULE_COUNT" != "2" ]]; then
    echo "[ERROR] Storage account $SERVICE_ACCOUNT does not have the expected network ACLs." >&2
    exit 1
fi

echo "Service Tunnel target $SERVICE_ACCOUNT is ready in $PEER_LOCATION."
