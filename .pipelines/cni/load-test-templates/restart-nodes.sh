#!/usr/bin/env bash

set -euo pipefail

readonly poll_interval_seconds=10
readonly not_ready_timeout_seconds=600
readonly ready_timeout="20m"

node_ready_status() {
    local node=$1
    local status
    if ! status=$(kubectl get node "$node" -o jsonpath='{.status.conditions[?(@.type=="Ready")].status}' 2>/dev/null); then
        echo "LookupFailed"
        return
    fi
    echo "${status:-Unknown}"
}

wait_for_node_not_ready() {
    local node=$1
    local deadline=$((SECONDS + not_ready_timeout_seconds))

    while ((SECONDS < deadline)); do
        local status
        status=$(node_ready_status "$node")
        if [[ "$status" != "True" && "$status" != "LookupFailed" ]]; then
            return 0
        fi
        sleep "$poll_interval_seconds"
    done

    echo "node $node did not become NotReady within ${not_ready_timeout_seconds}s" >&2
    return 1
}

parse_provider_id() {
    local provider_id=$1
    local -n resource_group_ref=$2
    local -n vmss_ref=$3
    local -n instance_ref=$4

    resource_group_ref=$(awk -F/ '{ for (i = 1; i <= NF; i++) if (tolower($i) == "resourcegroups") print $(i + 1) }' <<< "$provider_id")
    vmss_ref=$(awk -F/ '{ for (i = 1; i <= NF; i++) if (tolower($i) == "virtualmachinescalesets") print $(i + 1) }' <<< "$provider_id")
    instance_ref=$(awk -F/ '{ for (i = 1; i <= NF; i++) if (tolower($i) == "virtualmachines") print $(i + 1) }' <<< "$provider_id")

    [[ -n "$resource_group_ref" && -n "$vmss_ref" && -n "$instance_ref" ]]
}

restart_node() {
    local node=$1
    local provider_id
    local resource_group=""
    local vmss=""
    local instance=""

    provider_id=$(kubectl get node "$node" -o jsonpath='{.spec.providerID}')
    if ! parse_provider_id "$provider_id" resource_group vmss instance; then
        echo "failed to parse Azure provider ID for node $node: $provider_id" >&2
        return 1
    fi

    echo "Restarting node $node as VMSS instance ${vmss}/${instance}"
    az vmss restart --resource-group "$resource_group" --name "$vmss" --instance-ids "$instance" --no-wait
    wait_for_node_not_ready "$node"
    kubectl wait "node/$node" --for=condition=Ready --timeout="$ready_timeout"
}

main() {
    local node_output
    local -a nodes

    node_output=$(kubectl get nodes -o name)
    mapfile -t nodes < <(awk -F/ 'NF { print $NF }' <<< "$node_output" | sort)
    if ((${#nodes[@]} == 0)); then
        echo "no nodes found to restart" >&2
        return 1
    fi

    for node in "${nodes[@]}"; do
        restart_node "$node"
    done
}

if [[ "${BASH_SOURCE[0]}" == "$0" ]]; then
    main "$@"
fi
