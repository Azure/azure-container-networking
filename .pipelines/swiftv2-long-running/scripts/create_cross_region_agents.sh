#!/usr/bin/env bash
set -euo pipefail
trap 'echo "[ERROR] Failed during cross-region agent setup." >&2' ERR

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
az group show \
    --name "$SOURCE_RG" \
    --subscription "$SUBSCRIPTION_ID" \
    --output none

ensure_agent() {
    local agent_name="$1"
    local vnet_name="$2"
    local role="$3"
    local subnet_id existing_image existing_subnet existing_command
    local provisioning_state instance_state private_ip listeners

    subnet_id=$(az network vnet subnet show \
        --resource-group "$PEER_RG" \
        --vnet-name "$vnet_name" \
        --name "$CROSS_REGION_AGENT_SUBNET" \
        --subscription "$SUBSCRIPTION_ID" \
        --query id -o tsv)

    if az container show \
        --resource-group "$PEER_RG" \
        --name "$agent_name" \
        --subscription "$SUBSCRIPTION_ID" \
        --output none 2>/dev/null; then
        existing_image=$(az container show \
            --resource-group "$PEER_RG" \
            --name "$agent_name" \
            --subscription "$SUBSCRIPTION_ID" \
            --query "containers[0].image" -o tsv)
        existing_subnet=$(az container show \
            --resource-group "$PEER_RG" \
            --name "$agent_name" \
            --subscription "$SUBSCRIPTION_ID" \
            --query "subnetIds[0].id" -o tsv)
        existing_command=$(az container show \
            --resource-group "$PEER_RG" \
            --name "$agent_name" \
            --subscription "$SUBSCRIPTION_ID" \
            --query "join(' ', containers[0].command)" -o tsv)
        if [[ "$existing_image" != "$CROSS_REGION_AGENT_IMAGE" ||
            "${existing_subnet,,}" != "${subnet_id,,}" ||
            "$existing_command" != "$CROSS_REGION_AGENT_COMMAND" ]]; then
            echo "[ERROR] Container group $agent_name has incompatible image, subnet, or command." >&2
            return 1
        fi
        echo "Container group $agent_name already exists."
    else
        az container create \
            --resource-group "$PEER_RG" \
            --name "$agent_name" \
            --location "$PEER_LOCATION" \
            --subscription "$SUBSCRIPTION_ID" \
            --image "$CROSS_REGION_AGENT_IMAGE" \
            --os-type Linux \
            --cpu 1 \
            --memory 1.5 \
            --restart-policy Always \
            --ip-address Private \
            --ports 8080 \
            --subnet "$subnet_id" \
            --command-line "$CROSS_REGION_AGENT_COMMAND" \
            --tags \
                "$CROSS_REGION_TAG_KEY=$role" \
                "swiftv2-cross-region-scenario=$STAGE_LABEL" \
                "swiftv2-cross-region-source=$SOURCE_LOCATION" \
            --output none
    fi

    for attempt in $(seq 1 60); do
        provisioning_state=$(az container show \
            --resource-group "$PEER_RG" \
            --name "$agent_name" \
            --subscription "$SUBSCRIPTION_ID" \
            --query provisioningState -o tsv)
        instance_state=$(az container show \
            --resource-group "$PEER_RG" \
            --name "$agent_name" \
            --subscription "$SUBSCRIPTION_ID" \
            --query "containers[0].instanceView.currentState.state" -o tsv)

        if [[ "$provisioning_state" == "Succeeded" && "$instance_state" == "Running" ]]; then
            break
        fi
        if [[ "$provisioning_state" == "Failed" || "$instance_state" == "Terminated" ]]; then
            echo "[ERROR] Container group $agent_name entered state $provisioning_state/$instance_state." >&2
            return 1
        fi
        echo "Waiting for $agent_name ($provisioning_state/$instance_state, attempt $attempt/60)..."
        sleep 10
    done

    if [[ "$provisioning_state" != "Succeeded" || "$instance_state" != "Running" ]]; then
        echo "[ERROR] Container group $agent_name did not become ready." >&2
        return 1
    fi

    private_ip=$(az container show \
        --resource-group "$PEER_RG" \
        --name "$agent_name" \
        --subscription "$SUBSCRIPTION_ID" \
        --query ipAddress.ip -o tsv)
    if [[ -z "$private_ip" ]]; then
        echo "[ERROR] Container group $agent_name has no private IP." >&2
        return 1
    fi

    listeners=$(az container exec \
        --resource-group "$PEER_RG" \
        --name "$agent_name" \
        --subscription "$SUBSCRIPTION_ID" \
        --exec-command "ss -ltn" 2>&1)
    if ! grep -qE 'LISTEN.+:8080[[:space:]]' <<<"$listeners"; then
        echo "[ERROR] Container group $agent_name is not listening on TCP 8080." >&2
        echo "$listeners" >&2
        return 1
    fi

    echo "Container group $agent_name is ready at $private_ip."
}

ensure_agent \
    "$CROSS_REGION_CUSTOMER_AGENT" \
    "$CROSS_REGION_CUSTOMER_VNET" \
    "customer-agent"
ensure_agent \
    "$CROSS_REGION_INFRA_AGENT" \
    "$CROSS_REGION_INFRA_VNET" \
    "infra-agent"

echo "Cross-region test agents are ready in $PEER_LOCATION."
