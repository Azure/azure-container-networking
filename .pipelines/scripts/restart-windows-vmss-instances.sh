#!/usr/bin/env bash

set -euo pipefail

readonly WINDOWS_SELECTOR="kubernetes.io/os=windows"
readonly POLL_INTERVAL_SECONDS=10
readonly NOT_READY_TIMEOUT_SECONDS=600
readonly READY_TIMEOUT_SECONDS=1200
readonly STABLE_TIMEOUT_SECONDS=180

fail_infrastructure() {
    echo "##vso[task.logissue type=error;code=WINDOWS_RESTART_INFRASTRUCTURE]$*" >&2
    echo "Infrastructure failure: $*" >&2
    exit 1
}

node_ready_status() {
    local status
    if ! status=$(kubectl get node "$1" -o jsonpath='{.status.conditions[?(@.type=="Ready")].status}' 2>/dev/null); then
        echo "LookupFailed"
        return
    fi
    echo "${status:-Unknown}"
}

wait_for_node_status() {
    local node=$1
    local expected=$2
    local timeout=$3
    local description=$4
    local deadline=$((SECONDS + timeout))

    while ((SECONDS < deadline)); do
        local status
        status=$(node_ready_status "$node")
        if [[ "$expected" == "NotReady" && "$status" != "True" && "$status" != "LookupFailed" ]] ||
            [[ "$expected" == "$status" ]]; then
            echo "Observed node $node $description"
            return 0
        fi
        sleep "$POLL_INTERVAL_SECONDS"
    done

    fail_infrastructure "node $node did not become $description within ${timeout}s"
}

starting_event_count() {
    local node=$1
    local events
    events=$(kubectl get events --all-namespaces \
        --field-selector "involvedObject.kind=Node,involvedObject.name=${node}" \
        -o custom-columns='REASON:.reason,COUNT:.count' --no-headers) ||
        fail_infrastructure "failed to inspect Starting events for node $node"
    awk '$1 == "Starting" { total += ($2 == "<none>" || $2 == "" ? 1 : $2) } END { print total + 0 }' <<< "$events"
}

parse_provider_id() {
    local provider_id=$1
    local -n resource_group_ref=$2
    local -n vmss_ref=$3
    local -n instance_ref=$4
    local -a parts
    local i

    IFS='/' read -ra parts <<< "${provider_id#*://}"
    for ((i = 0; i < ${#parts[@]}; i++)); do
        case "${parts[$i],,}" in
            resourcegroups)
                resource_group_ref=${parts[$((i + 1))]:-}
                ;;
            virtualmachinescalesets)
                vmss_ref=${parts[$((i + 1))]:-}
                ;;
            virtualmachines)
                instance_ref=${parts[$((i + 1))]:-}
                ;;
        esac
    done

    [[ -n "$resource_group_ref" && -n "$vmss_ref" && -n "$instance_ref" ]]
}

wait_for_instance_running() {
    local resource_group=$1
    local vmss=$2
    local instance=$3
    local deadline=$((SECONDS + READY_TIMEOUT_SECONDS))

    while ((SECONDS < deadline)); do
        local power_state
        power_state=$(az vmss get-instance-view \
            --resource-group "$resource_group" \
            --name "$vmss" \
            --instance-id "$instance" \
            --query "statuses[?starts_with(code, 'PowerState/')].code | [0]" \
            --output tsv 2>/dev/null || true)
        if [[ "$power_state" == "PowerState/running" ]]; then
            return 0
        fi
        sleep "$POLL_INTERVAL_SECONDS"
    done

    fail_infrastructure "VMSS instance ${vmss}/${instance} did not return to PowerState/running"
}

