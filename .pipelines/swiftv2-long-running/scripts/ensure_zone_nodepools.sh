#!/usr/bin/env bash
# Idempotently creates per-zone high-NIC node pools for the long-running pod tests.
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
VNET_NAME=$(az network vnet list -g "$RG" --subscription "$SUBSCRIPTION_ID" --query "[?contains(name,'$CLUSTER')].name" -o tsv | head -1)
if [ -z "$VNET_NAME" ]; then
  echo "ERROR: Could not find VNet for cluster $CLUSTER in RG $RG"
  exit 1
fi
POD_SUBNET_ID="/subscriptions/${SUBSCRIPTION_ID}/resourceGroups/${RG}/providers/Microsoft.Network/virtualNetworks/${VNET_NAME}/subnets/podnet"
echo "    Pod Subnet: $POD_SUBNET_ID"

for ZONE in $ZONES; do
  POOL_NAME="npz${ZONE}"

  # Check if node pool already exists
  EXISTING=$(az aks nodepool show -g "$RG" --cluster-name "$CLUSTER" -n "$POOL_NAME" --subscription "$SUBSCRIPTION_ID" --query "name" -o tsv 2>/dev/null || true)
  if [ "$EXISTING" = "$POOL_NAME" ]; then
    echo "==> Node pool $POOL_NAME already exists in zone $ZONE, skipping creation"
    continue
  fi

  if [ "$ZONE" = "0" ]; then
    echo "==> Creating node pool $POOL_NAME with NO availability zone (generic) (1 node, $VM_SKU)"
    az aks nodepool add -g "$RG" -n "$POOL_NAME" \
      --node-count 1 \
      --node-vm-size "$VM_SKU" \
      --cluster-name "$CLUSTER" \
      --os-type Linux \
      --max-pods 250 \
      --subscription "$SUBSCRIPTION_ID" \
      --tags fastpathenabled=true aks-nic-enable-multi-tenancy=true stampcreatorserviceinfo=true "aks-nic-secondary-count=${PODS_PER_NODE}" \
      --aks-custom-headers AKSHTTPCustomFeatures=Microsoft.ContainerService/NetworkingMultiTenancyPreview \
      --pod-subnet-id "$POD_SUBNET_ID"
  else
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
  fi

  echo "    Node pool $POOL_NAME created (zone: ${ZONE})"
done

# Wait for zone pool nodes to be Ready, with VM health-check remediation
KUBECONFIG_FILE="/tmp/${CLUSTER}.kubeconfig"
az aks get-credentials -g "$RG" -n "$CLUSTER" --subscription "$SUBSCRIPTION_ID" --admin --overwrite-existing --file "$KUBECONFIG_FILE"

# Get the VMSS resource group (AKS manages nodes in MC_* RG)
MC_RG=$(az aks show -g "$RG" -n "$CLUSTER" --subscription "$SUBSCRIPTION_ID" --query "nodeResourceGroup" -o tsv)
echo "    Managed cluster RG: $MC_RG"

MAX_REMEDIATION_ATTEMPTS=2
INITIAL_WAIT_TIMEOUT=120   # seconds – short initial wait before checking VM health
POST_REMEDIATION_TIMEOUT=600  # seconds – longer wait after VM remediation

