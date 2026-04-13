#!/bin/bash

LOG_LINES=600

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
        echo "Init Container Logs:"
        for ic in $(kubectl get pod "$pod" -n "$namespace" -o jsonpath='{.spec.initContainers[*].name}'); do
            echo "--- Init Container: $ic (head) ---"
            kubectl logs "$pod" -n "$namespace" -c "$ic" | head -"$LOG_LINES"
            echo "--- Init Container: $ic (tail) ---"
            kubectl logs "$pod" -n "$namespace" -c "$ic" --tail="$LOG_LINES"
        done
        echo "------------------------------"
        echo "Sidecar Logs:"
        for sc in $(kubectl get pod "$pod" -n "$namespace" -o jsonpath='{.spec.containers[*].name}'); do
            echo "--- Sidecar: $sc (head) ---"
            kubectl logs "$pod" -n "$namespace" -c "$sc" | head -"$LOG_LINES"
            echo "--- Sidecar: $sc (tail) ---"
            kubectl logs "$pod" -n "$namespace" -c "$sc" --tail="$LOG_LINES"
        done
        echo "=============================="
        echo
    fi
done
