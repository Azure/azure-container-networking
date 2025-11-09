#!/usr/bin/env bash
set -euo pipefail

# === INPUT PARAMETERS ===
KUBECONFIG_PATH="$1"         # Path to kubeconfig file
POD_NETWORK_NAME="$2"        # Name of PodNetwork CRD
VNET_GUID="$3"               # GUID of VNET
SUBNET_GUID="$4"             # GUID of delegated subnet
SUBNET_ARM_ID="$5"           # ARM ID of delegated subnet
SUBNET_TOKEN="${6:-}"        # Optional override subnet token

# === STEP 1: Verify inputs ===
if [[ -z "$KUBECONFIG_PATH" || -z "$POD_NETWORK_NAME" || -z "$VNET_GUID" || -z "$SUBNET_ARM_ID" ]]; then
  echo "Usage: $0 <kubeconfig> <pod_network_name> <vnet_guid> <subnet_guid> <subnet_arm_id> [subnet_token]"
  exit 1
fi

# === STEP 2: Build PodNetwork YAML ===
export KUBECONFIG=$KUBECONFIG_PATH
TMPFILE=$(mktemp)

if [[ -n "$SUBNET_TOKEN" ]]; then
cat > "$TMPFILE" <<EOF
apiVersion: acn.azure.com/v1alpha1
kind: PodNetwork
metadata:
  name: ${POD_NETWORK_NAME}
  labels:
    kubernetes.azure.com/override-subnet-token: "${SUBNET_TOKEN}"
spec:
  networkID: "${VNET_GUID}"
  subnetResourceID: "${SUBNET_ARM_ID}"
  deviceType: VnetNIC
EOF
else
cat > "$TMPFILE" <<EOF
apiVersion: acn.azure.com/v1alpha1
kind: PodNetwork
metadata:
  name: ${POD_NETWORK_NAME}
spec:
  networkID: "${VNET_GUID}"
  subnetGUID: "${SUBNET_GUID}"
  subnetResourceID: "${SUBNET_ARM_ID}"
  deviceType: VnetNIC
EOF
fi

# === STEP 3: Apply the PodNetwork CRD ===
echo "Creating PodNetwork ${POD_NETWORK_NAME}..."
kubectl apply -f "$TMPFILE" || true

# === STEP 4: Wait until the CRD becomes Ready (if subnetToken not provided) ===
if [[ -z "$SUBNET_TOKEN" ]]; then
  echo "Waiting for PodNetwork ${POD_NETWORK_NAME} to become Ready..."
  for i in {1..30}; do
    STATUS=$(kubectl --kubeconfig="$KUBECONFIG_PATH" get podnetwork "$POD_NETWORK_NAME" -o jsonpath='{.status.status}' 2>/dev/null || echo "")
    if [[ "$STATUS" == "Ready" || "$STATUS" == "InUse" ]]; then
      echo "PodNetwork ${POD_NETWORK_NAME} is ${STATUS}"
      break
    else
      echo "Attempt $i: Current status: ${STATUS:-<none>}, retrying..."
      sleep 10
    fi

    if [[ $i -eq 30 ]]; then
      echo "Timed out waiting for PodNetwork ${POD_NETWORK_NAME} to become Ready."
      exit 1
    fi
  done
else
  echo "Using subnet token override — skipping status wait."
fi

echo "Multi-tenant PodNetwork setup complete!"
