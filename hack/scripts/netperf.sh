#!/bin/bash
# find the nodes with netperf pods and assign test vars
node_found=0
for node in $(kubectl get nodes -o name);
do
    if [ $node_found -lt 2 ]; then
        echo "Current : $node"
        node_name="${node##*/}"
        echo "checking whether the node has any netperf pods deployed to it"
        pod_count=$(kubectl get pods -l app=container6 -o wide | grep "$node_name" -c)
        netperf_pod=$(kubectl get pods -l app=container6 -o wide | grep "$node_name" | awk '{print $1}')
        echo "netperf pod : $netperf_pod"
        echo "pod_count: $pod_count"

        if [ $pod_count -gt 1 ]; then 
            target_pod=$(echo $netperf_pod | cut -d" " -f 1)
            target_pod_ip=$(kubectl get pod "$target_pod" -o jsonpath='{.status.podIP}')
            same_vm_pod=$(echo $netperf_pod | cut -d" " -f 2)
            kubectl exec -it $target_pod -- netserver
            node_found=$((node_found + 1))
            echo "Number of nodes found with netperf pod: $node_found"
        else
            diff_vm_pod=$netperf_pod
            node_found=$((node_found + 1))
            echo "Number of nodes found with netperf pod: $node_found"
        fi
    fi
done

echo "target netperf pod: $target_pod"
echo "target netperf pod IP: $target_pod_ip"
echo "same vm pod: $same_vm_pod"
echo "different vm pod: $diff_vm_pod"

#netperf on same vm pod
iteration=10
while [ $iteration -ge 0 ]
do
    echo "============ Iteration $iteration ===============" 
    kubectl exec -it $same_vm_pod -- netperf -H $target_pod_ip -l 30 -t TCP_STREAM >> "test3_netperf/same_vm_iteration_$iteration.log"
    echo "==============================="
    sleep 5s
    iteration=$((iteration-1))
done

#netperf on different vm pod
iteration=10
while [ $iteration -ge 0 ]
do
    echo "============ Iteration $iteration ===============" 
    kubectl exec -it $diff_vm_pod -- netperf -H $target_pod_ip -l 30 -t TCP_STREAM >> "test3_netperf/diff_vm_iteration_$iteration.log"
    echo "==============================="
    sleep 5s
    iteration=$((iteration-1))
done
