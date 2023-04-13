#!/bin/bash
set -ex
kubectl apply -f hack/manifests/pod.yaml
kubectl apply -f hack/manifests/hostprocess.yaml
sleep 1m
total_num_of_run=4
scale_up_of_pods=2400
scale_down_pods=1
echo "Total num of run $total_num_of_run"

function check_deployment() {
    available=-1
    replicas="$1"
    while [ "${available}" -ne "${replicas}" ]; do
        sleep 5s
        current_available=$(kubectl get deployment container  -o "jsonpath={.status.availableReplicas}" )
        if [ "$current_available" != '' ]; then
            available=$current_available
        fi
        echo "available replicas: ${available}"
    done
    echo "deployment complete."
}

for ((i=1; i <= total_num_of_run; i++))
do 
    echo "Current Run: $i"
    echo "Scaling pods to : $scale_up_of_pods"
    kubectl scale deployment container --replicas $scale_up_of_pods
    check_deployment $scale_up_of_pods
    echo "Scaling down pods to : $scale_down_pods"
    kubectl scale deployment container --replicas $scale_down_pods
    check_deployment $scale_down_pods
done

kubectl scale deployment container --replicas $scale_up_of_pods
check_deployment $scale_up_of_pods
