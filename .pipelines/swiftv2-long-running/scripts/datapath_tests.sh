#!/usr/bin/env bash
set -euo pipefail

# --------------------------
# Parameters / Environment
# --------------------------
RG=$1
BUILD_ID=$2

# Clusters
CLUSTER1="aks-1"
CLUSTER2="aks-2"

# VNet/Subnet mappings for test
declare -A VN_SUB_MAP
VN_SUB_MAP["vnet_a1"]="s1 s2"
VN_SUB_MAP["vnet_a2"]="s1"
VN_SUB_MAP["vnet_a3"]="s1"
VN_SUB_MAP["vnet_b1"]="s1"

# PN/PNI base names
PN_PREFIX="pn-${BUILD_ID}"
PNI_PREFIX="pni-${BUILD_ID}"

source ./helpers_network_ids.sh

# Create PodNetwork in a specific cluster
create_pn() {
    local cluster_context=$1   # kubeconfig or context name
    local pn_name=$2            # PodNetwork name
    local vnet_name=$3
    local subnet_name=$4

    echo "=== Creating PodNetwork ${pn_name} in cluster ${cluster_context} ==="

    # --- Fetch IDs ---
    VNET_GUID=$(get_vnet_guid "$RG" "$vnet_name")
    SUBNET_ARM_ID=$(get_subnet_arm_id "$RG" "$vnet_name" "$subnet_name")
    SUBNET_GUID=$(get_subnet_guid "$RG" "$vnet_name" "$subnet_name")

    echo "VNET_GUID: $VNET_GUID"
    echo "SUBNET_GUID: $SUBNET_GUID"
    echo "SUBNET_ARM_ID: $SUBNET_ARM_ID"

    # --- Create PodNetwork ---
    ./create_pn.sh \
        "$cluster_context" \
        "$pn_name" \
        "$VNET_GUID" \
        "$SUBNET_GUID" \
        "$SUBNET_ARM_ID"

    echo "PodNetwork ${pn_name} submitted successfully."
}


create_pni() {
    local KUBECONFIG_PATH=$1
    local NAMESPACE=$2
    local pni_name=$3
    local pod_network_name=$4
    local pni_type=$5
    local reservations=${6:-0}
    local cluster=$7

    echo "Creating PodNetworkInstance $pni_name for PN $pod_network_name on $cluster"
    ./create_pni.sh "$KUBECONFIG_PATH" "$NAMESPACE" "$pni_name" "$pod_network_name" "$pni_type" "$reservations" "$cluster"
}

create_pod_on_node() {
    local cluster="$1"
    local pn_name="$2"
    local pni_name="$3"
    local node_name="$4"
    local pod_name="$5"

    KUBECONFIG_PATH="/tmp/${cluster}.kubeconfig"
    echo "Creating pod '$pod_name' on node '$node_name' (PN: $pn_name, PNI: $pni_name)..."
    ./create_pod.sh "$pod_name" "$node_name" "linux" "$pn_name" "$pni_name" "weibeld/ubuntu-networking" "$KUBECONFIG_PATH"
}

get_nodes() {
    local cluster=$1
    kubectl --context "$cluster" get nodes -o name | sed 's|node/||'
}


# --- Part 1: Customer2 in aks-2 / vnet_b1/s1 ---
PN_C2="${PN_PREFIX}-c2"
PNI_C2="${PNI_PREFIX}-c2"

create_pn "/tmp/${CLUSTER2}.kubeconfig" "$PN_C2" "vnet_b1" "s1"
create_pni "/tmp/${CLUSTER2}.kubeconfig" "$PN_C2" "$PNI_C2" "$PN_C2" "explicit" "2" "$CLUSTER2"

# Create 2 pods for Customer2, one per node in aks-2
NODES_CLUSTER2=($(get_nodes "$CLUSTER2"))
for i in 0 1; do
    POD_NAME="pod-c2-$i"
    NODE_NAME="${NODES_CLUSTER2[$i]}"
    create_pod_on_node "$CLUSTER2" "$PN_C2" "$PNI_C2" "$NODE_NAME" "$POD_NAME"
done

# # --- Part 2: Other PNs/PNIs across multiple subnets ---
# PN_LIST=()
# PNI_LIST=()

# for vnet in "${!VN_SUB_MAP[@]}"; do
#     for subnet in ${VN_SUB_MAP[$vnet]}; do
#         PN_NAME="${PN_PREFIX}-${vnet}-${subnet}"
#         PNI_NAME="${PNI_PREFIX}-${vnet}-${subnet}"
#         PN_LIST+=("$PN_NAME")
#         PNI_LIST+=("$PNI_NAME")
#         # Assume cluster selection: default to aks-1 unless aks-2 needs pods
#         CLUSTER="$CLUSTER1"
#         create_pn "$CLUSTER" "$PN_NAME"
#         create_pni "$CLUSTER" "$PNI_NAME" "$PN_NAME" "$vnet" "$subnet"
#     done
# done

# # --- Part 3: Create 6 pods under these PN/PNI ---
# # 4 pods go to aks-1 nodes, 2 pods go to remaining aks-2 nodes

# # Get node lists
# NODES_CLUSTER1=($(get_nodes "$CLUSTER1"))
# NODES_CLUSTER2=($(get_nodes "$CLUSTER2")) 

# # 4 pods in aks-1, assign one per node
# for i in 0 1 2 3; do
#     POD_NAME="pod-${BUILD_ID}-c1-$i"
#     PN_IDX=$((i % ${#PN_LIST[@]}))
#     create_pod_on_node "$CLUSTER1" "${PN_LIST[$PN_IDX]}" "${PNI_LIST[$PN_IDX]}" "${NODES_CLUSTER1[$i]}" "$POD_NAME"
# done

# # Remaining 2 pods in aks-2, assign to leftover nodes
# for i in 0 1; do
#     POD_NAME="pod-${BUILD_ID}-c2-$i"
#     PN_IDX=$(( (i+4) % ${#PN_LIST[@]} ))
#     NODE_IDX=$i
#     create_pod_on_node "$CLUSTER2" "${PN_LIST[$PN_IDX]}" "${PNI_LIST[$PN_IDX]}" "${NODES_CLUSTER2[$NODE_IDX]}" "$POD_NAME"
# done

# echo "Datapath test pods created successfully."
