#!/usr/bin/env bash
# Idempotently creates per-zone high-NIC node pools for the hourly pod tests.
# Each zone gets a 1-node pool. The node runs 6 rotating pods + 1 DaemonSet always-on pod.
# Reuses the same VNet/podnet subnet as the existing nplinux pool.
#
# Usage: ensure_zone_nodepools.sh <SUBSCRIPTION_ID> <RESOURCE_GROUP> <CLUSTER_NAME> <VM_SKU_HIGHNIC> <ZONE_LIST> [PODS_PER_NODE]
# Example: ensure_zone_nodepools.sh <sub-id> sv2-long-run-eastus2euap aks-1 Standard_D16s_v3 "1 2 3 4" 7
set -euo pipefail

SUBSCRIPTION_ID=$1
RG=$2
CLUSTER=$3
VM_SKU=$4
ZONES=${5:-"1 2 3 4"}
PODS_PER_NODE=${6:-7}

echo "==> Ensuring zone node pools for cluster $CLUSTER in RG $RG"
echo "    Zones: $ZONES"
echo "    VM SKU: $VM_SKU"

# Get the existing pod subnet ID from the cluster's VNet
VNET_NAME=$(az network vnet list -g "$RG" --query "[?contains(name,'$CLUSTER')].name" -o tsv | head -1)
if [ -z "$VNET_NAME" ]; then
  echo "ERROR: Could not find VNet for cluster $CLUSTER in RG $RG"
  exit 1
fi
POD_SUBNET_ID="/subscriptions/${SUBSCRIPTION_ID}/resourceGroups/${RG}/providers/Microsoft.Network/virtualNetworks/${VNET_NAME}/subnets/podnet"
echo "    Pod Subnet: $POD_SUBNET_ID"

for ZONE in $ZONES; do
  POOL_NAME="npz${ZONE}"

  # Check if node pool already exists
  EXISTING=$(az aks nodepool show -g "$RG" --cluster-name "$CLUSTER" -n "$POOL_NAME" --query "name" -o tsv 2>/dev/null || true)
  if [ "$EXISTING" = "$POOL_NAME" ]; then
    echo "==> Node pool $POOL_NAME already exists in zone $ZONE, skipping creation"
    continue
  fi

  echo "==> Creating node pool $POOL_NAME in zone $ZONE (1 node, $VM_SKU)"
  az aks nodepool add -g "$RG" -n "$POOL_NAME" \
    --node-count 1 \
    --node-vm-size "$VM_SKU" \
    --cluster-name "$CLUSTER" \
    --os-type Linux \
    --max-pods 250 \
    --zones "$ZONE" \
    --subscription "$SUBSCRIPTION_ID" \
    --tags fastpathenabled=true aks-nic-enable-multi-tenancy=true stampcreatorserviceinfo=true "aks-nic-secondary-count=${PODS_PER_NODE}" \
    --aks-custom-headers AKSHTTPCustomFeatures=Microsoft.ContainerService/NetworkingMultiTenancyPreview \
    --pod-subnet-id "$POD_SUBNET_ID"

  echo "    Node pool $POOL_NAME created in zone $ZONE"
done

# Wait for all nodes to be Ready
echo "==> Waiting for all nodes to be Ready"
KUBECONFIG_FILE="/tmp/${CLUSTER}.kubeconfig"
az aks get-credentials -g "$RG" -n "$CLUSTER" --admin --overwrite-existing --file "$KUBECONFIG_FILE"
kubectl --kubeconfig "$KUBECONFIG_FILE" wait --for=condition=Ready nodes --all --timeout=10m

# Label the zone node pool nodes
for ZONE in $ZONES; do
  POOL_NAME="npz${ZONE}"

  echo "==> Labeling and tainting nodes in pool $POOL_NAME"
  kubectl --kubeconfig "$KUBECONFIG_FILE" label nodes -l agentpool=$POOL_NAME \
    nic-capacity=high-nic \
    workload-type=swiftv2-linux \
    hourly-zone-pool=true \
    --overwrite

  # Taint zone pool nodes so only test pods with the matching toleration can schedule here.
  # This prevents stray workloads from consuming vnet-nic capacity.
  kubectl --kubeconfig "$KUBECONFIG_FILE" taint nodes -l agentpool=$POOL_NAME \
    acn-test/zone-pool=true:NoSchedule \
    --overwrite

  # Verify zone label (AKS sets this automatically)
  NODE=$(kubectl --kubeconfig "$KUBECONFIG_FILE" get nodes -l agentpool=$POOL_NAME -o jsonpath='{.items[0].metadata.name}')
  ACTUAL_ZONE=$(kubectl --kubeconfig "$KUBECONFIG_FILE" get node "$NODE" -o jsonpath='{.metadata.labels.topology\.kubernetes\.io/zone}')
  echo "    Node $NODE zone label: $ACTUAL_ZONE"

  # The Go tests and DaemonSet manifests expect the zone label to be "<region>-<zone>" (e.g., "eastus2euap-1").
  # Fail fast if AKS uses a different format so we can fix the code before tests silently fail.
  LOCATION=$(az aks show -g "$RG" -n "$CLUSTER" --query location -o tsv)
  EXPECTED_ZONE="${LOCATION}-${ZONE}"
  if [ "$ACTUAL_ZONE" != "$EXPECTED_ZONE" ]; then
    echo "ERROR: Zone label mismatch! Expected '$EXPECTED_ZONE', got '$ACTUAL_ZONE'"
    echo "       The Go tests use '<location>-<zone>' format (e.g., 'eastus2euap-1')."
    echo "       Update GetZoneLabel() in datapath_hourly_shared.go and daemonset.yaml if format differs."
    exit 1
  fi
  echo "    Zone label verified: $ACTUAL_ZONE == $EXPECTED_ZONE"
done

echo "==> Zone node pool setup complete"
echo "==> Node summary:"
kubectl --kubeconfig "$KUBECONFIG_FILE" get nodes -l hourly-zone-pool=true \
  -o custom-columns='NAME:.metadata.name,ZONE:.metadata.labels.topology\.kubernetes\.io/zone,POOL:.metadata.labels.agentpool' \
  --sort-by='.metadata.labels.topology\.kubernetes\.io/zone'
