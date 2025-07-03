#!/usr/bin/env bash

# Script to create a BYO Cilium cluster with CNS and Cilium deployment
# This script orchestrates the creation of an AKS cluster with BYO CNI (no kube-proxy),
# deploys Azure CNS, and installs Cilium networking.

set -euo pipefail

# Default configuration
DEFAULT_CLUSTER_NAME="byocni-cluster"
DEFAULT_SUB=""
DEFAULT_CNS_VERSION="v1.5.38"
DEFAULT_AZURE_IPAM_VERSION="v0.3.0"
DEFAULT_CILIUM_DIR="1.14"
DEFAULT_CILIUM_IMAGE_REGISTRY="acnpublic.azurecr.io"
DEFAULT_CILIUM_VERSION_TAG=""
DEFAULT_IPV6_HP_BPF_VERSION=""
DEFAULT_CNS_IMAGE_REPO="MCR"
DEFAULT_AZCLI="az"

# Script configuration
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd "${SCRIPT_DIR}/../.." && pwd)"

# Function to display usage
usage() {
    cat << EOF
Usage: $0 [OPTIONS]

Creates a BYO Cilium cluster with the following steps:
1. Creates an AKS cluster with overlay networking and no kube-proxy
2. Deploys Azure CNS to the cluster
3. Deploys Cilium networking components

OPTIONS:
    -c, --cluster CLUSTER_NAME      Name of the AKS cluster (default: ${DEFAULT_CLUSTER_NAME})
    -s, --subscription SUB_ID       Azure subscription ID (required)
    -z, --azcli AZCLI_COMMAND      Azure CLI command (default: ${DEFAULT_AZCLI})
    --cns-version VERSION           CNS version to deploy (default: ${DEFAULT_CNS_VERSION})
    --azure-ipam-version VERSION    Azure IPAM version (default: ${DEFAULT_AZURE_IPAM_VERSION})
    --cilium-dir DIR                Cilium version directory (default: ${DEFAULT_CILIUM_DIR})
    --cilium-registry REGISTRY      Cilium image registry (default: ${DEFAULT_CILIUM_IMAGE_REGISTRY})
    --cilium-version-tag TAG        Cilium version tag (default: auto-detected)
    --ipv6-hp-bpf-version VERSION   IPv6 HP BPF version for dualstack (default: auto-detected)
    --cns-image-repo REPO           CNS image repository (default: ${DEFAULT_CNS_IMAGE_REPO})
    --dry-run                       Show commands that would be executed without running them
    -h, --help                      Display this help message

EXAMPLES:
    # Basic usage with required subscription
    $0 --subscription 9b8218f9-902a-4d20-a65c-e98acec5362f

    # Custom cluster name and CNS version
    $0 --cluster my-cilium-cluster --subscription 9b8218f9-902a-4d20-a65c-e98acec5362f --cns-version v1.6.0

    # Using different Cilium version
    $0 --subscription 9b8218f9-902a-4d20-a65c-e98acec5362f --cilium-dir 1.16 --cilium-version-tag v1.16.0

    # Dry run to see what commands would be executed
    $0 --subscription 9b8218f9-902a-4d20-a65c-e98acec5362f --dry-run

EOF
}

# Function to log messages
log() {
    echo "[$(date '+%Y-%m-%d %H:%M:%S')] $*" >&2
}

# Function to log errors
error() {
    echo "[$(date '+%Y-%m-%d %H:%M:%S')] ERROR: $*" >&2
}

# Function to execute commands with optional dry-run
execute() {
    if [[ "${DRY_RUN}" == "true" ]]; then
        echo "DRY-RUN: $*"
    else
        log "Executing: $*"
        bash -c "$*"
    fi
}

