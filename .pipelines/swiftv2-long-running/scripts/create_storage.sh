#!/usr/bin/env bash
set -e
trap 'echo "[ERROR] Failed during Storage Account creation." >&2' ERR

SUBSCRIPTION_ID=$1
LOCATION=$2
RG=$3

RAND=$(openssl rand -hex 4)
SA1="sa1${RAND}"
SA2="sa2${RAND}"

az account set --subscription "$SUBSCRIPTION_ID"
for SA in "$SA1" "$SA2"; do
  echo "Creating storage account $SA"
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
  
  if az storage account show --name "$SA" --resource-group "$RG" &>/dev/null; then
    echo "[OK] Storage account $SA verified successfully."
  else
    echo "[ERROR] Storage account $SA not found after creation!" >&2
    exit 1
  fi
done

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
bash "$SCRIPT_DIR/manage_storage_rbac.sh" assign "$SUBSCRIPTION_ID" "$RG" "$SA1 $SA2"

for SA in "$SA1" "$SA2"; do
  echo "Creating test container in $SA"
  az storage container create \
    --name "test" \
    --account-name "$SA" \
    --auth-mode login \
    && echo "[OK] Container 'test' created in $SA"
  
  echo "Uploading test blob to $SA"
  az storage blob upload \
    --account-name "$SA" \
    --container-name "test" \
    --name "hello.txt" \
    --data "Hello from Private Endpoint - Storage: $SA" \
    --auth-mode login \
    --overwrite \
    && echo "[OK] Test blob 'hello.txt' uploaded to $SA/test/"
done

echo "Removing RBAC role after blob upload"
bash "$SCRIPT_DIR/manage_storage_rbac.sh" delete "$SUBSCRIPTION_ID" "$RG" "$SA1 $SA2"
echo "All storage accounts created and verified successfully."

set +x
echo "##vso[task.setvariable variable=StorageAccount1;isOutput=true]$SA1"
echo "##vso[task.setvariable variable=StorageAccount2;isOutput=true]$SA2"
set -x
