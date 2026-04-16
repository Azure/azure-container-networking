#!/usr/bin/env bash
set -e
trap 'echo "[ERROR] Failed during Storage Account creation." >&2' ERR

SUBSCRIPTION_ID=$1
LOCATION=$2
RG=$3

az account set --subscription "$SUBSCRIPTION_ID"

TAG_KEY="swiftv2-longrun-role"

# Discover storage account by the unique pipeline tag
discover_storage_account() {
  local role="$1"
  local matches
  matches=$(az storage account list -g "$RG" --query "[?tags.\"${TAG_KEY}\" == '${role}'].name | sort(@)" -o tsv 2>/dev/null || true)
  if [[ -z "$matches" ]]; then
    return 0
  fi
  local count
  count=$(printf '%s\n' "$matches" | sed '/^$/d' | wc -l | tr -d ' ')
  if [[ "$count" -gt 1 ]]; then
    echo "[ERROR] Multiple storage accounts found with tag ${TAG_KEY}=${role} in resource group '$RG':" >&2
    printf '%s\n' "$matches" >&2
    exit 1
  fi
  printf '%s\n' "$matches"
}

# Discover existing storage accounts or create new ones
SA1=$(discover_storage_account "sa1")
SA2=$(discover_storage_account "sa2")

if [[ -n "$SA1" && -n "$SA2" ]]; then
  echo "Found existing storage accounts: $SA1, $SA2. Reusing."
else
  RAND=$(openssl rand -hex 4)
  SA1="${SA1:-sa1${RAND}}"
  SA2="${SA2:-sa2${RAND}}"
  ROLES=("sa1" "sa2")
  IDX=0
  for SA in "$SA1" "$SA2"; do
    ROLE="${ROLES[$IDX]}"
    echo "Creating storage account $SA with tag ${TAG_KEY}=${ROLE}"
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
      --tags "${TAG_KEY}=${ROLE}" \
      --query "name" -o tsv \
    && echo "Storage account $SA created successfully."

    if az storage account show --name "$SA" --resource-group "$RG" &>/dev/null; then
      echo "[OK] Storage account $SA verified successfully."
    else
      echo "[ERROR] Storage account $SA not found after creation!" >&2
      exit 1
    fi
    IDX=$((IDX + 1))
  done
fi

# Storage Blob Data Contributor is pre-assigned at the subscription level.
# See LONGRUNNING-TESTS.md "Prerequisites" for required RBAC roles.

for SA in "$SA1" "$SA2"; do
  echo "Creating test container in $SA"
  az storage container create \
    --name "test" \
    --account-name "$SA" \
    --auth-mode login \
    && echo "[OK] Container 'test' created in $SA"
  
  echo "Uploading test blob to $SA"
  
  # Retry blob upload with exponential backoff if RBAC hasn't propagated yet
  MAX_RETRIES=5
  SLEEP_TIME=10
  
  for i in $(seq 1 $MAX_RETRIES); do
    if az storage blob upload \
      --account-name "$SA" \
      --container-name "test" \
      --name "hello.txt" \
      --data "Hello from Private Endpoint - Storage: $SA" \
      --auth-mode login \
      --overwrite 2>&1; then
      echo "[OK] Test blob 'hello.txt' uploaded to $SA/test/"
      break
    else
      if [ $i -lt $MAX_RETRIES ]; then
        echo "[WARN] Blob upload failed (attempt $i/$MAX_RETRIES). Waiting ${SLEEP_TIME}s for RBAC propagation..."
        sleep $SLEEP_TIME
        SLEEP_TIME=$((SLEEP_TIME * 2))
      else
        echo "[ERROR] Failed to upload blob after $MAX_RETRIES attempts"
        exit 1
      fi
    fi
  done
done

# Export storage account names as Azure DevOps output variables so downstream
# stages can consume them without rediscovering.
echo "##vso[task.setvariable variable=storageAccount1;isOutput=true]$SA1"
echo "##vso[task.setvariable variable=storageAccount2;isOutput=true]$SA2"

echo "All storage accounts created and verified successfully."
echo "  storageAccount1=$SA1"
echo "  storageAccount2=$SA2"
