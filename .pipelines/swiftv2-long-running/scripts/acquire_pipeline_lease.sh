#!/usr/bin/env bash
# Acquires a pipeline-run lease using a Kubernetes ConfigMap.
# Prevents concurrent pipeline runs from stepping on each other.
#
# Lease is a ConfigMap in the 'default' namespace of aks-1.
# If another run holds the lease (within TTL), this script exits immediately
# with code 2 so pipeline stages can be skipped without failing the run.
# If the lease is expired or absent, it claims it.
#
# Usage: acquire_pipeline_lease.sh <kubeconfig> <run_id> [lease_ttl_minutes]
# Example: acquire_pipeline_lease.sh /tmp/aks-1.kubeconfig 12345 120
set -euo pipefail

KUBECONFIG_FILE=$1
RUN_ID=$2
LEASE_TTL_MIN=${3:-120}

NAMESPACE="default"
CM_NAME="acn-pipeline-lease"
NOW=$(date +%s)
EXPIRY=$((NOW + LEASE_TTL_MIN * 60))

write_lease() {
  kubectl --kubeconfig "$KUBECONFIG_FILE" create configmap "$CM_NAME" \
    -n "$NAMESPACE" \
    --from-literal=runId="$RUN_ID" \
    --from-literal=startTime="$NOW" \
    --from-literal=expiryTime="$EXPIRY" \
    --dry-run=client -o yaml | kubectl --kubeconfig "$KUBECONFIG_FILE" apply -f -
  echo "Lease acquired by run $RUN_ID (expires in ${LEASE_TTL_MIN}m)"
}

echo "==> Attempting to acquire pipeline lease (run $RUN_ID)"
echo "  ConfigMap: $CM_NAME, Namespace: $NAMESPACE"
echo "  TTL: ${LEASE_TTL_MIN}m"

# Check for existing lease
EXISTING=$(kubectl --kubeconfig "$KUBECONFIG_FILE" get configmap "$CM_NAME" \
  -n "$NAMESPACE" -o json 2>/dev/null || echo "")

if [ -z "$EXISTING" ]; then
  echo "  No existing lease, acquiring..."
  write_lease
  exit 0
fi

EXISTING_RUN=$(kubectl --kubeconfig "$KUBECONFIG_FILE" get configmap "$CM_NAME" \
  -n "$NAMESPACE" -o jsonpath='{.data.runId}' 2>/dev/null || echo "")
EXISTING_EXPIRY=$(kubectl --kubeconfig "$KUBECONFIG_FILE" get configmap "$CM_NAME" \
  -n "$NAMESPACE" -o jsonpath='{.data.expiryTime}' 2>/dev/null || echo "0")

# If lease is expired, claim it
if [ "$NOW" -gt "${EXISTING_EXPIRY:-0}" ]; then
  echo "  Existing lease from run $EXISTING_RUN has expired, claiming..."
  write_lease
  exit 0
fi

REMAINING=$(( (EXISTING_EXPIRY - NOW) / 60 ))
echo "  Lease held by run $EXISTING_RUN (expires in ${REMAINING}m)"
START_TIME=$(kubectl --kubeconfig "$KUBECONFIG_FILE" get configmap "$CM_NAME" \
  -n "$NAMESPACE" -o jsonpath='{.data.startTime}' 2>/dev/null || echo "")
echo "  LEASE DETAILS: startTime=$START_TIME"
echo "  Lease is currently held; skipping lease-gated stages for this run."
exit 2
