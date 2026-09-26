#!/usr/bin/env bash

set -euo pipefail

export NODE_RESTART_TIMEOUT_SECONDS=2

# shellcheck disable=SC1091
source "$(dirname "${BASH_SOURCE[0]}")/restart-nodes.sh"

resource_group=""
vmss=""
instance=""
parse_provider_id \
    "azure:///subscriptions/sub/resourceGroups/MC_test/providers/Microsoft.Compute/virtualMachineScaleSets/aksnpwin/virtualMachines/3" \
    resource_group vmss instance

[[ "$resource_group" == "MC_test" ]]
[[ "$vmss" == "aksnpwin" ]]
[[ "$instance" == "3" ]]

if parse_provider_id "azure:///subscriptions/sub/resourceGroups/MC_test" resource_group vmss instance; then
    echo "expected incomplete provider ID to fail" >&2
    exit 1
fi

kubectl() {
    return 1
}

[[ "$(node_ready_status "unavailable-node")" == "LookupFailed" ]]

if restart_node "unavailable-node"; then
    echo "expected restart of a non-Ready node to fail" >&2
    exit 1
fi

test_dir=$(mktemp -d)
trap 'rm -rf "$test_dir"' EXIT
echo 0 > "$test_dir/boot-id-calls"
echo 0 > "$test_dir/restart-calls"
echo 0 > "$test_dir/ready-waits"

kubectl() {
    case "$*" in
        "get node ready-node -o jsonpath={.status.conditions[?(@.type==\"Ready\")].status}")
            echo "True"
            ;;
        "get node ready-node -o jsonpath={.status.nodeInfo.bootID}")
            local calls
            calls=$(<"$test_dir/boot-id-calls")
            calls=$((calls + 1))
            echo "$calls" > "$test_dir/boot-id-calls"
            if ((calls < 3)); then
                echo "old-boot-id"
            else
                echo "new-boot-id"
            fi
            ;;
        "get node ready-node -o jsonpath={.spec.providerID}")
            echo "azure:///subscriptions/sub/resourceGroups/MC_test/providers/Microsoft.Compute/virtualMachineScaleSets/aksnp/virtualMachines/0"
            ;;
        "wait node/ready-node --for=condition=Ready --timeout=20m")
            echo 1 > "$test_dir/ready-waits"
            ;;
        *)
            echo "unexpected kubectl command: $*" >&2
            return 1
            ;;
    esac
}

az() {
    [[ "$*" == "vmss restart --resource-group MC_test --name aksnp --instance-ids 0" ]]
    echo 1 > "$test_dir/restart-calls"
}

sleep() {
    :
}

restart_node "ready-node"
[[ "$(<"$test_dir/restart-calls")" == "1" ]]
[[ "$(<"$test_dir/ready-waits")" == "1" ]]
[[ "$(<"$test_dir/boot-id-calls")" == "3" ]]