wait_for_node_components() {
    local node=$1
    local cluster_ip
    local pod_output
    local -a cns_pods
    local -a privileged_pods

    kubectl wait "node/${node}" --for=condition=Ready --timeout="${READY_TIMEOUT_SECONDS}s" ||
        fail_infrastructure "Windows node $node did not remain Ready while checking components"

    pod_output=$(kubectl get pods -n kube-system -l k8s-app=azure-cns-win \
        --field-selector "spec.nodeName=${node}" -o name) ||
        fail_infrastructure "failed to list azure-cns-win pods on node $node"
    [[ -n "$pod_output" ]] || fail_infrastructure "azure-cns-win pod is missing from Windows node $node"
    mapfile -t cns_pods <<< "$pod_output"
    kubectl wait -n kube-system --for=condition=Ready --timeout="${READY_TIMEOUT_SECONDS}s" "${cns_pods[@]}" ||
        fail_infrastructure "azure-cns-win did not become ready on Windows node $node"

    pod_output=$(kubectl get pods -n kube-system -l app=privileged-daemonset,os=windows \
        --field-selector "spec.nodeName=${node}" -o name) ||
        fail_infrastructure "failed to list privileged Windows pods on node $node"
    [[ -n "$pod_output" ]] || fail_infrastructure "privileged Windows pod is missing from node $node"
    mapfile -t privileged_pods <<< "$pod_output"
    kubectl wait -n kube-system --for=condition=Ready --timeout="${READY_TIMEOUT_SECONDS}s" "${privileged_pods[@]}" ||
        fail_infrastructure "privileged Windows pod did not become ready on node $node"

    cluster_ip=$(kubectl get service kubernetes -n default -o jsonpath='{.spec.clusterIP}') ||
        fail_infrastructure "failed to resolve the Kubernetes service ClusterIP"
    kubectl exec -n kube-system "${privileged_pods[0]#pod/}" -- powershell.exe -NoProfile -NonInteractive -Command \
        "\$service = Get-Service kubeproxy -ErrorAction Stop; if (\$service.Status -ne 'Running') { throw 'kubeproxy is not running' }; if (-not (Test-NetConnection -ComputerName '${cluster_ip}' -Port 443 -InformationLevel Quiet)) { throw 'Kubernetes service ClusterIP is unreachable' }" ||
        fail_infrastructure "kube-proxy or Kubernetes service connectivity is unhealthy on node $node"
}

verify_cluster_stability() {
    local expected_windows_nodes=$1
    local deadline=$((SECONDS + STABLE_TIMEOUT_SECONDS))

    while ((SECONDS < deadline)); do
        local ready_windows_nodes
        local cns_ready
        local privileged_ready

        ready_windows_nodes=$(kubectl get nodes -l "$WINDOWS_SELECTOR" \
            -o custom-columns='READY:.status.conditions[?(@.type=="Ready")].status' --no-headers |
            awk '$1 == "True" { count++ } END { print count + 0 }') ||
            fail_infrastructure "failed to inspect Windows node readiness"
        cns_ready=$(kubectl get daemonset -n kube-system -l k8s-app=azure-cns-win \
            -o jsonpath='{range .items[*]}{.status.numberReady}{"\n"}{end}' |
            awk '{ total += $1 } END { print total + 0 }') ||
            fail_infrastructure "failed to inspect azure-cns-win readiness"
        privileged_ready=$(kubectl get daemonset privileged-daemonset -n kube-system \
            -o jsonpath='{.status.numberReady}') ||
            fail_infrastructure "failed to inspect privileged Windows daemonset readiness"

        if [[ "$ready_windows_nodes" -ne "$expected_windows_nodes" ||
            "$cns_ready" -ne "$expected_windows_nodes" ||
            "$privileged_ready" -ne "$expected_windows_nodes" ]]; then
            fail_infrastructure "Windows nodes or required daemonsets became unstable after restart (nodes=${ready_windows_nodes}/${expected_windows_nodes}, azure-cns-win=${cns_ready}/${expected_windows_nodes}, privileged=${privileged_ready}/${expected_windows_nodes})"
        fi
        sleep "$POLL_INTERVAL_SECONDS"
    done
}