for ZONE in $ZONES; do
  POOL_NAME="npz${ZONE}"
  echo "==> Checking node pool $POOL_NAME for remediator taints before waiting for Ready"

  # Pre-check: remediate nodes tainted by AKS node auto-repair BEFORE waiting for Ready.
  # A tainted node can still be "Ready" in k8s but unschedulable for pods, so this must
  # run first to avoid skipping remediation.
  VMSS_NAME=$(az vmss list -g "$MC_RG" --subscription "$SUBSCRIPTION_ID" --query "[?contains(name,'${POOL_NAME}')].name" -o tsv | head -1)
  if [ -n "$VMSS_NAME" ]; then
    TAINTED_NODES=$(kubectl --kubeconfig "$KUBECONFIG_FILE" get nodes -l agentpool="$POOL_NAME" \
      -o jsonpath='{range .items[*]}{.metadata.name}{"\t"}{range .spec.taints[*]}{.key}{"\n"}{end}{end}' 2>/dev/null \
      | grep "remediator.kubernetes.azure.com/unschedulable" | cut -f1 | sort -u || true)

    if [ -n "$TAINTED_NODES" ]; then
      for TAINTED_NODE in $TAINTED_NODES; do
        echo "    WARNING: Node $TAINTED_NODE has remediator.kubernetes.azure.com/unschedulable taint"

        INSTANCE_ID=$(az vmss list-instances -g "$MC_RG" -n "$VMSS_NAME" --subscription "$SUBSCRIPTION_ID" \
          --query "[].{id:instanceId, computerName:osProfile.computerName}" -o json \
          | jq -r ".[] | select(.computerName == \"$TAINTED_NODE\") | .id")

        if [ -n "$INSTANCE_ID" ]; then
          echo "    Deleting tainted node's VMSS instance $INSTANCE_ID from $VMSS_NAME..."
          az vmss delete-instances -g "$MC_RG" -n "$VMSS_NAME" --subscription "$SUBSCRIPTION_ID" \
            --instance-ids "$INSTANCE_ID" --no-wait || true
        else
          echo "    Could not map node $TAINTED_NODE to a VMSS instance, skipping"
        fi
      done

      echo "    Waiting for VMSS to reconcile after tainted node removal..."
      sleep 30

      CURRENT_COUNT=$(az vmss show -g "$MC_RG" -n "$VMSS_NAME" --subscription "$SUBSCRIPTION_ID" --query "sku.capacity" -o tsv)
      if [ "$CURRENT_COUNT" -lt 1 ]; then
        echo "    VMSS capacity dropped to $CURRENT_COUNT, scaling back to 1..."
        az vmss scale -g "$MC_RG" -n "$VMSS_NAME" --subscription "$SUBSCRIPTION_ID" --new-capacity 1
      fi

      echo "    Waiting for replacement node to become Ready (timeout ${POST_REMEDIATION_TIMEOUT}s)..."
      kubectl --kubeconfig "$KUBECONFIG_FILE" wait --for=condition=Ready nodes \
        -l agentpool="$POOL_NAME" --timeout="${POST_REMEDIATION_TIMEOUT}s" 2>/dev/null || true
    fi
  fi

  echo "==> Waiting for node pool $POOL_NAME nodes to be Ready"

  attempt=0
  node_ready=false

  while [ "$node_ready" = "false" ] && [ $attempt -le $MAX_REMEDIATION_ATTEMPTS ]; do
    # Try a short wait first
    if kubectl --kubeconfig "$KUBECONFIG_FILE" wait --for=condition=Ready nodes \
        -l agentpool="$POOL_NAME" --timeout="${INITIAL_WAIT_TIMEOUT}s" 2>/dev/null; then
      echo "    Node pool $POOL_NAME nodes are Ready"
      node_ready=true
      break
    fi

    echo "    Nodes in pool $POOL_NAME not ready after ${INITIAL_WAIT_TIMEOUT}s, checking Azure VM health..."
    attempt=$((attempt + 1))

    # Find the VMSS backing this node pool
    VMSS_NAME=$(az vmss list -g "$MC_RG" --subscription "$SUBSCRIPTION_ID" --query "[?contains(name,'${POOL_NAME}')].name" -o tsv | head -1)
    if [ -z "$VMSS_NAME" ]; then
      echo "ERROR: Could not find VMSS for pool $POOL_NAME in $MC_RG"
      exit 1
    fi
    echo "    VMSS: $VMSS_NAME"

    # Check each instance in the VMSS
    remediated=false
    INSTANCES=$(az vmss list-instances -g "$MC_RG" -n "$VMSS_NAME" --subscription "$SUBSCRIPTION_ID" \
      --query "[].{id:instanceId, state:provisioningState, powerState:instanceView.statuses[?starts_with(code,'PowerState/')].displayStatus | [0]}" \
      -o json --expand instanceView)

    INSTANCE_COUNT=$(echo "$INSTANCES" | jq length)
    echo "    Found $INSTANCE_COUNT VMSS instance(s)"

    for i in $(seq 0 $((INSTANCE_COUNT - 1))); do
      INSTANCE_ID=$(echo "$INSTANCES" | jq -r ".[$i].id")
      PROV_STATE=$(echo "$INSTANCES" | jq -r ".[$i].state")
      POWER_STATE=$(echo "$INSTANCES" | jq -r ".[$i].powerState // \"unknown\"")

      echo "    Instance $INSTANCE_ID: provisioningState=$PROV_STATE, powerState=$POWER_STATE"

      if [ "$PROV_STATE" = "Failed" ] || [ "$POWER_STATE" = "VM stopped" ] || [ "$POWER_STATE" = "VM deallocated" ]; then
        echo "    WARNING: Instance $INSTANCE_ID is in unhealthy state ($PROV_STATE / $POWER_STATE)"
        echo "    Deleting failed instance $INSTANCE_ID from VMSS $VMSS_NAME..."

        # Delete the failed instance – VMSS auto-scaling will provision a replacement
        az vmss delete-instances -g "$MC_RG" -n "$VMSS_NAME" --subscription "$SUBSCRIPTION_ID" --instance-ids "$INSTANCE_ID" --no-wait || true

        remediated=true
      fi
    done

    if [ "$remediated" = "true" ]; then
      echo "    Deleted unhealthy instance(s). Waiting for VMSS to reconcile the desired count..."
      # Give VMSS time to detect the missing instance and start provisioning
      sleep 30

      # Ensure the scale set still has the right instance count (1 node per zone pool)
      CURRENT_COUNT=$(az vmss show -g "$MC_RG" -n "$VMSS_NAME" --subscription "$SUBSCRIPTION_ID" --query "sku.capacity" -o tsv)
      if [ "$CURRENT_COUNT" -lt 1 ]; then
        echo "    VMSS capacity dropped to $CURRENT_COUNT, scaling back to 1..."
        az vmss scale -g "$MC_RG" -n "$VMSS_NAME" --subscription "$SUBSCRIPTION_ID" --new-capacity 1
      fi

      echo "    Waiting for replacement node to become Ready (timeout ${POST_REMEDIATION_TIMEOUT}s)..."
      if kubectl --kubeconfig "$KUBECONFIG_FILE" wait --for=condition=Ready nodes \
          -l agentpool="$POOL_NAME" --timeout="${POST_REMEDIATION_TIMEOUT}s" 2>/dev/null; then
        echo "    Replacement node in pool $POOL_NAME is Ready"
        node_ready=true
      else
        echo "    Replacement node still not ready after ${POST_REMEDIATION_TIMEOUT}s (attempt $attempt/$MAX_REMEDIATION_ATTEMPTS)"
      fi
    else
      echo "    All VMs appear provisioned OK but K8s node not Ready. Waiting longer..."
      if kubectl --kubeconfig "$KUBECONFIG_FILE" wait --for=condition=Ready nodes \
          -l agentpool="$POOL_NAME" --timeout="${POST_REMEDIATION_TIMEOUT}s" 2>/dev/null; then
        echo "    Node pool $POOL_NAME nodes are now Ready"
        node_ready=true
      else
        echo "    Node pool $POOL_NAME still not ready (attempt $attempt/$MAX_REMEDIATION_ATTEMPTS)"
      fi
    fi
  done

  if [ "$node_ready" = "false" ]; then
    echo "ERROR: Node pool $POOL_NAME nodes failed to become Ready after $MAX_REMEDIATION_ATTEMPTS remediation attempts"
    exit 1
  fi
