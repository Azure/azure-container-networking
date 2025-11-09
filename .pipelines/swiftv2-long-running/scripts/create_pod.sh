#!/bin/bash
# Usage:
# ./create_pod.sh <POD_NAME> <NODE_NAME> <OS> <PN_NAME> <PNI_NAME> [IMAGE]
# Example:
# ./create_pod.sh netpod1 aks-node1 linux podnet-a pni-exp praqma/network-multitool

set -euo pipefail

POD_NAME=$1
NODE_NAME=$2
OS=$3
PN_NAME=$4
PNI_NAME=$5
IMAGE=${6:-weibeld/ubuntu-networking}
KUBECONFIG_PATH=$7

echo "Creating pod '$POD_NAME' on node '$NODE_NAME' using image '$IMAGE'..."
echo "PodNetwork: $PN_NAME, PodNetworkInstance: $PNI_NAME, OS: $OS"

export KUBECONFIG=$KUBECONFIG_PATH
cat <<EOF | kubectl apply -f -
apiVersion: v1
kind: Pod
metadata:
  name: $POD_NAME
  labels:
    kubernetes.azure.com/pod-network-instance: $PNI_NAME
    kubernetes.azure.com/pod-network: $PN_NAME
spec:
  nodeName: $NODE_NAME
  nodeSelector:
    kubernetes.io/os: $OS
  containers:
  - name: net-debugger
    image: $IMAGE
    command: ["/bin/sh", "-c"]
    args:
      - |
        echo "Pod Network Diagnostics started on \$(hostname)";
        echo "----------------------------------------------";
        while true; do
          echo "[$(date)] Running net tests...";
          ip addr show;
          ip route show;
          sleep 60;
        done
    resources:
      limits:
        cpu: 300m
        memory: 600Mi
      requests:
        cpu: 300m
        memory: 600Mi
    securityContext:
      privileged: true
  restartPolicy: Always
EOF

echo "Pod '$POD_NAME' created successfully."
kubectl get pod "$POD_NAME" -o wide
