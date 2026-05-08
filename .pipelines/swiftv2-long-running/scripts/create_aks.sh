#!/usr/bin/env bash
set -euo pipefail
trap 'echo "[ERROR] Failed during Resource group or AKS cluster creation." >&2' ERR
SUBSCRIPTION_ID=$1
LOCATION=$2
RG=$3
VM_SKU_DEFAULT=$4
VM_SKU_HIGHNIC=$5
DELEGATOR_APP_NAME=$6
DELEGATOR_RG=$7
DELEGATOR_SUB=$8
# Optional managed Windows nodepool. Off by default; set to "true" (typically via
# the pipeline scenario flag) to provision the npwin pool. Normalize to lowercase
# because Azure DevOps serializes YAML booleans as "True"/"False".
ENABLE_MANAGED_WINDOWS=$(echo "${9:-false}" | tr '[:upper:]' '[:lower:]')
DELEGATOR_BASE_URL=${10:-"http://localhost:8080"}
WINDOWS_ADMIN_USERNAME="${WINDOWS_ADMIN_USERNAME:-azureuser}"

CLUSTER_COUNT=2
PODS_PER_NODE=7
CLUSTER_PREFIX="aks"

echo "Setting active subscription to $SUBSCRIPTION_ID"
az account set --subscription "$SUBSCRIPTION_ID"


stamp_vnet() {
    local vnet_id="$1"

    # Idempotency guard: stamp_vnet sets the 'stampcreatorserviceinfo=true' tag
    # on the VNet as a side-effect of a successful PUT to the delegator. If the
    # tag is already present, the cluster's VNet is already registered with DNC
    # and re-stamping is unnecessary. This lets stamp_vnet be called on every
    # pipeline run without depending on cluster-creation state.
    local existing_tag
    existing_tag=$(az network vnet show --ids "$vnet_id" \
      --query "tags.stampcreatorserviceinfo" -o tsv 2>/dev/null || true)
    if [[ "$existing_tag" == "true" ]]; then
        echo "VNet $vnet_id already stamped (stampcreatorserviceinfo=true). Skipping."
        return 0
    fi

    responseFile="response.txt"
    modified_vnet="${vnet_id//\//%2F}"
    cmd_stamp_curl="'curl -v -X PUT ${DELEGATOR_BASE_URL}/VirtualNetwork/$modified_vnet/stampcreatorservicename'"
    cmd_containerapp_exec="az containerapp exec -n $DELEGATOR_APP_NAME -g $DELEGATOR_RG --subscription $DELEGATOR_SUB --command $cmd_stamp_curl"
    
    max_retries=10
    sleep_seconds=15
    retry_count=0

    while [[ $retry_count -lt $max_retries ]]; do
        script --quiet -c "$cmd_containerapp_exec" "$responseFile"
        if grep -qF "200 OK" "$responseFile"; then
            echo "Subnet Delegator successfully stamped the vnet"
            return 0
        else
            echo "Subnet Delegator failed to stamp the vnet, attempt $((retry_count+1))"
            cat "$responseFile"
            retry_count=$((retry_count+1))
            sleep "$sleep_seconds"
        fi
    done

    echo "Failed to stamp the vnet even after $max_retries attempts"
    exit 1
}

wait_for_provisioning() {
  local rg="$1" clusterName="$2"
  echo "Waiting for AKS '$clusterName' in RG '$rg'..."
  local max_attempts=40
  local attempt=0
  
  while [[ $attempt -lt $max_attempts ]]; do
    state=$(az aks show --resource-group "$rg" --name "$clusterName" --query provisioningState -o tsv 2>/dev/null || true)
    echo "Attempt $((attempt+1))/$max_attempts - Provisioning state: $state"
    
    if [[ "$state" =~ Succeeded ]]; then
      echo "Provisioning succeeded"
      return 0
    fi
    if [[ "$state" =~ Failed|Canceled ]]; then
      echo "Provisioning finished with state: $state"
      return 1
    fi
    
    attempt=$((attempt+1))
    sleep 15
  done
  
  echo "Timeout waiting for AKS cluster provisioning after $((max_attempts * 15)) seconds"
  return 1
}

