#!/bin/bash
set -e

echo "Deploying windows ds..."
kubectl apply -f ../../test/integration/manifests/load/privileged-daemonset-windows.yaml

echo "Waiting for ds to be ready on windows nodes..."
kubectl rollout status daemonset/privileged-daemonset -n kube-system --timeout=300s

WINDOWS_NODES=$(kubectl get nodes -l kubernetes.io/os=windows -o jsonpath='{.items[*].metadata.name}')

if [ -z "$WINDOWS_NODES" ]; then
    echo "No windows nodes found, skipping kubeclusterconfig.json patch"
    exit 0
fi

echo "Found windows nodes: $WINDOWS_NODES"

for NODE in $WINDOWS_NODES; do
    echo "Patching kubeclusterconfig.json on node: $NODE"
    
    # get pod running on this specific node
    POD_NAME=$(kubectl get pods -n kube-system -l app=privileged-daemonset,os=windows --field-selector spec.nodeName=$NODE -o jsonpath='{.items[0].metadata.name}')
    
    if [ -z "$POD_NAME" ]; then
        echo "Warning: No privileged daemonset pod found on node $NODE, skipping"
        continue
    fi
    
    echo "Using pod $POD_NAME on node $NODE"
    
    # patch so restart script recognizes acn files should be cleaned up
    kubectl exec -n kube-system $POD_NAME -- powershell.exe -Command "(Get-Content 'c:\k\kubeclusterconfig.json') -replace '\"Name\":\s*\"none\"', '\"Name\": \"azure\"' | Set-Content 'c:\k\kubeclusterconfig.json'"
    
    echo "Displaying file contents after patching on node $NODE:"
    kubectl exec -n kube-system $POD_NAME -- powershell.exe -Command "Get-Content 'c:\k\kubeclusterconfig.json'"
done

echo "Finished patching kubeclusterconfig.json on all Windows nodes"
