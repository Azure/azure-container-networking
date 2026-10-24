#!/usr/bin/env bash

export CROSS_REGION_TAG_KEY="swiftv2-cross-region-role"
export CROSS_REGION_INIT_TAG="swiftv2-cross-region-initialized"
export CROSS_REGION_LOCK_NAME="pipeline-do-not-delete"
export CROSS_REGION_AGENT_IMAGE="nicolaka/netshoot@sha256:a20c2531bf35436ed3766cd6cfe89d352b050ccc4d7005ce6400adf97503da1b"
export CROSS_REGION_AGENT_COMMAND="socat TCP-LISTEN:8080,reuseaddr,fork EXEC:/bin/cat"

export CROSS_REGION_CUSTOMER_VNET="cr-customer-vnet"
export CROSS_REGION_INFRA_VNET="cr-infra-vnet"
export CROSS_REGION_CUSTOMER_VNET_CIDR="192.168.240.0/22"
export CROSS_REGION_INFRA_VNET_CIDR="192.168.244.0/22"

export CROSS_REGION_AGENT_SUBNET="agent"
export CROSS_REGION_PE_SUBNET="private-endpoints"
export CROSS_REGION_CUSTOMER_AGENT_CIDR="192.168.240.0/24"
export CROSS_REGION_CUSTOMER_PE_CIDR="192.168.241.0/24"
export CROSS_REGION_INFRA_AGENT_CIDR="192.168.244.0/24"
export CROSS_REGION_INFRA_PE_CIDR="192.168.245.0/24"

export CROSS_REGION_CUSTOMER_NSG="cr-customer-agent-nsg"
export CROSS_REGION_INFRA_NSG="cr-infra-agent-nsg"
export CROSS_REGION_CUSTOMER_NAT="cr-customer-agent-nat"
export CROSS_REGION_INFRA_NAT="cr-infra-agent-nat"
export CROSS_REGION_CUSTOMER_NAT_PIP="cr-customer-agent-nat-pip"
export CROSS_REGION_INFRA_NAT_PIP="cr-infra-agent-nat-pip"
export CROSS_REGION_CUSTOMER_AGENT="cr-customer-agent"
export CROSS_REGION_INFRA_AGENT="cr-infra-agent"

export CROSS_REGION_PRIVATE_DNS_ZONE="privatelink.blob.core.windows.net"
export CROSS_REGION_CONTAINER="test"
export CROSS_REGION_BLOB="hello.txt"

require_cross_region_commands() {
    local command
    for command in az jq python3 sha256sum; do
        if ! command -v "$command" >/dev/null 2>&1; then
            echo "[ERROR] Required command '$command' is not installed." >&2
            return 1
        fi
    done
}

cross_region_storage_account_name() {
    local subscription_id="$1"
    local stage_label="$2"
    local peer_location="$3"
    local role="$4"
    local suffix digest

    case "$role" in
        service-tunnel) suffix="st" ;;
        private-link-customer) suffix="pc" ;;
        private-link-infra) suffix="pi" ;;
        *)
            echo "[ERROR] Unsupported storage role '$role'." >&2
            return 1
            ;;
    esac

    digest=$(printf '%s' "${subscription_id}|${stage_label}|${peer_location}|${role}" | sha256sum | cut -c1-14)
    printf 'sv2cr%s%s\n' "$digest" "$suffix"
}

validate_cross_region_cidrs() {
    local customer_cidr="$1"
    local infra_cidr="$2"
    shift 2

    python3 - "$customer_cidr" "$infra_cidr" "$@" <<'PY'
import ipaddress
import sys

peer_networks = [ipaddress.ip_network(value) for value in sys.argv[1:3]]
source_networks = [ipaddress.ip_network(value) for value in sys.argv[3:]]

if peer_networks[0].overlaps(peer_networks[1]):
    raise SystemExit(
        f"peer address spaces overlap: {peer_networks[0]} and {peer_networks[1]}"
    )

for peer in peer_networks:
    for source in source_networks:
        if peer.overlaps(source):
            raise SystemExit(
                f"peer address space {peer} overlaps source address space {source}"
            )
PY
}

ensure_cross_region_resource_group_policy() {
    local subscription_id="$1"
    local resource_group="$2"
    local pipeline_name="$3"
    local deletion_due_time resource_group_id

    deletion_due_time=$(date -u -d "+14 days" "+%Y-%m-%dT%H:%M:%SZ")
    resource_group_id=$(az group show \
        --name "$resource_group" \
        --subscription "$subscription_id" \
        --query id -o tsv)

    az tag update \
        --resource-id "$resource_group_id" \
        --operation Merge \
        --tags \
            "deletion_due_time=$deletion_due_time" \
            "gc_scenario=$pipeline_name" \
            "gc_skip=true" \
            "swiftv2-cross-region=true" \
        --output none
}

ensure_cross_region_resource_group_lock() {
    local subscription_id="$1"
    local resource_group="$2"

    if az lock show \
        --name "$CROSS_REGION_LOCK_NAME" \
        --resource-group "$resource_group" \
        --subscription "$subscription_id" \
        --output none 2>/dev/null; then
        echo "Resource group lock already exists on $resource_group."
        return 0
    fi

    az lock create \
        --name "$CROSS_REGION_LOCK_NAME" \
        --resource-group "$resource_group" \
        --subscription "$subscription_id" \
        --lock-type CanNotDelete \
        --notes "Applied by SwiftV2 long-running cross-region provisioning" \
        --output none
}

