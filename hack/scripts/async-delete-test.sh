echo "create busybox deployment and verify async delete"
kubectl apply -f ../manifests/busybox.yaml
kubectl rollout status deployment busybox

echo "temporarily disable CNS daemonset and attempt busybox pod delete"
kubectl -n kube-system patch daemonset azure-cns -p '{"spec": {"template": {"spec": {"nodeSelector": {"non-existing": "true"}}}}}'

echo "delete busybox pod"
for node in $(kubectl get nodes -o name);
do
    node_name="${node##*/}"
    busybox_pod=$(kubectl get pods -l k8s-app=busybox -o wide | grep "$node_name" | awk '{print $1}')
    if [ -z $busybox_pod  ]; then
        continue
    else
        echo "wait 1 min for delete to processes and error to catch. expect a file to be written to var/run/azure-vnet/deleteIDs"
        kubectl delete pod $busybox_pod 
        sleep 60s
        
        echo "restore azure-cns pods"
        kubectl -n kube-system patch daemonset azure-cns --type json -p='[{"op": "remove", "path": "/spec/template/spec/nodeSelector/non-existing"}]'
        echo "wait 5s for cns to start back up"
        sleep 5s

        echo "check directory for pending delete"
        cns_pod=$(kubectl get pods -l k8s-app=azure-cns -n kube-system -o wide | grep "$node_name" | awk '{print $1}')
        file=$(kubectl exec -it $cns_pod -c debug -n kube-system -- ls var/run/azure-vnet/deleteIDs)
        if [ -z $file ]; then
            while [ -z $file ]; 
            do
                file=$(kubectl exec -i $cns_pod -c debug -n kube-system -- ls var/run/azure-vnet/deleteIDs)
            done
        fi
        echo "pending deletes"
        echo $file

        echo "wait 30s for filesystem delete to occur"
        sleep 30s
        echo "check directory is now empty"
        check_directory=$(kubectl exec -i $cns_pod -c debug -n kube-system -- ls var/run/azure-vnet/deleteIDs)
        if [ -z $check_directory ]; then
            echo "async delete success"
            break
        else
            echo "##[error]async delete failure. file still exists in deleteIDs directory."
            exit 1
        fi
    fi
done

# The loop above stops CNS before it deletes the Pod, so the pending delete file
# is always on disk before CNS starts and CNS reads it in the startup directory
# scan. When CNS runs but does not answer, the file instead appears while the
# fsnotify watcher listens. That is a different code path and it must release the
# IP without a CNS restart. SIGSTOP gives that state: the socket stays open, the
# CNI request times out after 15s, and the kernel queues the create event until
# SIGCONT. shareProcessNamespace on the daemonset lets the debug container signal
# the CNS process.
echo "verify async delete for a create event received while CNS runs"
kubectl rollout status deployment busybox

busybox_pod=$(kubectl get pods -l k8s-app=busybox --no-headers -o custom-columns=NAME:.metadata.name | head -1)
if [ -z "$busybox_pod" ]; then
    echo "##[error]no busybox pod found"
    exit 1
fi
node_name=$(kubectl get pod $busybox_pod -o jsonpath='{.spec.nodeName}')
cns_pod=$(kubectl get pods -l k8s-app=azure-cns -n kube-system -o jsonpath="{.items[?(@.spec.nodeName=='$node_name')].metadata.name}")
if [ -z "$cns_pod" ]; then
    echo "##[error]no CNS pod on node $node_name"
    exit 1
fi

cns_pid=$(kubectl exec -i $cns_pod -c debug -n kube-system -- sh -c 'for p in /proc/[0-9]*; do if [ "$(cat $p/comm 2>/dev/null)" = "azure-cns" ]; then echo ${p#/proc/}; fi; done' | tr -d '\r' | head -1)
if [ -z "$cns_pid" ]; then
    echo "##[error]azure-cns process not visible from the debug container. is shareProcessNamespace set on the daemonset?"
    exit 1
fi

restarts_before=$(kubectl get pod $cns_pod -n kube-system -o jsonpath='{.status.containerStatuses[?(@.name=="cns-container")].restartCount}')

echo "stop the CNS process $cns_pid so the CNI cannot reach it"
kubectl exec -i $cns_pod -c debug -n kube-system -- sh -c "kill -STOP $cns_pid"

echo "delete busybox pod $busybox_pod. the CNI DEL waits for the 15s CNS request timeout"
kubectl delete pod $busybox_pod --timeout=120s

echo "check the CNI wrote a pending delete while CNS was stopped"
pending_file=$(kubectl exec -i $cns_pod -c debug -n kube-system -- ls /var/run/azure-vnet/deleteIDs)

echo "resume the CNS process"
kubectl exec -i $cns_pod -c debug -n kube-system -- sh -c "kill -CONT $cns_pid"

if [ -z "$pending_file" ]; then
    echo "##[error]async delete failure. the CNI wrote no file, so the async delete path did not run."
    exit 1
fi
echo "pending deletes"
echo $pending_file

echo "wait up to 60s for CNS to process the create event"
waited=0
while [ $waited -lt 60 ]; do
    sleep 5s
    waited=$((waited + 5))
    live_file=$(kubectl exec -i $cns_pod -c debug -n kube-system -- ls /var/run/azure-vnet/deleteIDs)
    if [ -z "$live_file" ]; then
        break
    fi
done

if [ -n "$live_file" ]; then
    echo "##[error]async delete failure. CNS did not process the create event. file still exists in deleteIDs directory."
    exit 1
fi

restarts_after=$(kubectl get pod $cns_pod -n kube-system -o jsonpath='{.status.containerStatuses[?(@.name=="cns-container")].restartCount}')
if [ "$restarts_before" != "$restarts_after" ]; then
    echo "##[error]async delete failure. CNS restarted, so the startup scan processed the file instead of the create event."
    exit 1
fi

echo "async delete success for the create event"

