#!/usr/bin/env bash
# Smart verification of long-running infrastructure.
# Checks a few "final products" to determine if full setup should be skipped.
#
# Final product checks:
#   1. AKS clusters exist and have Ready nodes (proves: RG, cluster, node pools all working)
#   2. Customer VNets exist (proves: VNets, subnets, delegations created)
#   3. VNet peerings are connected (proves: peerings set up)
#   4. Storage accounts exist (proves: storage created; RBAC implied by later checks)
#   5. Private endpoints exist (proves: PE + DNS zone setup)
#   6. NSGs exist (proves: network security rules applied)
#   7. VMSS exist (proves: BYON nodes joined)
#
# If ALL checks pass → infraExists=true (skip setup)
# If ANY check fails → infraExists=false (run setup), log warnings + emit metric
#
# Outputs (ADO variables):
#   infraExists      - "true" if all infrastructure verified
#   missingResources - space-separated list of what's missing
#
# Usage: verify_infrastructure.sh <subscription_id> <resource_group> [cluster_prefix] [cluster_count]
# Example: verify_infrastructure.sh <sub-id> sv2-long-run-eastus2euap aks 2
set -euo pipefail

SUBSCRIPTION_ID=$1
RG=$2
CLUSTER_PREFIX=${3:-aks}
CLUSTER_COUNT=${4:-2}

INFRA_EXISTS=true
MISSING=()

echo "============================================"
echo "==> Infrastructure Verification"
echo "    Resource Group: $RG"
echo "    Clusters: ${CLUSTER_PREFIX}-1 .. ${CLUSTER_PREFIX}-${CLUSTER_COUNT}"
echo "============================================"

# -----------------------------------------------
# Check 1: AKS clusters exist and nodes are Ready
# This is the strongest signal — if kubeconfig works and nodes respond,
# the cluster, node pools, and networking are all functional.
# -----------------------------------------------
for i in $(seq 1 "$CLUSTER_COUNT"); do
  CLUSTER="${CLUSTER_PREFIX}-${i}"
  echo ""
  echo "==> Checking cluster $CLUSTER"

  STATE=$(az aks show -g "$RG" -n "$CLUSTER" --subscription "$SUBSCRIPTION_ID" \
    --query provisioningState -o tsv 2>/dev/null || true)

  if [ "$STATE" != "Succeeded" ]; then
    echo "  WARNING: Cluster $CLUSTER not found or not healthy (state: ${STATE:-not found})"
    MISSING+=("cluster:$CLUSTER")
    INFRA_EXISTS=false
    continue
  fi

  KUBECONFIG_FILE="/tmp/${CLUSTER}.kubeconfig"
  if ! az aks get-credentials -g "$RG" -n "$CLUSTER" --admin --overwrite-existing \
    --subscription "$SUBSCRIPTION_ID" --file "$KUBECONFIG_FILE" 2>/dev/null; then
    echo "  WARNING: Failed to get credentials for cluster $CLUSTER"
    MISSING+=("kubeconfig:$CLUSTER")
    INFRA_EXISTS=false
    continue
  fi

  # Count Ready nodes using JSONPath to avoid false negatives from statuses like
  # "Ready,SchedulingDisabled" (cordoned nodes) that grep " Ready " would miss.
  READY_NODES=$(kubectl --kubeconfig "$KUBECONFIG_FILE" get nodes \
    -o jsonpath='{range .items[*]}{.status.conditions[?(@.type=="Ready")].status}{"\n"}{end}' \
    2>/dev/null | grep -c "^True$" || echo "0")

  if [ "$READY_NODES" -lt 1 ]; then
    echo "  WARNING: Cluster $CLUSTER has no Ready nodes"
    MISSING+=("nodes:$CLUSTER")
    INFRA_EXISTS=false
  else
    echo "  OK: $CLUSTER has $READY_NODES Ready node(s)"
  fi
done

# -----------------------------------------------
# Check 2: Customer VNets exist
# If these exist, the VNet creation + subnet delegation succeeded.
# -----------------------------------------------
for VNET in cx_vnet_v1 cx_vnet_v2; do
  echo ""
  echo "==> Checking VNet $VNET"
  if az network vnet show -g "$RG" -n "$VNET" --subscription "$SUBSCRIPTION_ID" &>/dev/null; then
    echo "  OK: $VNET exists"
  else
    echo "  WARNING: VNet $VNET not found in $RG"
    MISSING+=("vnet:$VNET")
    INFRA_EXISTS=false
  fi