# ensure_windows_enabled makes sure the cluster has Windows admin credentials so
# that managed Windows nodepools can be added. Idempotent: if windowsProfile is
# already set, it does nothing.
ensure_windows_enabled() {
  local cluster=$1 rg=$2 sub=$3
  local existing_user
  existing_user=$(az aks show -g "$rg" -n "$cluster" --subscription "$sub" \
    --query "windowsProfile.adminUsername" -o tsv 2>/dev/null || true)
  if [[ -n "$existing_user" && "$existing_user" != "null" ]]; then
    echo "Cluster $cluster already has Windows enabled (admin: $existing_user). Skipping enable."
    return 0
  fi

  # Random per-enable password. Cluster nodes are not used interactively; this
  # credential only matters to AKS for provisioning the Windows pool. Subsequent
  # runs see windowsProfile already set and skip.
  local pwd
  pwd="$(openssl rand -base64 24 | tr -dc 'A-Za-z0-9')Aa1!"

  echo "Enabling Windows on cluster $cluster (admin: $WINDOWS_ADMIN_USERNAME)"
  if ! az aks update -g "$rg" -n "$cluster" --subscription "$sub" \
      --windows-admin-username "$WINDOWS_ADMIN_USERNAME" \
      --windows-admin-password "$pwd" >/dev/null; then
    echo "[ERROR] Failed to enable Windows on cluster $cluster. The cluster may" >&2
    echo "        need to be recreated with --windows-admin-* at create time, or" >&2
    echo "        the AKS API version may not support enabling Windows on an" >&2
    echo "        existing cluster. Disable enableManagedWindows for this scenario" >&2
    echo "        or recreate the cluster to proceed." >&2
    return 1
  fi
}

