#!/usr/bin/env bash
set -e

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
    }"
done
