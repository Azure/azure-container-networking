#!/bin/bash
# Should be used for manual validation. Does not check if correct kernel is installed or if installation failed.

# DUMMY_CNI     - Flag to provide a temporary CNI
if [ $DUMMY_CNI = "true" ]; then
    echo "-- Install dummy CNI for nodes to be marked as Ready --"
    kubectl get pods -Aowide
    kubectl apply -f https://raw.githubusercontent.com/Azure/azure-container-networking/v1.5.3/hack/manifests/cni-installer-v1.yaml
    kubectl rollout status ds -n kube-system azure-cni
fi

echo "-- Start privileged daemonset --"
kubectl get pods -Aowide
kubectl apply -f https://raw.githubusercontent.com/Azure/azure-container-networking/v1.5.21/test/integration/manifests/load/privileged-daemonset.yaml
sleep 3s
kubectl rollout status ds -n kube-system privileged-daemonset

kubectl get pods -n kube-system -l os=linux,app=privileged-daemonset -owide
privList=`kubectl get pods -n kube-system -l os=linux,app=privileged-daemonset -owide --no-headers | awk '{print $1}'`
for pod in $privList; do
    echo "-- Stage Ubuntu kernel upgrade --"
    kubectl cp kernel-upgrade.sh -n kube-system $pod:/
    kubectl exec -i -n kube-system $pod -- bash ./kernel-upgrade.sh
done

kubectl get pods -n kube-system -l os=linux,app=privileged-daemonset -owide
privArray=(`kubectl get pods -n kube-system -l os=linux,app=privileged-daemonset -owide --no-headers | awk '{print $1}'`)
nodeArray=(`kubectl get pods -n kube-system -l os=linux,app=privileged-daemonset -owide --no-headers | awk '{print $7}'`)

echo "-- Restart nodes --"
i=0
for _ in ${privArray[@]}; do
    echo "-- Restarting Node ${nodeArray[i]} through ${privArray[i]} --"
    kubectl exec -i -n kube-system ${privArray[i]} -- bash -c "reboot"
    echo "-- Waiting for condition NotReady --"
    kubectl wait --for=condition=Ready=false -n kube-system pod/${privArray[i]} --timeout=90s
    echo "-- Waiting for condition Ready --"
    kubectl wait --for=condition=Ready -n kube-system pod/${privArray[i]} --timeout=90s
    ((i = i + 1))
done

echo "-- Check kernel --"
kubectl get node -owide
