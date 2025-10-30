#!/usr/bin/env bash
set -e
trap 'echo "[ERROR] Failed during Storage Account creation." >&2' ERR

SUBSCRIPTION_ID=$1
LOCATION=$2
RG=$3

RAND=$(openssl rand -hex 4)
SA1="sa1${RAND}"
SA2="sa2${RAND}"

# Set subscription context
az account set --subscription "$SUBSCRIPTION_ID"

# Create storage accounts
for SA in "$SA1" "$SA2"; do
  echo "==> Creating storage account $SA"
  az storage account create \
    --name "$SA" \
    --resource-group "$RG" \
    --location "$LOCATION" \
    --sku Standard_LRS \
    --kind StorageV2 \
    --allow-blob-public-access false \
    --allow-shared-key-access false \
    --https-only true \
    --min-tls-version TLS1_2 \
    --query "name" -o tsv \
  && echo "Storage account $SA created successfully."
  # Verify creation success
  echo "==> Verifying storage account $SA exists..."
  if az storage account show --name "$SA" --resource-group "$RG" &>/dev/null; then
    echo "[OK] Storage account $SA verified successfully."
  else
    echo "[ERROR] Storage account $SA not found after creation!" >&2
    exit 1
  fi
done

echo "All storage accounts created and verified successfully."

# Set pipeline output variables
set +x
echo "##vso[task.setvariable variable=StorageAccount1;isOutput=true]$SA1"
echo "##vso[task.setvariable variable=StorageAccount2;isOutput=true]$SA2"
set -x
