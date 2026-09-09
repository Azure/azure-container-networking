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

SUFFIX="${RANDOM}"

debug_capture() {
    local pod="$1" node cns phase i
    for i in $(seq 1 30); do
        sleep 8
        kubectl -n default get pod "$pod" >/dev/null 2>&1 || return 0
        phase=$(kubectl -n default get pod "$pod" -o jsonpath='{.status.phase}' 2>/dev/null)
        case "$phase" in Running|Succeeded) return 0;; esac
        node=$(kubectl -n default get pod "$pod" -o jsonpath='{.spec.nodeName}' 2>/dev/null)
        if [ -z "$node" ]; then
            echo "DEBUG $pod phase=$phase not-yet-scheduled"
            continue
        fi
        echo "===== DEBUG $pod phase=$phase node=$node ====="
        kubectl -n default get pod "$pod" -o wide 2>&1
        echo "DEBUG $pod nodeSelector=$(kubectl -n default get pod "$pod" -o jsonpath='{.spec.nodeSelector}' 2>/dev/null)"
        kubectl -n default describe pod "$pod" 2>&1 | sed -n '/Events:/,$p'
        echo "DEBUG node $node os=$(kubectl get node "$node" -o jsonpath='{.metadata.labels.kubernetes\.io/os}' 2>/dev/null) taints=$(kubectl get node "$node" -o jsonpath='{.spec.taints}' 2>/dev/null)"
        cns=$(kubectl -n kube-system get pod -l k8s-app=azure-cns --field-selector spec.nodeName="$node" -o jsonpath='{.items[0].metadata.name}' 2>/dev/null)
        echo "DEBUG cns-on-node=$cns ready=$(kubectl -n kube-system get pod "$cns" -o jsonpath='{.status.containerStatuses[0].ready}' 2>/dev/null)"
        if [ -n "$cns" ]; then
            kubectl -n kube-system logs "$cns" -c cns-container --tail=25 2>&1 | sed 's/^/DEBUG cnslog: /'
        fi
        return 0
    done
}

echo "===== DEBUG node inventory ====="
kubectl get nodes -o wide 2>&1
kubectl get nodes -o 'custom-columns=NAME:.metadata.name,OS:.metadata.labels.kubernetes\.io/os,UNSCHED:.spec.unschedulable,TAINTS:.spec.taints' 2>&1

debug_capture "$POD_NAME-wireserver-${SUFFIX}" > "/tmp/diag-wireserver-${SUFFIX}.log" 2>&1 &
kubectl run "$POD_NAME-wireserver-${SUFFIX}" -it --rm --image busybox --restart Never --pod-running-timeout=5m "${OVERRIDES[@]}" -- wget --timeout=3 --header=Metadata:true "http://168.63.129.16/machine/plugins?comp=nmagent&type=getinterfaceinfov1"
rc=$?
cat "/tmp/diag-wireserver-${SUFFIX}.log" 2>/dev/null
if [ $rc -eq 0 ]; then
    echo "wireserver connectivity expected to fail but succeeded"
    exit 1
fi

debug_capture "$POD_NAME-metadata-${SUFFIX}" > "/tmp/diag-metadata-${SUFFIX}.log" 2>&1 &
kubectl run "$POD_NAME-metadata-${SUFFIX}" -it --rm --image busybox --restart Never --pod-running-timeout=5m "${OVERRIDES[@]}" -- wget --timeout=3 --header=Metadata:true "http://169.254.169.254/metadata/instance?api-version=2021-02-01"
rc=$?
cat "/tmp/diag-metadata-${SUFFIX}.log" 2>/dev/null
if [ $rc -ne 0 ]; then
    echo "metadata server connectivity expected to succeed but failed"
    exit 1
fi