#!/usr/bin/env bash

set -euo pipefail

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
