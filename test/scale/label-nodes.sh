#!/bin/sh
cmd=$1
retries=0
while [ $retries -lt 5 ]; do
    $cmd
    if [ $? -eq 0 ]; then
        break
    fi
    retries=$((retries+1))
    sleep 5s
done
    
if [ $retries -eq 5 ]; then
    echo "Error in executing $cmd"
    exit 1
fi

for node in $(kubectl get nodes -o name);
do
    echo "Current : $node"
    node_name="${node##*/}"
    echo "Apply label to the node"
    kubectl label node $node_name connectivity-test=true
    kubectl label node $node_name scale-test=true
    if [ $? -eq 0 ]; then
        echo "Label applied to the node"
    else
        echo "Error in applying label to the node $node_name"
    fi
    sleep 2s
done
