#!/usr/bin/env bash
set -e

ACTION=$1           # "assign" or "delete"
SUBSCRIPTION_ID=$2
RG=$3
STORAGE_ACCOUNTS=$4 # Space-separated list of storage account names

if [ "$ACTION" != "assign" ] && [ "$ACTION" != "delete" ]; then
  echo "[ERROR] Invalid action. Use 'assign' or 'delete'" >&2
  exit 1
fi

az account set --subscription "$SUBSCRIPTION_ID"
SP_OBJECT_ID=$(az ad signed-in-user show --query id -o tsv 2>/dev/null || az account show --query user.name -o tsv)

if [ "$ACTION" == "assign" ]; then
  echo "Assigning Storage Blob Data Contributor role to service principal"
  for SA in $STORAGE_ACCOUNTS; do
    echo "Processing storage account: $SA"
    SA_SCOPE="/subscriptions/${SUBSCRIPTION_ID}/resourceGroups/${RG}/providers/Microsoft.Storage/storageAccounts/${SA}"
    
    EXISTING=$(az role assignment list \
      --assignee "$SP_OBJECT_ID" \
      --role "Storage Blob Data Contributor" \
      --scope "$SA_SCOPE" \
      --query "[].id" -o tsv)
    
    if [ -n "$EXISTING" ]; then
      echo "[OK] Role assignment already exists for $SA"
      continue
    fi
    
    az role assignment create \
      --assignee "$SP_OBJECT_ID" \
      --role "Storage Blob Data Contributor" \
      --scope "$SA_SCOPE" \
      --output none \
      && echo "[OK] Role assigned to service principal for $SA"
  done
  
elif [ "$ACTION" == "delete" ]; then
  echo "Removing Storage Blob Data Contributor role from service principal"
  
  for SA in $STORAGE_ACCOUNTS; do
    echo "Processing storage account: $SA"
    SA_SCOPE="/subscriptions/${SUBSCRIPTION_ID}/resourceGroups/${RG}/providers/Microsoft.Storage/storageAccounts/${SA}"
    
    ASSIGNMENT_ID=$(az role assignment list \
      --assignee "$SP_OBJECT_ID" \
      --role "Storage Blob Data Contributor" \
      --scope "$SA_SCOPE" \
      --query "[0].id" -o tsv 2>/dev/null || echo "")
    
    if [ -z "$ASSIGNMENT_ID" ]; then
      echo "[OK] No role assignment found for $SA (already deleted or never existed)"
      continue
    fi
    
    az role assignment delete --ids "$ASSIGNMENT_ID" --output none \
      && echo "[OK] Role removed from service principal for $SA" \
      || echo "[WARNING] Failed to remove role for $SA (may not exist)"
  done
  
  echo "==> Performing sanity check to verify RBAC cleanup..."
  
  for SA in $STORAGE_ACCOUNTS; do
    SA_SCOPE="/subscriptions/${SUBSCRIPTION_ID}/resourceGroups/${RG}/providers/Microsoft.Storage/storageAccounts/${SA}"
    
    REMAINING=$(az role assignment list \
      --assignee "$SP_OBJECT_ID" \
      --role "Storage Blob Data Contributor" \
      --scope "$SA_SCOPE" \
      --query "[].id" -o tsv 2>/dev/null || echo "")
    
    if [ -n "$REMAINING" ]; then
      echo "[ERROR] RBAC leak detected: Role assignment still exists for $SA after deletion!" >&2
      echo "Assignment ID(s): $REMAINING" >&2
    fi
  done
fi

echo "RBAC management completed successfully."
