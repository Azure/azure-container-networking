#!/usr/bin/env bash
# Releases the pipeline-run lease (ConfigMap on aks-1).
# Only releases if the lease is held by the current run ID.
#
# Usage: release_pipeline_lease.sh <kubeconfig> <run_id>
set -euo pipefail

KUBECONFIG_FILE=$1
RUN_ID=$2

NAMESPACE="default"
CM_NAME="acn-pipeline-lease"

EXISTING_RUN=$(kubectl --kubeconfig "$KUBECONFIG_FILE" get configmap "$CM_NAME" \
  -n "$NAMESPACE" -o jsonpath='{.data.runId}' 2>/dev/null || echo "")

if [ "$EXISTING_RUN" = "$RUN_ID" ]; then
  kubectl --kubeconfig "$KUBECONFIG_FILE" delete configmap "$CM_NAME" -n "$NAMESPACE"
  echo "Lease released by run $RUN_ID"
elif [ -z "$EXISTING_RUN" ]; then
  echo "No lease found (already released or never acquired)"
else
  echo "WARNING: Lease held by different run ($EXISTING_RUN), not releasing"
fi
