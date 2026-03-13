#!/usr/bin/env bash
# teardown.sh — Delete the e2e test resource group and all contained resources.
set -euo pipefail

RESOURCE_GROUP="${RESOURCE_GROUP:-aksmigrate-e2e-test}"

RED='\033[0;31m'
GREEN='\033[0;32m'
CYAN='\033[0;36m'
NC='\033[0m'

echo -e "${CYAN}[INFO]${NC}  Deleting resource group '${RESOURCE_GROUP}'..."

if az group exists --name "${RESOURCE_GROUP}" | grep -q true; then
    az group delete --name "${RESOURCE_GROUP}" --yes --no-wait
    echo -e "${GREEN}[OK]${NC}    Deletion initiated (--no-wait). Resource group '${RESOURCE_GROUP}' will be deleted in the background."
else
    echo -e "${GREEN}[OK]${NC}    Resource group '${RESOURCE_GROUP}' does not exist. Nothing to do."
fi

# Clean up local output directory
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
OUTPUT_DIR="${SCRIPT_DIR}/output"
if [[ -d "${OUTPUT_DIR}" ]]; then
    echo -e "${CYAN}[INFO]${NC}  Cleaning up local output directory: ${OUTPUT_DIR}"
    rm -rf "${OUTPUT_DIR}"
    echo -e "${GREEN}[OK]${NC}    Local output cleaned."
fi