ensure_cross_region_storage_account() {
    local subscription_id="$1"
    local resource_group="$2"
    local location="$3"
    local account_name="$4"
    local role="$5"
    local stage_label="$6"

    if az storage account show \
        --name "$account_name" \
        --resource-group "$resource_group" \
        --subscription "$subscription_id" \
        --output none 2>/dev/null; then
        local actual_location actual_role
        actual_location=$(az storage account show \
            --name "$account_name" \
            --resource-group "$resource_group" \
            --subscription "$subscription_id" \
            --query location -o tsv)
        actual_role=$(az storage account show \
            --name "$account_name" \
            --resource-group "$resource_group" \
            --subscription "$subscription_id" \
            --query "tags.\"$CROSS_REGION_TAG_KEY\"" -o tsv)

        if [[ "$actual_location" != "$location" || "$actual_role" != "$role" ]]; then
            echo "[ERROR] Storage account $account_name has incompatible location or role." >&2
            return 1
        fi
        echo "Storage account $account_name already exists."
        return 0
    fi

    local name_available
    name_available=$(az storage account check-name \
        --name "$account_name" \
        --subscription "$subscription_id" \
        --query nameAvailable -o tsv)
    if [[ "$name_available" != "true" ]]; then
        echo "[ERROR] Storage account name $account_name is unavailable outside $resource_group." >&2
        return 1
    fi

    az storage account create \
        --name "$account_name" \
        --resource-group "$resource_group" \
        --location "$location" \
        --subscription "$subscription_id" \
        --sku Standard_LRS \
        --kind StorageV2 \
        --allow-blob-public-access false \
        --allow-shared-key-access true \
        --https-only true \
        --min-tls-version TLS1_2 \
        --public-network-access Enabled \
        --default-action Allow \
        --tags \
            "$CROSS_REGION_TAG_KEY=$role" \
            "swiftv2-cross-region-scenario=$stage_label" \
        --output none
}

ensure_cross_region_test_blob() {
    local subscription_id="$1"
    local resource_group="$2"
    local account_name="$3"
    local content="$4"
    local initialized account_key account_id

    initialized=$(az storage account show \
        --name "$account_name" \
        --resource-group "$resource_group" \
        --subscription "$subscription_id" \
        --query "tags.\"$CROSS_REGION_INIT_TAG\"" -o tsv)
    if [[ "$initialized" == "true" ]]; then
        echo "Test blob for $account_name is already initialized."
        return 0
    fi

    echo "Initializing test blob for $account_name before applying network restrictions."
    az storage account update \
        --name "$account_name" \
        --resource-group "$resource_group" \
        --subscription "$subscription_id" \
        --public-network-access Enabled \
        --default-action Allow \
        --output none

    account_key=$(az storage account keys list \
        --account-name "$account_name" \
        --resource-group "$resource_group" \
        --subscription "$subscription_id" \
        --query '[0].value' -o tsv)
    if [[ -z "$account_key" ]]; then
        echo "[ERROR] Could not retrieve a key for storage account $account_name." >&2
        return 1
    fi

    AZURE_STORAGE_KEY="$account_key" az storage container create \
        --name "$CROSS_REGION_CONTAINER" \
        --account-name "$account_name" \
        --auth-mode key \
        --only-show-errors \
        --output none
    AZURE_STORAGE_KEY="$account_key" az storage blob upload \
        --account-name "$account_name" \
        --container-name "$CROSS_REGION_CONTAINER" \
        --name "$CROSS_REGION_BLOB" \
        --data "$content" \
        --auth-mode key \
        --overwrite true \
        --only-show-errors \
        --output none
    unset account_key

    account_id=$(az storage account show \
        --name "$account_name" \
        --resource-group "$resource_group" \
        --subscription "$subscription_id" \
        --query id -o tsv)
    az tag update \
        --resource-id "$account_id" \
        --operation Merge \
        --tags "$CROSS_REGION_INIT_TAG=true" \
        --output none
}

ensure_cross_region_private_dns_link() {
    local subscription_id="$1"
    local dns_resource_group="$2"
    local zone_name="$3"
    local vnet_id="$4"
    local link_name="$5"
    local existing_link

    existing_link=$(az network private-dns link vnet list \
        --resource-group "$dns_resource_group" \
        --zone-name "$zone_name" \
        --subscription "$subscription_id" \
        --query "[?virtualNetwork.id=='$vnet_id'].name | [0]" -o tsv)
    if [[ -n "$existing_link" ]]; then
        echo "Private DNS zone is already linked to $vnet_id as $existing_link."
        return 0
    fi

    az network private-dns link vnet create \
        --resource-group "$dns_resource_group" \
        --zone-name "$zone_name" \
        --name "$link_name" \
        --virtual-network "$vnet_id" \
        --registration-enabled false \
        --subscription "$subscription_id" \
        --output none
}