done

# -----------------------------------------------
# Check 3: VNet peerings are connected
# If peerings are Active, cross-VNet connectivity is set up.
# -----------------------------------------------
echo ""
echo "==> Checking VNet peerings"
PEERING_COUNT=$(az network vnet peering list -g "$RG" --vnet-name cx_vnet_v1 \
  --subscription "$SUBSCRIPTION_ID" --query "length([?peeringState=='Connected'])" -o tsv 2>/dev/null || echo "0")
if [ "$PEERING_COUNT" -gt 0 ]; then
  echo "  OK: $PEERING_COUNT active peering(s) on cx_vnet_v1"
else
  echo "  WARNING: No active peerings found on cx_vnet_v1"
  MISSING+=("peerings")
  INFRA_EXISTS=false
fi

# -----------------------------------------------
# Check 4: Storage accounts exist
# RBAC/container correctness is implied by PE and NSG checks below —
# those tasks run after storage in the same job, so if they exist,
# storage creation completed successfully.
# -----------------------------------------------
echo ""
echo "==> Checking storage accounts"
SA1=$(az storage account list -g "$RG" --subscription "$SUBSCRIPTION_ID" \
  --query "[?starts_with(name, 'sa1')].name | [0]" -o tsv 2>/dev/null || true)
SA2=$(az storage account list -g "$RG" --subscription "$SUBSCRIPTION_ID" \
  --query "[?starts_with(name, 'sa2')].name | [0]" -o tsv 2>/dev/null || true)

if [[ -n "$SA1" && -n "$SA2" ]]; then
  echo "  OK: Found storage accounts: $SA1, $SA2"
else
  echo "  WARNING: Expected 2 storage accounts (sa1*, sa2*) in $RG, found: ${SA1:-none}, ${SA2:-none}"
  MISSING+=("storage")
  INFRA_EXISTS=false
fi

# -----------------------------------------------
# Check 5: Private endpoints exist
# -----------------------------------------------
echo ""
echo "==> Checking private endpoints"
PE_COUNT=$(az network private-endpoint list -g "$RG" --subscription "$SUBSCRIPTION_ID" \
  --query "length(@)" -o tsv 2>/dev/null || echo "0")
if [ "$PE_COUNT" -gt 0 ]; then
  echo "  OK: $PE_COUNT private endpoint(s) found"
else
  echo "  WARNING: No private endpoints found in $RG"
  MISSING+=("private-endpoints")
  INFRA_EXISTS=false
fi

# -----------------------------------------------
# Check 6: NSGs exist
# -----------------------------------------------
echo ""
echo "==> Checking network security groups"
NSG_COUNT=$(az network nsg list -g "$RG" --subscription "$SUBSCRIPTION_ID" \
  --query "length(@)" -o tsv 2>/dev/null || echo "0")
if [ "$NSG_COUNT" -gt 0 ]; then
  echo "  OK: $NSG_COUNT NSG(s) found"
else
  echo "  WARNING: No NSGs found in $RG"
  MISSING+=("nsgs")
  INFRA_EXISTS=false
fi

# -----------------------------------------------
# Check 7: VMSS exist (BYON nodes)
# -----------------------------------------------
echo ""
echo "==> Checking VMSS"
VMSS_COUNT=$(az vmss list -g "$RG" --subscription "$SUBSCRIPTION_ID" \
  --query "length(@)" -o tsv 2>/dev/null || echo "0")
if [ "$VMSS_COUNT" -gt 0 ]; then
  echo "  OK: $VMSS_COUNT VMSS found"
else
  echo "  WARNING: No VMSS found in $RG"
  MISSING+=("vmss")
  INFRA_EXISTS=false
fi

# -----------------------------------------------
# Summary
# -----------------------------------------------
echo ""
echo "============================================"
if [ "$INFRA_EXISTS" = "true" ]; then
  echo "==> RESULT: All infrastructure verified. Setup will be SKIPPED."
else
  echo "==> RESULT: Missing resources detected. Setup will RUN."
  echo "    Missing: ${MISSING[*]}"
  echo ""
  echo "##vso[task.logissue type=warning]Infrastructure resources missing: ${MISSING[*]}. Running full setup."
fi
echo "============================================"

# Set ADO output variables
echo "##vso[task.setvariable variable=infraExists;isOutput=true]$INFRA_EXISTS"
echo "##vso[task.setvariable variable=missingResources;isOutput=true]${MISSING[*]:-none}"
