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
  
  # Assign RBAC role to pipeline service principal for blob access
  echo "==> Assigning Storage Blob Data Contributor role to service principal"
  SP_OBJECT_ID=$(az ad signed-in-user show --query id -o tsv 2>/dev/null || az account show --query user.name -o tsv)
  SA_SCOPE="/subscriptions/${SUBSCRIPTION_ID}/resourceGroups/${RG}/providers/Microsoft.Storage/storageAccounts/${SA}"
  
  az role assignment create \
    --assignee "$SP_OBJECT_ID" \
    --role "Storage Blob Data Contributor" \
    --scope "$SA_SCOPE" \
    --output none \
    && echo "[OK] RBAC role assigned to service principal for $SA"
  
  # Create container and upload test blob for private endpoint testing
  echo "==> Creating test container in $SA"
  az storage container create \
    --name "test" \
    --account-name "$SA" \
    --auth-mode login \
    && echo "[OK] Container 'test' created in $SA"
  
  # Upload test blob
  echo "==> Uploading test blob to $SA"
  az storage blob upload \
    --account-name "$SA" \
    --container-name "test" \
    --name "hello.txt" \
    --data "Hello from Private Endpoint - Storage: $SA" \
    --auth-mode login \
    --overwrite \
    && echo "[OK] Test blob 'hello.txt' uploaded to $SA/test/"
done

echo "All storage accounts created and verified successfully."

# Set pipeline output variables
set +x
echo "##vso[task.setvariable variable=StorageAccount1;isOutput=true]$SA1"
echo "##vso[task.setvariable variable=StorageAccount2;isOutput=true]$SA2"
set -x
