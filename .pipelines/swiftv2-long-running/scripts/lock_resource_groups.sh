#!/usr/bin/env bash
set -euo pipefail

SUBSCRIPTION_ID=$1
RG=$2
PIPELINE_NAME=${3:-"swiftv2-long-running"}

LOCK_NAME="pipeline-do-not-delete"
DELETION_DUE_TIME=$(date -u -d "+14 days" "+%Y-%m-%dT%H:%M:%SZ")
TAGS="deletion_due_time=$DELETION_DUE_TIME gc_scenario=$PIPELINE_NAME gc_skip=true"

az account set --subscription "$SUBSCRIPTION_ID"

# Tag and lock the main resource group
echo "==> Tagging resource group $RG"
az group update -n "$RG" --tags $TAGS 2>/dev/null || true

echo "==> Locking resource group $RG"
az lock create \
  --name "$LOCK_NAME" \
  --resource-group "$RG" \
  --lock-type CanNotDelete \
  --notes "Applied by long-running pipeline to prevent accidental deletion" \
  2>/dev/null || echo "  Lock already exists on $RG, skipping."

# Tag and lock each AKS cluster's managed (MC_) resource group
for CLUSTER in aks-1 aks-2; do
  MC_RG=$(az aks show \
    -g "$RG" -n "$CLUSTER" \
    --query nodeResourceGroup -o tsv 2>/dev/null || true)

  if [[ -z "$MC_RG" ]]; then
    echo "  [WARN] Could not find managed RG for $CLUSTER, skipping."
    continue
  fi

  echo "==> Tagging managed resource group $MC_RG"
  az group update -n "$MC_RG" --tags $TAGS 2>/dev/null || true

  echo "==> Locking managed resource group $MC_RG (cluster: $CLUSTER)"
  az lock create \
    --name "$LOCK_NAME" \
    --resource-group "$MC_RG" \
    --lock-type CanNotDelete \
    --notes "Applied by long-running pipeline for AKS cluster $CLUSTER" \
    2>/dev/null || echo "  Lock already exists on $MC_RG, skipping."
done

echo "All resource group locks and tags applied."
