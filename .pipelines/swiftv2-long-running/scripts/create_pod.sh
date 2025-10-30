#!/bin/bash
# Usage:
# ./create_pod.sh <RESOURCE_GROUP> <CLUSTER_NAME> <NAMESPACE> <POD_NAME> <NODE_NAME> [IMAGE]

set -euo pipefail

RESOURCE_GROUP=$1
CLUSTER_NAME=$2
NAMESPACE=$3
POD_NAME=$4
NODE_NAME=$5
IMAGE=${6:-nginx}

echo "Getting AKS credentials..."
az aks get-credentials -g "$RESOURCE_GROUP" -n "$CLUSTER_NAME" --overwrite-existing

echo "Creating namespace (if not exists)..."
kubectl get ns "$NAMESPACE" >/dev/null 2>&1 || kubectl create ns "$NAMESPACE"

echo "Creating pod '$POD_NAME' on node '$NODE_NAME' using image '$IMAGE'..."

cat <<EOF | kubectl apply -f -
apiVersion: v1
kind: Pod
metadata:
  name: $POD_NAME
  namespace: $NAMESPACE
  labels:
    app: $POD_NAME
spec:
  nodeSelector:
    kubernetes.io/hostname: $NODE_NAME
  containers:
  - name: main
    image: $IMAGE
    command: ["sleep", "3600"]
    imagePullPolicy: IfNotPresent
  restartPolicy: Always
EOF

echo "Pod '$POD_NAME' created successfully in namespace '$NAMESPACE'."
kubectl get pod "$POD_NAME" -n "$NAMESPACE" -o wide
