#!/usr/bin/env bash
# setup.sh — Create AKS cluster and deploy e2e test scenarios for aksmigrate validation.
# Idempotent: safe to re-run. Skips resources that already exist.
set -euo pipefail

###############################################################################
# Configuration
###############################################################################
RESOURCE_GROUP="${RESOURCE_GROUP:-aksmigrate-e2e-test}"
CLUSTER_NAME="${CLUSTER_NAME:-aksmigrate-e2e}"
REGION="${REGION:-eastus2}"
K8S_VERSION="${K8S_VERSION:-1.32}"
NODE_COUNT="${NODE_COUNT:-2}"
VM_SIZE="${VM_SIZE:-Standard_DS2_v2}"

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd "${SCRIPT_DIR}/../.." && pwd)"
MANIFESTS_DIR="${SCRIPT_DIR}/manifests"
BINARY="${REPO_ROOT}/aksmigrate"

# Colors
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
CYAN='\033[0;36m'
NC='\033[0m' # No Color

info()  { echo -e "${CYAN}[INFO]${NC}  $*"; }
ok()    { echo -e "${GREEN}[OK]${NC}    $*"; }
warn()  { echo -e "${YELLOW}[WARN]${NC}  $*"; }
fail()  { echo -e "${RED}[FAIL]${NC}  $*"; exit 1; }

###############################################################################
# Step 1: Resource Group
###############################################################################
info "Checking resource group '${RESOURCE_GROUP}'..."
if az group exists --name "${RESOURCE_GROUP}" | grep -q true; then
    ok "Resource group '${RESOURCE_GROUP}' already exists."
else
    info "Creating resource group '${RESOURCE_GROUP}' in '${REGION}'..."
    az group create --name "${RESOURCE_GROUP}" --location "${REGION}" --output none
    ok "Resource group created."
fi

###############################################################################
# Step 2: AKS Cluster
###############################################################################
info "Checking AKS cluster '${CLUSTER_NAME}'..."
if az aks show --resource-group "${RESOURCE_GROUP}" --name "${CLUSTER_NAME}" --output none 2>/dev/null; then
    ok "AKS cluster '${CLUSTER_NAME}' already exists."
else
    info "Creating AKS cluster '${CLUSTER_NAME}' (K8s ${K8S_VERSION}, NPM, ${NODE_COUNT} nodes)..."
    az aks create \
        --resource-group "${RESOURCE_GROUP}" \
        --name "${CLUSTER_NAME}" \
        --kubernetes-version "${K8S_VERSION}" \
        --network-plugin azure \
        --network-policy azure \
        --node-count "${NODE_COUNT}" \
        --node-vm-size "${VM_SIZE}" \
        --generate-ssh-keys \
	 --tier premium \
        --output none
    ok "AKS cluster created."
fi

###############################################################################
# Step 3: Get Credentials
###############################################################################
info "Fetching kubeconfig for '${CLUSTER_NAME}'..."
az aks get-credentials \
    --resource-group "${RESOURCE_GROUP}" \
    --name "${CLUSTER_NAME}" \
    --overwrite-existing \
    --output none
ok "Kubeconfig configured."

# Verify connectivity
info "Verifying cluster connectivity..."
kubectl cluster-info > /dev/null 2>&1 || fail "Cannot connect to cluster."
ok "Connected to cluster."

###############################################################################
# Step 4: Build binary
###############################################################################
info "Building aksmigrate binary..."
cd "${REPO_ROOT}"
go build -o "${BINARY}" ./cmd/aksmigrate
ok "Binary built: ${BINARY}"

###############################################################################
# Step 5: Deploy scenario manifests
###############################################################################
SCENARIOS=(
    "scenario1-ipblock"
    "scenario2-namedports"
    "scenario3-endport"
    "scenario4-lb-ingress"
    "scenario5-host-egress"
    "scenario6-combined"
)

# Apply namespaces first (other resources depend on them existing)
info "Creating namespaces..."
for scenario in "${SCENARIOS[@]}"; do
    kubectl apply -f "${MANIFESTS_DIR}/${scenario}/namespace.yaml"
done
ok "All namespaces created."

# Now apply the remaining resources
for scenario in "${SCENARIOS[@]}"; do
    info "Deploying ${scenario}..."
    kubectl apply -f "${MANIFESTS_DIR}/${scenario}/"
    ok "${scenario} deployed."
done

###############################################################################
# Step 6: Wait for pods to be Ready
###############################################################################
NAMESPACES=(
    "e2e-ipblock"
    "e2e-namedports"
    "e2e-endport"
    "e2e-lb-ingress"
    "e2e-host-egress"
    "e2e-combined"
)

info "Waiting for all pods to be Ready (timeout 5m)..."
for ns in "${NAMESPACES[@]}"; do
    if ! kubectl wait --for=condition=Available deployment --all \
        --namespace="${ns}" --timeout=300s 2>/dev/null; then
        warn "Some deployments in ${ns} are not ready yet, continuing..."
    else
        ok "All deployments in ${ns} are Ready."
    fi
done

###############################################################################
# Step 7: Wait for LoadBalancer IP assignment
###############################################################################
info "Waiting for LoadBalancer IPs to be assigned..."
for ns in "${NAMESPACES[@]}"; do
    lb_services=$(kubectl get svc -n "${ns}" -o json | \
        jq -r '.items[] | select(.spec.type=="LoadBalancer") | .metadata.name' 2>/dev/null || true)
    for svc in ${lb_services}; do
        info "  Waiting for LB IP on ${ns}/${svc}..."
        for i in $(seq 1 60); do
            ip=$(kubectl get svc "${svc}" -n "${ns}" -o jsonpath='{.status.loadBalancer.ingress[0].ip}' 2>/dev/null || true)
            if [[ -n "${ip}" ]]; then
                ok "  ${ns}/${svc} has LB IP: ${ip}"
                break
            fi
            sleep 5
        done
        if [[ -z "${ip:-}" ]]; then
            warn "  Timed out waiting for LB IP on ${ns}/${svc}"
        fi
    done
done

###############################################################################
# Summary
###############################################################################
echo ""
echo "============================================="
echo -e "${GREEN}Setup complete!${NC}"
echo "============================================="
echo "Resource Group: ${RESOURCE_GROUP}"
echo "Cluster:        ${CLUSTER_NAME}"
echo "Region:         ${REGION}"
echo "K8s Version:    ${K8S_VERSION}"
echo "Binary:         ${BINARY}"
echo ""
echo "Namespaces deployed:"
for ns in "${NAMESPACES[@]}"; do
    echo "  - ${ns}"
done
echo ""
echo "Next steps:"
echo "  1. Run validation: ./test/e2e/run-validation.sh"
echo "  2. Run Go tests:   go test -v -tags e2e -timeout 30m ./test/e2e/"
echo "  3. Teardown:       ./test/e2e/teardown.sh"
