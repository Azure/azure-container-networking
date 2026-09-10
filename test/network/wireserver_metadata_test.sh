#!/bin/bash
# The two connectivity checks each run a throwaway busybox pod named
# "${POD_NAME}-wireserver-${SUFFIX}" / "${POD_NAME}-metadata-${SUFFIX}", where
# POD_NAME is "wget" (or "wget-${POOL}" when POOL is set) and SUFFIX is a random
# value for uniqueness across retries. When POOL is set the pods are also pinned
# to that agent pool. The pods always target Linux nodes (kubernetes.io/os=linux)
# so the Linux busybox image is never scheduled onto a Windows node, where it
# cannot be pulled/run and the check would time out.
POOL="${POOL:-}"
POD_NAME="wget"
NODE_SELECTOR='"kubernetes.io/os":"linux"'
if [ -n "$POOL" ]; then
    POD_NAME="wget-${POOL}"
    NODE_SELECTOR="${NODE_SELECTOR},\"agentpool\":\"${POOL}\""
fi
OVERRIDES=(--overrides="{\"spec\":{\"nodeSelector\":{${NODE_SELECTOR}}}}")

SUFFIX="${RANDOM}"

kubectl run "$POD_NAME-wireserver-${SUFFIX}" -it --rm --image busybox --restart Never --pod-running-timeout=1m "${OVERRIDES[@]}" -- wget --timeout=3 --header=Metadata:true "http://168.63.129.16/machine/plugins?comp=nmagent&type=getinterfaceinfov1"
if [ $? -eq 0 ]; then
    echo "wireserver connectivity expected to fail but succeeded"
    exit 1
fi

kubectl run "$POD_NAME-metadata-${SUFFIX}" -it --rm --image busybox --restart Never --pod-running-timeout=1m "${OVERRIDES[@]}" -- wget --timeout=3 --header=Metadata:true "http://169.254.169.254/metadata/instance?api-version=2021-02-01"
if [ $? -ne 0 ]; then
    echo "metadata server connectivity expected to succeed but failed"
    exit 1
fi