#!/usr/bin/env bash
# Acquires a pipeline-run lease using a Kubernetes ConfigMap.
# Prevents concurrent pipeline runs from stepping on each other.
#
# Lease is a ConfigMap in the 'default' namespace of aks-1.
# If another run holds the lease (within TTL), this script waits and retries.
# If the lease is expired or absent, it claims it.
#
# Usage: acquire_pipeline_lease.sh <kubeconfig> <run_id> [max_wait_minutes] [lease_ttl_minutes]
# Example: acquire_pipeline_lease.sh /tmp/aks-1.kubeconfig 12345 30 240
set -euo pipefail

KUBECONFIG_FILE=$1
RUN_ID=$2
MAX_WAIT_MIN=${3:-30}
LEASE_TTL_MIN=${4:-120}

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
echo "  TTL: ${LEASE_TTL_MIN}m, Max wait: ${MAX_WAIT_MIN}m"

# Check for existing lease
EXISTING=$(kubectl --kubeconfig "$KUBECONFIG_FILE" get configmap "$CM_NAME" \
  -n "$NAMESPACE" -o json 2>/dev/null || echo "")

if [ -z "$EXISTING" ]; then
  echo "  No existing lease, acquiring..."
  write_lease
  exit 0
fi

EXISTING_RUN=$(echo "$EXISTING" | grep -o '"runId":"[^"]*"' | cut -d'"' -f4)
EXISTING_EXPIRY=$(echo "$EXISTING" | grep -o '"expiryTime":"[^"]*"' | cut -d'"' -f4)

# If lease is expired, claim it
if [ "$NOW" -gt "${EXISTING_EXPIRY:-0}" ]; then
  echo "  Existing lease from run $EXISTING_RUN has expired, claiming..."
  write_lease
  exit 0
fi

REMAINING=$(( (EXISTING_EXPIRY - NOW) / 60 ))
echo "  Lease held by run $EXISTING_RUN (expires in ${REMAINING}m)"
echo "  LEASE DETAILS: startTime=$(echo "$EXISTING" | grep -o '"startTime":"[^"]*"' | cut -d'"' -f4)"
echo "  Waiting up to ${MAX_WAIT_MIN}m for release..."

ELAPSED=0
INTERVAL=30
while [ "$ELAPSED" -lt "$((MAX_WAIT_MIN * 60))" ]; do
  sleep "$INTERVAL"
  ELAPSED=$((ELAPSED + INTERVAL))

  EXISTING=$(kubectl --kubeconfig "$KUBECONFIG_FILE" get configmap "$CM_NAME" \
    -n "$NAMESPACE" -o json 2>/dev/null || echo "")

  # Lease was released
  if [ -z "$EXISTING" ]; then
    echo "  Lease released, acquiring..."
    NOW=$(date +%s)
    EXPIRY=$((NOW + LEASE_TTL_MIN * 60))
    write_lease
    exit 0
  fi

  EXISTING_EXPIRY=$(echo "$EXISTING" | grep -o '"expiryTime":"[^"]*"' | cut -d'"' -f4)
  NOW_CHECK=$(date +%s)

  # Lease expired while we waited
  if [ "$NOW_CHECK" -gt "${EXISTING_EXPIRY:-0}" ]; then
    echo "  Lease expired, claiming..."
    NOW=$(date +%s)
    EXPIRY=$((NOW + LEASE_TTL_MIN * 60))
    write_lease
    exit 0
  fi

  echo "  Waiting... ($((ELAPSED / 60))m / ${MAX_WAIT_MIN}m)"
done

echo "ERROR: Could not acquire lease after ${MAX_WAIT_MIN}m. Held by run $EXISTING_RUN."
echo "  To manually release: kubectl delete configmap $CM_NAME -n $NAMESPACE --kubeconfig '$KUBECONFIG_FILE'"
exit 1