# Function to check prerequisites
check_prerequisites() {
    log "Checking prerequisites..."
    
    # Check if we're in the right directory
    if [[ ! -f "${REPO_ROOT}/Makefile" ]]; then
        error "Not in the azure-container-networking repository root"
        exit 1
    fi
    
    # Check if Azure CLI is available
    if ! command -v "${AZCLI}" &> /dev/null; then
        error "Azure CLI (${AZCLI}) not found. Please install Azure CLI."
        exit 1
    fi
    
    # Check if kubectl is available
    if ! command -v kubectl &> /dev/null; then
        error "kubectl not found. Please install kubectl."
        exit 1
    fi
    
    # Check if envsubst is available
    if ! command -v envsubst &> /dev/null; then
        error "envsubst not found. Please install gettext package."
        exit 1
    fi
    
    # Check if make is available
    if ! command -v make &> /dev/null; then
        error "make not found. Please install make."
        exit 1
    fi
    
    # Verify Azure CLI is logged in
    if [[ "${DRY_RUN}" != "true" ]]; then
        if ! "${AZCLI}" account show &> /dev/null; then
            error "Azure CLI is not logged in. Please run '${AZCLI} login' first."
            exit 1
        fi
    fi
    
    log "Prerequisites check passed"
}

# Function to validate Cilium manifest directory
validate_cilium_dir() {
    local cilium_path="${REPO_ROOT}/test/integration/manifests/cilium/v${CILIUM_DIR}"
    if [[ ! -d "${cilium_path}" ]]; then
        error "Cilium directory v${CILIUM_DIR} not found at ${cilium_path}"
        error "Available Cilium versions:"
        # Use a more robust way to list versions
        find "${REPO_ROOT}/test/integration/manifests/cilium/" -maxdepth 1 -type d -name "v*" -printf "%f\n" | sort || true
        exit 1
    fi
    
    # Check required directories exist
    local required_dirs=("cilium-config" "cilium-operator/files" "cilium-operator/templates" "cilium-agent/files" "cilium-agent/templates")
    for dir in "${required_dirs[@]}"; do
        if [[ ! -d "${cilium_path}/${dir}" ]]; then
            error "Required Cilium directory ${dir} not found in v${CILIUM_DIR}"
            exit 1
        fi
    done
    
    log "Cilium directory v${CILIUM_DIR} validation passed"
}

# Function to auto-detect versions if not provided
detect_versions() {
    if [[ -z "${CILIUM_VERSION_TAG}" ]]; then
        # Try to extract version from directory name or use a sensible default
        case "${CILIUM_DIR}" in
            "1.14") CILIUM_VERSION_TAG="v1.14.8" ;;
            "1.16") CILIUM_VERSION_TAG="v1.16.0" ;;
            "1.17") CILIUM_VERSION_TAG="v1.17.0" ;;
            *) CILIUM_VERSION_TAG="v${CILIUM_DIR}.0" ;;
        esac
        log "Auto-detected Cilium version tag: ${CILIUM_VERSION_TAG}"
    fi
    
    if [[ -z "${IPV6_HP_BPF_VERSION}" ]]; then
        # Try to get from git tags or use default
        if command -v git &> /dev/null && [[ -d "${REPO_ROOT}/.git" ]]; then
            IPV6_HP_BPF_VERSION=$(cd "${REPO_ROOT}" && git describe --match "ipv6-hp-bpf*" --tags --always 2>/dev/null | head -1 || echo "")
        fi
        if [[ -z "${IPV6_HP_BPF_VERSION}" ]]; then
            IPV6_HP_BPF_VERSION="ipv6-hp-bpf-v0.1.0"
        fi
        log "Auto-detected IPv6 HP BPF version: ${IPV6_HP_BPF_VERSION}"
    fi
}

# Function to create AKS cluster
create_cluster() {
    log "Creating AKS cluster with BYO CNI and no kube-proxy..."
    
    local make_cmd="AZCLI=${AZCLI} CLUSTER=${CLUSTER_NAME} SUB=${SUBSCRIPTION} make overlay-byocni-nokubeproxy-up"
    
    execute "cd '${SCRIPT_DIR}' && ${make_cmd}"
    
    log "AKS cluster created successfully"
}