main() {
    local cluster_name=${1:?usage: restart-windows-vmss-instances.sh CLUSTER_NAME}
    local node_resource_group
    local node_output
    local -a nodes
    local expected_windows_nodes
    declare -A starting_events_before

    node_resource_group=$(az aks show --resource-group "$cluster_name" --name "$cluster_name" \
        --query nodeResourceGroup --output tsv) ||
        fail_infrastructure "AKS metadata lookup failed for cluster $cluster_name"
    [[ -n "$node_resource_group" ]] || fail_infrastructure "AKS node resource group could not be resolved"

    node_output=$(kubectl get nodes -l "$WINDOWS_SELECTOR" -o name) ||
        fail_infrastructure "failed to list Windows nodes"
    mapfile -t nodes < <(sed 's#^node/##' <<< "$node_output" | sed '/^$/d' | sort)
    expected_windows_nodes=${#nodes[@]}
    ((expected_windows_nodes > 0)) || fail_infrastructure "no Windows nodes were found"

    kubectl apply -f test/integration/manifests/load/privileged-daemonset-windows.yaml ||
        fail_infrastructure "failed to apply the privileged Windows daemonset"
    kubectl rollout status daemonset/privileged-daemonset -n kube-system --timeout="${READY_TIMEOUT_SECONDS}s" ||
        fail_infrastructure "privileged Windows daemonset did not become ready before restart"

    for node in "${nodes[@]}"; do
        local provider_id
        local resource_group=""
        local vmss=""
        local instance=""

        [[ "$(node_ready_status "$node")" == "True" ]] ||
            fail_infrastructure "Windows node $node was not Ready before restart"

        provider_id=$(kubectl get node "$node" -o jsonpath='{.spec.providerID}') ||
            fail_infrastructure "failed to read providerID for node $node"
        parse_provider_id "$provider_id" resource_group vmss instance ||
            fail_infrastructure "could not map node $node providerID '$provider_id' to a VMSS instance"
        [[ "${resource_group,,}" == "${node_resource_group,,}" ]] ||
            fail_infrastructure "node $node maps to unexpected resource group $resource_group (expected $node_resource_group)"

        starting_events_before[$node]=$(starting_event_count "$node")
        echo "Restarting Windows node $node as VMSS instance ${vmss}/${instance}"
        az vmss restart --resource-group "$resource_group" --name "$vmss" \
            --instance-ids "$instance" --no-wait ||
            fail_infrastructure "Azure rejected restart of VMSS instance ${vmss}/${instance}"

        wait_for_node_status "$node" "NotReady" "$NOT_READY_TIMEOUT_SECONDS" "NotReady"
        wait_for_instance_running "$resource_group" "$vmss" "$instance"
        wait_for_node_status "$node" "True" "$READY_TIMEOUT_SECONDS" "Ready"
        wait_for_node_components "$node"

        local starting_events_after
        starting_events_after=$(starting_event_count "$node")
        if ((starting_events_after - starting_events_before[$node] > 1)); then
            fail_infrastructure "node $node emitted repeated Starting events during one requested restart (before=${starting_events_before[$node]}, after=${starting_events_after})"
        fi
    done

    verify_cluster_stability "$expected_windows_nodes"

    for node in "${nodes[@]}"; do
        local starting_events_after
        starting_events_after=$(starting_event_count "$node")
        if ((starting_events_after - starting_events_before[$node] > 1)); then
            fail_infrastructure "node $node restarted unexpectedly after recovery (before=${starting_events_before[$node]}, after=${starting_events_after})"
        fi
    done

    kubectl get nodes -l "$WINDOWS_SELECTOR" -o wide
    kubectl get pods -n kube-system -l k8s-app=azure-cns-win -o wide
    kubectl get pods -n kube-system -l app=privileged-daemonset,os=windows -o wide
}

if [[ "${BASH_SOURCE[0]}" == "$0" ]]; then
    main "$@"
fi
