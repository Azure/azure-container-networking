#!/bin/bash
# Create and wait for a PodNetworkInstance (PNI) in AKS using kubectl.
#
# Usage:
# ./create_pni.sh <kubeconfig> <namespace> <pni_name> <pod_network_name> <pni_type> <reservations> <default_deny_enabled>
#
# Example:
# ./create_pni.sh ~/.kube/config testns pni-exp mypodnet explicit 5 true
# ./create_pni.sh ~/.kube/config testns pni-imp mypodnet implicit 0 false

set -euo pipefail

KUBECONFIG_PATH=$1
NAMESPACE=$2
PNI_NAME=$3
POD_NETWORK_NAME=$4
PNI_TYPE=$5           # "explicit" or "implicit"
RESERVATIONS=${6:-0}  # only used for explicit

export KUBECONFIG=$KUBECONFIG_PATH

if ! kubectl get namespace "$NAMESPACE" >/dev/null 2>&1; then
  echo "Namespace '$NAMESPACE' not found. Creating it..."
  kubectl create namespace "$NAMESPACE"
else
  echo "Namespace '$NAMESPACE' already exists."
fi

echo "Creating PodNetworkInstance:"
echo "  Namespace:      $NAMESPACE"
echo "  Name:           $PNI_NAME"
echo "  Network:        $POD_NETWORK_NAME"
echo "  Type:           $PNI_TYPE"
echo "  Reservations:   $RESERVATIONS"
echo

# --- Apply PNI manifest ---
if [[ "$PNI_TYPE" == "explicit" ]]; then
cat <<EOF | kubectl apply -f -
apiVersion: acn.azure.com/v1alpha1
kind: PodNetworkInstance
metadata:
  name: $PNI_NAME
  namespace: $NAMESPACE
spec:
  podNetworkConfigs:
  - podNetwork: $POD_NETWORK_NAME
    podIPReservationSize: $RESERVATIONS
EOF
else
cat <<EOF | kubectl apply -f -
apiVersion: acn.azure.com/v1alpha1
kind: PodNetworkInstance
metadata:
  name: $PNI_NAME
  namespace: $NAMESPACE
spec:
  podNetworkConfigs:
  - podNetwork: $POD_NETWORK_NAME
EOF
fi

echo "PodNetworkInstance '$PNI_NAME' applied."

# --- Wait for readiness ---
echo "Waiting for PodNetworkInstance '$PNI_NAME' to become Ready..."

MAX_ATTEMPTS=30
SLEEP_INTERVAL=10

for i in $(seq 1 $MAX_ATTEMPTS); do
  STATUS=$(kubectl get podnetworkinstance "$PNI_NAME" -n "$NAMESPACE" -o jsonpath='{.status.status}' 2>/dev/null || echo "")
  RES_COUNT=$(kubectl get podnetworkinstance "$PNI_NAME" -n "$NAMESPACE" -o jsonpath='{.status.podIPAddresses[*]}' 2>/dev/null | wc -w || echo 0)

  echo "Attempt $i: status='$STATUS', reservations=$RES_COUNT/$RESERVATIONS"

  if [[ "$PNI_TYPE" == "explicit" && "$RES_COUNT" -ge "$RESERVATIONS" ]]; then
    echo "All $RESERVATIONS IP reservations are ready."
    exit 0
  fi

  if [[ "$STATUS" == "Ready" || "$STATUS" == "InUse" ]]; then
    echo "PNI '$PNI_NAME' is in status '$STATUS'."
    exit 0
  fi

  sleep $SLEEP_INTERVAL
done

echo "Timeout waiting for PodNetworkInstance '$PNI_NAME' to become Ready."
kubectl get podnetworkinstance "$PNI_NAME" -n "$NAMESPACE" -o yaml || true
exit 1