# Function to deploy CNS
deploy_cns() {
    log "Deploying Azure CNS to the cluster..."
    
    local make_cmd="sudo -E env \"PATH=\$PATH\" make test-load CNS_ONLY=true CNS_VERSION=${CNS_VERSION} AZURE_IPAM_VERSION=${AZURE_IPAM_VERSION} INSTALL_CNS=true INSTALL_OVERLAY=true CNS_IMAGE_REPO=${CNS_IMAGE_REPO}"
    
    execute "cd '${REPO_ROOT}' && ${make_cmd}"
    
    log "Azure CNS deployed successfully"
}

# Function to deploy Cilium
deploy_cilium() {
    log "Deploying Cilium networking components..."
    
    local cilium_path="${REPO_ROOT}/test/integration/manifests/cilium/v${CILIUM_DIR}"
    
    # Set environment variables for templating
    export DIR="${CILIUM_DIR}"
    export CILIUM_IMAGE_REGISTRY="${CILIUM_IMAGE_REGISTRY}"
    export CILIUM_VERSION_TAG="${CILIUM_VERSION_TAG}"
    export IPV6_HP_BPF_VERSION="${IPV6_HP_BPF_VERSION}"
    
    # Deploy Cilium config
    log "Applying Cilium configuration..."
    execute "kubectl apply -f '${cilium_path}/cilium-config/cilium-config.yaml'"
    
    # Deploy Cilium operator files
    log "Applying Cilium operator files..."
    execute "kubectl apply -f '${cilium_path}/cilium-operator/files'"
    
    # Deploy Cilium agent files
    log "Applying Cilium agent files..."
    execute "kubectl apply -f '${cilium_path}/cilium-agent/files'"
    
    # Deploy Cilium operator with environment substitution
    log "Applying Cilium operator deployment with templating..."
    execute "envsubst '\${CILIUM_VERSION_TAG},\${CILIUM_IMAGE_REGISTRY},\${IPV6_HP_BPF_VERSION}' < '${cilium_path}/cilium-operator/templates/deployment.yaml' | kubectl apply -f -"
    
    # Deploy Cilium agent with environment substitution
    log "Applying Cilium agent daemonset with templating..."
    execute "envsubst '\${CILIUM_VERSION_TAG},\${CILIUM_IMAGE_REGISTRY},\${IPV6_HP_BPF_VERSION}' < '${cilium_path}/cilium-agent/templates/daemonset.yaml' | kubectl apply -f -"
    
    log "Cilium networking components deployed successfully"
}

# Function to verify deployment
verify_deployment() {
    if [[ "${DRY_RUN}" == "true" ]]; then
        log "Skipping deployment verification in dry-run mode"
        return
    fi
    
    log "Verifying deployment..."
    
    # Wait for Cilium operator to be ready
    log "Waiting for Cilium operator to be ready..."
    execute "kubectl wait --for=condition=available --timeout=300s deployment/cilium-operator -n kube-system"
    
    # Wait for Cilium agents to be ready
    log "Waiting for Cilium agents to be ready..."
    execute "kubectl wait --for=condition=ready --timeout=300s pod -l k8s-app=cilium -n kube-system"
    
    # Show status
    log "Deployment status:"
    execute "kubectl get pods -n kube-system | grep cilium"
    execute "kubectl get pods -n kube-system | grep azure-cns"
    
    log "Deployment verification completed"
}

# Function to show completion message
show_completion_message() {
    log "BYO Cilium cluster setup completed successfully!"
    echo ""
    echo "Cluster details:"
    echo "  Name: ${CLUSTER_NAME}"
    echo "  Subscription: ${SUBSCRIPTION}"
    echo "  CNS Version: ${CNS_VERSION}"
    echo "  Cilium Version: ${CILIUM_VERSION_TAG}"
    echo ""
    echo "To get the kubeconfig for this cluster, run:"
    echo "  ${AZCLI} aks get-credentials --resource-group ${CLUSTER_NAME} --name ${CLUSTER_NAME}"
    echo ""
    echo "To verify the installation:"
    echo "  kubectl get pods -n kube-system"
    echo "  kubectl get nodes"
}

