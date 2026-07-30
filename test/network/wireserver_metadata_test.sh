#!/bin/bash
# Optional per-pool parameterization: when POOL is set the throwaway pods are
# named "wget-${POOL}" and pinned to that agent pool via nodeSelector, so this
# test can run per-pool in parallel. When POOL is empty the behavior is
# unchanged (pod name "wget", no node selector).
POOL="${POOL:-}"
POD_NAME="wget"
OVERRIDES=()
if [ -n "$POOL" ]; then
    POD_NAME="wget-${POOL}"
    OVERRIDES=(--overrides="{\"spec\":{\"nodeSelector\":{\"agentpool\":\"${POOL}\"}}}")
fi

kubectl run "$POD_NAME" -it --rm --image busybox --restart Never "${OVERRIDES[@]}" -- wget --timeout=3 --header=Metadata:true "http://168.63.129.16/machine/plugins?comp=nmagent&type=getinterfaceinfov1"
if [ $? -eq 0 ]; then
    echo "wireserver connectivity expected to fail but succeeded"
    exit 1
fi

kubectl run "$POD_NAME" -it --rm --image busybox --restart Never "${OVERRIDES[@]}" -- wget --timeout=3 --header=Metadata:true "http://169.254.169.254/metadata/instance?api-version=2021-02-01"
if [ $? -ne 0 ]; then
    echo "metadata server connectivity expected to succeed but failed"
    exit 1
fi