#!/usr/bin/env bash
# run-migration.sh — Execute actual NPM-to-Cilium migration (manual step).
# WARNING: This performs an irreversible cluster migration.
#
# Prerequisites:
#   1. setup.sh has been run successfully
#   2. run-validation.sh reports all PASS
#   3. You have reviewed the dry-run output
set -euo pipefail

###############################################################################
# Configuration
###############################################################################
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd "${SCRIPT_DIR}/../.." && pwd)"
BINARY="${REPO_ROOT}/aksmigrate"
OUTPUT_DIR="${SCRIPT_DIR}/output"
K8S_VERSION="${K8S_VERSION:-1.31}"
CLUSTER_NAME="${CLUSTER_NAME:-aksmigrate-e2e}"
RESOURCE_GROUP="${RESOURCE_GROUP:-aksmigrate-e2e-test}"
TRANSLATE_DIR="${OUTPUT_DIR}/cilium-patches"
SNAPSHOT_FILE="${OUTPUT_DIR}/pre-migration-snapshot.json"

# Colors
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
CYAN='\033[0;36m'
NC='\033[0m'

info()  { echo -e "${CYAN}[INFO]${NC}  $*"; }
ok()    { echo -e "${GREEN}[OK]${NC}    $*"; }
warn()  { echo -e "${YELLOW}[WARN]${NC}  $*"; }
die()   { echo -e "${RED}[FAIL]${NC}  $*"; exit 1; }

###############################################################################
# Safety check
###############################################################################
echo -e "${RED}WARNING: This script performs an actual NPM-to-Cilium migration.${NC}"
echo "Cluster: ${CLUSTER_NAME} in ${RESOURCE_GROUP}"
echo ""
read -r -p "Type 'migrate' to confirm: " CONFIRM
if [[ "${CONFIRM}" != "migrate" ]]; then
    echo "Aborted."
    exit 0
fi

###############################################################################
# Preflight
###############################################################################
[[ -x "${BINARY}" ]] || die "Binary not found at ${BINARY}. Run setup.sh first."
kubectl cluster-info > /dev/null 2>&1 || die "Cannot connect to cluster."

###############################################################################
# Step 1: Apply translated patches
###############################################################################
info "Step 1: Applying translated patches..."

if [[ -d "${TRANSLATE_DIR}/patched" ]]; then
    info "Applying patched NetworkPolicies..."
    kubectl apply -f "${TRANSLATE_DIR}/patched/"
    ok "Patched NetworkPolicies applied."
fi

if [[ -d "${TRANSLATE_DIR}/cilium" ]]; then
    warn "CiliumNetworkPolicies will be applied after migration (CRDs not yet available)."
fi

###############################################################################
# Step 2: Execute migration
###############################################################################
info "Step 2: Migrating cluster to Cilium dataplane..."
az aks update \
    --resource-group "${RESOURCE_GROUP}" \
    --name "${CLUSTER_NAME}" \
    --network-dataplane cilium \
    --output none
ok "Migration command submitted."

###############################################################################
# Step 3: Wait for nodes to become Ready
###############################################################################
info "Step 3: Waiting for nodes to become Ready..."
for i in $(seq 1 60); do
    NOT_READY=$(kubectl get nodes --no-headers 2>/dev/null | grep -cv " Ready " || true)
    if [[ "${NOT_READY}" -eq 0 ]]; then
        ok "All nodes are Ready."
        break
    fi
    info "  ${NOT_READY} node(s) not ready yet... (attempt ${i}/60)"
    sleep 30
done

# Verify nodes
kubectl get nodes

###############################################################################
# Step 4: Apply CiliumNetworkPolicies
###############################################################################
if [[ -d "${TRANSLATE_DIR}/cilium" ]]; then
    info "Step 4: Applying CiliumNetworkPolicies..."
    kubectl apply -f "${TRANSLATE_DIR}/cilium/"
    ok "CiliumNetworkPolicies applied."
fi

###############################################################################
# Step 5: Post-migration connectivity validation
###############################################################################
if [[ -f "${SNAPSHOT_FILE}" ]]; then
    info "Step 5: Running post-migration connectivity validation..."
    "${BINARY}" conntest validate \
        --pre-snapshot "${SNAPSHOT_FILE}" \
        --output "${OUTPUT_DIR}/post-migration-snapshot.json" 2>&1 | tee "${OUTPUT_DIR}/validation.log"
    ok "Post-migration validation complete. Check ${OUTPUT_DIR}/validation.log"
else
    warn "Step 5: No pre-migration snapshot found, skipping validation."
    warn "  Run 'aksmigrate conntest snapshot --phase post-migration' manually."
fi

###############################################################################
# Summary
###############################################################################
echo ""
echo "============================================="
echo -e "${GREEN}Migration complete!${NC}"
echo "============================================="
echo "Cluster: ${CLUSTER_NAME}"
echo "Dataplane: cilium"
echo ""
echo "Verify with:"
echo "  kubectl get pods -n kube-system | grep cilium"
echo "  az aks show -g ${RESOURCE_GROUP} -n ${CLUSTER_NAME} --query networkProfile"