done

# Label the zone node pool nodes
for ZONE in $ZONES; do
  POOL_NAME="npz${ZONE}"

  echo "==> Labeling and tainting nodes in pool $POOL_NAME"
  kubectl --kubeconfig "$KUBECONFIG_FILE" label nodes -l agentpool=$POOL_NAME \
    longrunning-zone-pool=true \
    --overwrite

  # Taint zone pool nodes so only test pods with the matching toleration can schedule here.
  # This prevents stray workloads from consuming vnet-nic capacity.
  kubectl --kubeconfig "$KUBECONFIG_FILE" taint nodes -l agentpool=$POOL_NAME \
    acn-test/zone-pool=true:NoSchedule \
    --overwrite

  # Verify zone label (AKS sets this automatically for zonal node pools)
  NODE=$(kubectl --kubeconfig "$KUBECONFIG_FILE" get nodes -l agentpool=$POOL_NAME -o jsonpath='{.items[0].metadata.name}')
  if [ "$ZONE" = "0" ]; then
    echo "    Node $NODE is a generic (non-zonal) node pool, skipping zone label verification"
  else
    ACTUAL_ZONE=$(kubectl --kubeconfig "$KUBECONFIG_FILE" get node "$NODE" -o jsonpath='{.metadata.labels.topology\.kubernetes\.io/zone}')
    echo "    Node $NODE zone label: $ACTUAL_ZONE"

    # The Go tests and DaemonSet manifests expect the zone label to be "<region>-<zone>" (e.g., "eastus2euap-1").
    # Fail fast if AKS uses a different format so we can fix the code before tests silently fail.
    LOCATION=$(az aks show -g "$RG" -n "$CLUSTER" --subscription "$SUBSCRIPTION_ID" --query location -o tsv)
    EXPECTED_ZONE="${LOCATION}-${ZONE}"
    if [ "$ACTUAL_ZONE" != "$EXPECTED_ZONE" ]; then
      echo "ERROR: Zone label mismatch! Expected '$EXPECTED_ZONE', got '$ACTUAL_ZONE'"
      echo "       The Go tests use '<location>-<zone>' format (e.g., 'eastus2euap-1')."
      echo "       Update GetZoneLabel() in datapath_longrunning_shared.go and daemonset.yaml if format differs."
      exit 1
    fi
    echo "    Zone label verified: $ACTUAL_ZONE == $EXPECTED_ZONE"
  fi
done

echo "==> Zone node pool setup complete"
echo "==> Node summary:"
kubectl --kubeconfig "$KUBECONFIG_FILE" get nodes -l longrunning-zone-pool=true \
  -o custom-columns='NAME:.metadata.name,ZONE:.metadata.labels.topology\.kubernetes\.io/zone,POOL:.metadata.labels.agentpool' \
  --sort-by='.metadata.labels.topology\.kubernetes\.io/zone'