for i in $(seq 1 "$CLUSTER_COUNT"); do
    echo "Creating cluster #$i..."

    CLUSTER_NAME="${CLUSTER_PREFIX}-${i}"

    # Check if cluster already exists and is healthy
    EXISTING_STATE=$(az aks show -g "$RG" -n "$CLUSTER_NAME" --query provisioningState -o tsv 2>/dev/null || true)
    if [[ "$EXISTING_STATE" == "Succeeded" ]]; then
      echo "Cluster $CLUSTER_NAME already exists (state: $EXISTING_STATE). Skipping creation."
    else
      make -C ./hack/aks azcfg AZCLI=az REGION=$LOCATION
      make -C ./hack/aks swiftv2-podsubnet-cluster-up \
        AZCLI=az REGION=$LOCATION \
        SUB=$SUBSCRIPTION_ID \
        GROUP=$RG \
        CLUSTER=$CLUSTER_NAME \
        VM_SIZE=$VM_SKU_DEFAULT
      wait_for_provisioning "$RG" "$CLUSTER_NAME"
    fi

    # Always (re-)stamp the cluster VNet. stamp_vnet is idempotent (it short-
    # circuits when stampcreatorserviceinfo=true is already set), so this is
    # safe to run on every pipeline run. This also self-heals clusters that
    # were created in a prior run where the stamp call failed or was skipped.
    vnet_id=$(az network vnet show -g "$RG" --name "$CLUSTER_NAME" --query id -o tsv)
    stamp_vnet "$vnet_id"

    # Add high-NIC nodepool if it doesn't exist
    NPLINUX_EXISTS=$(az aks nodepool show -g "$RG" --cluster-name "$CLUSTER_NAME" -n nplinux --query provisioningState -o tsv 2>/dev/null || true)
    if [[ -n "$NPLINUX_EXISTS" ]]; then
      echo "Nodepool nplinux already exists on $CLUSTER_NAME (state: $NPLINUX_EXISTS). Skipping."
    else
      make -C ./hack/aks linux-swiftv2-nodepool-up \
        AZCLI=az REGION=$LOCATION \
        GROUP=$RG \
        VM_SIZE=$VM_SKU_HIGHNIC \
        PODS_PER_NODE=$PODS_PER_NODE \
        CLUSTER=$CLUSTER_NAME \
        SUB=$SUBSCRIPTION_ID
    fi

    # Optional managed Windows swiftv2 nodepools. Mirrors the Linux topology:
    #   npwin  = high-NIC ($VM_SKU_HIGHNIC, aks-nic-secondary-count=$PODS_PER_NODE)
    #   npwinl = low-NIC  ($VM_SKU_DEFAULT, aks-nic-secondary-count=1)
    # Both share the same multi-tenancy tags/headers as nplinux/nodepool1.
    if [[ "$ENABLE_MANAGED_WINDOWS" == "true" ]]; then
      NPWIN_EXISTS=$(az aks nodepool show -g "$RG" --cluster-name "$CLUSTER_NAME" -n npwin --query provisioningState -o tsv 2>/dev/null || true)
      if [[ -n "$NPWIN_EXISTS" ]]; then
        echo "Nodepool npwin already exists on $CLUSTER_NAME (state: $NPWIN_EXISTS). Skipping."
      else
        ensure_windows_enabled "$CLUSTER_NAME" "$RG" "$SUBSCRIPTION_ID"

        POD_SUBNET_ID="/subscriptions/${SUBSCRIPTION_ID}/resourceGroups/${RG}/providers/Microsoft.Network/virtualNetworks/${CLUSTER_NAME}/subnets/podnet"
        echo "Adding managed Windows swiftv2 nodepool npwin on $CLUSTER_NAME"
        az aks nodepool add -g "$RG" -n npwin \
          --cluster-name "$CLUSTER_NAME" \
          --subscription "$SUBSCRIPTION_ID" \
          --node-count 2 \
          --node-vm-size "$VM_SKU_HIGHNIC" \
          --os-type Windows \
          --os-sku Windows2022 \
          --max-pods 250 \
          --tags fastpathenabled=true aks-nic-enable-multi-tenancy=true stampcreatorserviceinfo=true "aks-nic-secondary-count=${PODS_PER_NODE}" \
          --aks-custom-headers AKSHTTPCustomFeatures=Microsoft.ContainerService/NetworkingMultiTenancyPreview \
          --pod-subnet-id "$POD_SUBNET_ID"
      fi

      NPWINL_EXISTS=$(az aks nodepool show -g "$RG" --cluster-name "$CLUSTER_NAME" -n npwinl --query provisioningState -o tsv 2>/dev/null || true)
      if [[ -n "$NPWINL_EXISTS" ]]; then
        echo "Nodepool npwinl already exists on $CLUSTER_NAME (state: $NPWINL_EXISTS). Skipping."
      else
        ensure_windows_enabled "$CLUSTER_NAME" "$RG" "$SUBSCRIPTION_ID"

        POD_SUBNET_ID="/subscriptions/${SUBSCRIPTION_ID}/resourceGroups/${RG}/providers/Microsoft.Network/virtualNetworks/${CLUSTER_NAME}/subnets/podnet"
        echo "Adding managed Windows swiftv2 low-NIC nodepool npwinl on $CLUSTER_NAME"
        az aks nodepool add -g "$RG" -n npwinl \
          --cluster-name "$CLUSTER_NAME" \
          --subscription "$SUBSCRIPTION_ID" \
          --node-count 2 \
          --node-vm-size "$VM_SKU_DEFAULT" \
          --os-type Windows \
          --os-sku Windows2022 \
          --max-pods 30 \
          --tags fastpathenabled=true aks-nic-enable-multi-tenancy=true stampcreatorserviceinfo=true "aks-nic-secondary-count=1" \
          --aks-custom-headers AKSHTTPCustomFeatures=Microsoft.ContainerService/NetworkingMultiTenancyPreview \
          --pod-subnet-id "$POD_SUBNET_ID"
      fi
    fi

    az aks get-credentials -g "$RG" -n "$CLUSTER_NAME" --admin --overwrite-existing \
      --file "/tmp/${CLUSTER_NAME}.kubeconfig"
    
    echo "Waiting for all nodes in $CLUSTER_NAME to be Ready..."
    kubectl --kubeconfig "/tmp/${CLUSTER_NAME}.kubeconfig" wait --for=condition=Ready nodes --all --timeout=10m
done

echo "All clusters complete."
