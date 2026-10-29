#!/usr/bin/env bash
set -uo pipefail

out=${1:?output directory required}
mkdir -p "$out"
declare -A previous_state
deadline=$((SECONDS + 10800))
last_host_capture=0

capture() {
    local file=$1
    shift
    if ! timeout --signal=TERM --kill-after=5s 45s kubectl --request-timeout=15s "$@" > "$file" 2>&1; then
        echo "$(date -u +%FT%TZ) capture failed: $* (see $file)"
        return 1
    fi
}

while (( SECONDS < deadline )); do
    stamp=$(date -u +%Y%m%dT%H%M%SZ)
    snapshot="$out/$stamp"
    mkdir -p "$snapshot"
    capture "$snapshot/daemonset.json" get ds azure-cns-win -n kube-system -o json
    capture "$snapshot/events.json" get events -A -o json
    capture "$snapshot/nodes.json" get nodes -o json
    capture "$snapshot/all-pods.json" get pods -A -o json
    if ! capture "$snapshot/cns-pods.json" get pods -n kube-system -l k8s-app=azure-cns-win -o json; then
        sleep 15
        continue
    fi

    while IFS=$'\t' read -r pod uid; do
        [ -n "$pod" ] || continue
        pod_dir="$out/$pod-$uid"
        mkdir -p "$pod_dir"
        jq --arg uid "$uid" '.items[] | select(.metadata.uid == $uid)' "$snapshot/cns-pods.json" > "$pod_dir/latest.json"
        state=$(jq -c '[.status.initContainerStatuses, .status.containerStatuses, .metadata.deletionTimestamp]' "$pod_dir/latest.json")
        if [[ "${previous_state[$uid]:-}" != "$state" ]]; then
            previous_state[$uid]=$state
            cp "$pod_dir/latest.json" "$pod_dir/state-$stamp.json"
            capture "$pod_dir/describe-$stamp.txt" describe pod "$pod" -n kube-system
            echo "$stamp $pod $(jq -c '.status.initContainerStatuses' "$pod_dir/latest.json")"
            while read -r container; do
                capture "$pod_dir/$container-previous-$stamp.log" logs "$pod" -n kube-system -c "$container" --previous --timestamps --tail=1000 --limit-bytes=1048576
            done < <(jq -r '.spec.initContainers[]?.name, .spec.containers[]?.name' "$pod_dir/latest.json")
        fi
        while read -r container; do
            capture "$pod_dir/$container-current-$stamp.log" logs "$pod" -n kube-system -c "$container" --timestamps --tail=1000 --limit-bytes=1048576
        done < <(jq -r '.spec.initContainers[]?.name, .spec.containers[]?.name' "$pod_dir/latest.json")
        if grep -F 'Azure CNI returned endpoint state with diagnostics' "$pod_dir/"*-"$stamp.log" > "$pod_dir/endpoint-diagnostics-$stamp.txt"; then
            echo "$stamp endpoint-state diagnostics: pod=$pod uid=$uid"
            cat "$pod_dir/endpoint-diagnostics-$stamp.txt"
            capture "$pod_dir/endpoint-diagnostics-$stamp.json" get pod "$pod" -n kube-system -o json
        else
            rm "$pod_dir/endpoint-diagnostics-$stamp.txt"
        fi
    done < <(jq -r '.items[] | [.metadata.name, .metadata.uid] | @tsv' "$snapshot/cns-pods.json")

    if (( SECONDS - last_host_capture >= 60 )) && jq -e 'any(.items[]; any(.status.initContainerStatuses[]?; .state.terminated.exitCode != 0))' "$snapshot/cns-pods.json" > /dev/null; then
        last_host_capture=$SECONDS
        if capture "$snapshot/privileged-pods.json" get pods -n kube-system -l os=windows,app=privileged-daemonset -o json; then
            while IFS=$'\t' read -r pod node; do
                [ -n "$pod" ] || continue
                capture "$snapshot/$node-processes.txt" exec -n kube-system "$pod" -- powershell -NoProfile -Command 'Get-CimInstance Win32_Process | Where-Object { $_.Name -match "azure-vnet|azure-cns|dropgz|containerd|kubelet" } | Select-Object Name,ProcessId,ParentProcessId,CreationDate,CommandLine | Format-List; Get-Service containerd,kubelet -ErrorAction Continue'
                capture "$snapshot/$node-cni-tail.txt" exec -n kube-system "$pod" -- powershell -NoProfile -Command 'Get-Content C:\k\azure-vnet.log -Tail 300 -ErrorAction Continue; Get-Content C:\k\azurecns\azure-cns.log -Tail 100 -ErrorAction Continue'
            done < <(jq -r '.items[] | [.metadata.name, .spec.nodeName] | @tsv' "$snapshot/privileged-pods.json")
        fi
    fi
    sleep 15
done
