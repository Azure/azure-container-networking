#!/bin/bash

kubectl get pods -A -o wide --no-headers | while read -r line; do
    namespace=$(echo "$line" | awk '{print $1}')
    pod=$(echo "$line" | awk '{print $2}')
    status=$(echo "$line" | awk '{print $4}')

    if [[ "$status" != "Running" ]]; then
        echo "=============================="
        echo "Namespace: $namespace"
        echo "Pod: $pod"
        echo "Status: $status"
        echo "------------------------------"
        echo "Events:"
        kubectl describe pod "$pod" -n "$namespace" | awk '/^Events:/,/^$/'
        echo "------------------------------"
        echo "Logs:"
        kubectl logs "$pod" -n "$namespace" --tail=100
        echo "=============================="
        echo
    fi
done
