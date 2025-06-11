#!/bin/bash
NAMESPACE="kube-system"

echo "Deploy nginx pods for MTU testing"
kubectl apply -f ../manifests/nginx.yaml
kubectl wait --for=condition=available --timeout=60s -n $NAMESPACE deployment/nginx

# Check node count
node_count=$(kubectl get nodes --no-headers | wc -l)

# in CNI release test scenario scale deployments to 3 * node count to get replicas on each node
if [ "$node_count" -gt 1 ]; then
    echo "Scaling nginx deployment to $((3 * node_count)) replicas"
    kubectl scale deployment nginx --replicas=$((3 * node_count)) -n $NAMESPACE
fi
# Wait for nginx pods to be ready
kubectl wait --for=condition=available --timeout=60s -n $NAMESPACE deployment/nginx



echo "Checking MTU for pods in namespace: $NAMESPACE using Cilium agent and nginx MTU"

# Get all nodes
nodes=$(kubectl get nodes -o jsonpath='{.items[*].metadata.name}')

for node in $nodes; do
    echo "Checking node: $node"

    # Get the Cilium agent pod running on this node
    cilium_pod=$(kubectl get pods -n $NAMESPACE -o wide --field-selector spec.nodeName=$node -l k8s-app=cilium -o jsonpath='{.items[0].metadata.name}')

    if [ -z "$cilium_pod" ]; then
        echo "Failed to find Cilium agent pod on node $node"
        echo "##[error]Failed to find Cilium agent pod on node $node"
        exit 1
    fi

    # Get the MTU of eth0 in the Cilium agent pod
    cilium_mtu=$(kubectl exec -n $NAMESPACE $cilium_pod -- cat /sys/class/net/eth0/mtu 2>/dev/null)

    if [ -z "$cilium_mtu" ]; then
        echo "Failed to get MTU from Cilium agent pod on node $node"
        echo "##[error]Failed to get MTU from Cilium agent pod on node $node"
        exit 1
    fi

    echo "Cilium agent eth0 MTU: $cilium_mtu"

    # Get an nginx pod running on this node
    nginx_pod=$(kubectl get pods -n $NAMESPACE -o wide --field-selector spec.nodeName=$node -l app=nginx -o jsonpath='{.items[0].metadata.name}')
    if [ -z "$nginx_pod" ]; then
        echo "Failed to find nginx pod on node $node"
        echo "##[error]Failed to find nginx pod on node $node"
        exit 1
    fi
    # Get the MTU of eth0 in the nginx pod
    nginx_mtu=$(kubectl exec -n $NAMESPACE $nginx_pod -- cat /sys/class/net/eth0/mtu 2>/dev/null)
    if [ -z "$nginx_mtu" ]; then
        echo "Failed to get MTU from nginx pod on node $node"
        echo "##[error]Failed to get MTU from nginx pod on node $node"
        exit 1
    fi
    echo "Nginx pod eth0 MTU: $nginx_mtu"

    # Get the node's eth0 MTU
    node_mtu=$(kubectl debug node/$node -it --image=busybox -- sh -c "cat /sys/class/net/eth0/mtu" 2>/dev/null | tail -n 1)

    if [ -z "$node_mtu" ]; then
        echo "Failed to get MTU from node $node"
        echo "##[error]Failed to get MTU from node $node"
        exit 1
    fi
    echo "Node eth0 MTU: $node_mtu"

    # Check if the MTUs match
    if [ "$cilium_mtu" -eq "$nginx_mtu" ] && [ "$nginx_mtu" -eq "$node_mtu" ]; then
        echo "MTU validation passed for node $node"
    else
        echo "MTU validation failed for node $node"
        echo "Cilium agent MTU: $cilium_mtu, Nginx pod MTU: $nginx_mtu, Node MTU: $node_mtu"
        echo "##[error]MTU validation failed. MTUs do not match."
        exit 1
    fi

    echo "----------------------------------------"

done

# Clean up
kubectl delete deployment nginx -n $NAMESPACE
echo "Cleaned up nginx deployment"

# Clean up the debug pod
debug_pod=$(kubectl get pods -o name | grep "node-debugger")
if [ -n "$debug_pod" ]; then
    kubectl delete $debug_pod
    kubectl wait --for=delete $debug_pod --timeout=60s
    if [ $? -ne 0 ]; then
        echo "Failed to clean up debug pod $debug_pod"
    fi
else
    echo "No debug pod found"
fi