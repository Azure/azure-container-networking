#!/usr/bin/env bash
set -e
trap 'echo "[ERROR] Failed during Storage Account creation." >&2' ERR

SUBSCRIPTION_ID=$1
LOCATION=$2
RG=$3

RAND=$(openssl rand -hex 4)
SA1="sa1${RAND}"
SA2="sa2${RAND}"
API_VER="2025-06-01"

# Create storage accounts
for SA in "$SA1" "$SA2"; do
  echo "==> Creating storage account $SA"
  az rest --method put \
    --url "https://management.azure.com/subscriptions/$SUBSCRIPTION_ID/resourceGroups/$RG/providers/Microsoft.Storage/storageAccounts/$SA?api-version=$API_VER" \
    --body "{
      \"location\": \"$LOCATION\",
      \"sku\": { \"name\": \"Standard_LRS\" },
      \"kind\": \"StorageV2\",
      \"properties\": {
        \"minimumTlsVersion\": \"TLS1_2\",
        \"allowBlobPublicAccess\": false,
        \"allowSharedKeyAccess\": false
      }
    }" \
  && echo "Storage account $SA created successfully."
done

echo "All storage accounts created successfully."
set +x
	echo "##vso[task.setvariable variable=StorageAccount1;isOutput=true]$SA1"
	echo "##vso[task.setvariable variable=StorageAccount2;isOutput=true]$SA2"
set -x
