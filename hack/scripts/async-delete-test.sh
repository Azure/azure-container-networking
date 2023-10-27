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
        file=$(kubectl exec -it $cns_pod -n kube-system -- ls var/run/azure-vnet/deleteIDs)
        if [ -z $file ]; then
            while [ -z $file ]; 
            do
                file=$(kubectl exec -i $cns_pod -n kube-system -- ls var/run/azure-vnet/deleteIDs)
            done
        fi
        echo "pending deletes"
        echo $file

        echo "wait 30s for filesystem delete to occur"
        sleep 30s
        echo "check directory is now empty"
        check_directory=$(kubectl exec -i $cns_pod -n kube-system -- ls var/run/azure-vnet/deleteIDs)
        if [ -z $check_directory ]; then
            echo "async delete success"
            break
        else
            echo "##[error]async delete failure. file still exists in deleteIDs directory."
        fi
    fi
done

