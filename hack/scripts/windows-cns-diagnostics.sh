#!/usr/bin/env bash

start_windows_cns_diagnostics() {
    local label=$1
    windows_diagnostic_root=$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)
    windows_diagnostic_dir="${BUILD_ARTIFACTSTAGINGDIRECTORY:?}/windows-repro/$label-$(date -u +%Y%m%dT%H%M%SZ)"
    mkdir -p "$windows_diagnostic_dir"
    bash "$windows_diagnostic_root/hack/scripts/watch-windows-cns.sh" "$windows_diagnostic_dir" > "$windows_diagnostic_dir/watcher.log" 2>&1 &
    windows_diagnostic_pid=$!
    trap 'finish_windows_cns_diagnostics $?' EXIT
}

finish_windows_cns_diagnostics() {
    local result=$1
    trap - EXIT
    if kill -0 "$windows_diagnostic_pid" 2>/dev/null; then
        kill "$windows_diagnostic_pid" || echo "Diagnostic watcher already stopped."
    else
        echo "##vso[task.logissue type=warning]Windows CNS diagnostic watcher exited early."
    fi
    wait "$windows_diagnostic_pid" || true
    if [ "$result" -ne 0 ]; then
        echo "Collecting node logs before this failed task can retry."
        if ! (cd "$windows_diagnostic_root/hack/scripts" &&
            acnLogs="$windows_diagnostic_dir/node-logs" cni=cniv2 timeout 300 bash collect-windows-logs.sh > "$windows_diagnostic_dir/node-collector.log" 2>&1); then
            echo "##vso[task.logissue type=warning]Node log collection failed or exceeded five minutes; see node-collector.log."
        fi
    fi
    exit "$result"
}