# Parse command line arguments
CLUSTER_NAME="${DEFAULT_CLUSTER_NAME}"
SUBSCRIPTION="${DEFAULT_SUB}"
CNS_VERSION="${DEFAULT_CNS_VERSION}"
AZURE_IPAM_VERSION="${DEFAULT_AZURE_IPAM_VERSION}"
CILIUM_DIR="${DEFAULT_CILIUM_DIR}"
CILIUM_IMAGE_REGISTRY="${DEFAULT_CILIUM_IMAGE_REGISTRY}"
CILIUM_VERSION_TAG="${DEFAULT_CILIUM_VERSION_TAG}"
IPV6_HP_BPF_VERSION="${DEFAULT_IPV6_HP_BPF_VERSION}"
CNS_IMAGE_REPO="${DEFAULT_CNS_IMAGE_REPO}"
AZCLI="${DEFAULT_AZCLI}"
DRY_RUN="false"

while [[ $# -gt 0 ]]; do
    case $1 in
        -c|--cluster)
            CLUSTER_NAME="$2"
            shift 2
            ;;
        -s|--subscription)
            SUBSCRIPTION="$2"
            shift 2
            ;;
        -z|--azcli)
            AZCLI="$2"
            shift 2
            ;;
        --cns-version)
            CNS_VERSION="$2"
            shift 2
            ;;
        --azure-ipam-version)
            AZURE_IPAM_VERSION="$2"
            shift 2
            ;;
        --cilium-dir)
            CILIUM_DIR="$2"
            shift 2
            ;;
        --cilium-registry)
            CILIUM_IMAGE_REGISTRY="$2"
            shift 2
            ;;
        --cilium-version-tag)
            CILIUM_VERSION_TAG="$2"
            shift 2
            ;;
        --ipv6-hp-bpf-version)
            IPV6_HP_BPF_VERSION="$2"
            shift 2
            ;;
        --cns-image-repo)
            CNS_IMAGE_REPO="$2"
            shift 2
            ;;
        --dry-run)
            DRY_RUN="true"
            shift
            ;;
        -h|--help)
            usage
            exit 0
            ;;
        *)
            error "Unknown option: $1"
            usage
            exit 1
            ;;
    esac
done

# Validate required parameters
if [[ -z "${SUBSCRIPTION}" ]]; then
    error "Subscription ID is required. Use --subscription to specify it."
    usage
    exit 1
fi

# Main execution
main() {
    log "Starting BYO Cilium cluster creation..."
    log "Configuration:"
    log "  Cluster Name: ${CLUSTER_NAME}"
    log "  Subscription: ${SUBSCRIPTION}"
    log "  Azure CLI: ${AZCLI}"
    log "  CNS Version: ${CNS_VERSION}"
    log "  Azure IPAM Version: ${AZURE_IPAM_VERSION}"
    log "  Cilium Directory: ${CILIUM_DIR}"
    log "  Cilium Registry: ${CILIUM_IMAGE_REGISTRY}"
    log "  CNS Image Repo: ${CNS_IMAGE_REPO}"
    log "  Dry Run: ${DRY_RUN}"
    
    check_prerequisites
    validate_cilium_dir
    detect_versions
    
    log "Final configuration:"
    log "  Cilium Version Tag: ${CILIUM_VERSION_TAG}"
    log "  IPv6 HP BPF Version: ${IPV6_HP_BPF_VERSION}"
    
    create_cluster
    deploy_cns
    deploy_cilium
    verify_deployment
    show_completion_message
}

# Run main function
main "$@"