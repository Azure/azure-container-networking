#!/usr/bin/env bash
set -euo pipefail

# Triggers the Function App endpoints and waits for Azure DevOps pipeline completion.
#
# Examples:
#   ./test.sh --image myacr.azurecr.io/app:v1.0.0
#   ./test.sh --image=myacr.azurecr.io/app:v1.0.0
#   ./test.sh -i public/oss/moby/moby:latest

API_BASE="https://acn-dalec-e2e-dfazhxbfg4frcwbg.westcentralus-01.azurewebsites.net"
TRIGGER_URL="${API_BASE}/api/trigger"
STATUS_URL="${API_BASE}/api/status"

# Fixed auth scope.
FUNCTION_API_APP_ID_URI="api://d3b254c1-a8e6-44c0-a1ed-f23fb70a432b"

# Fixed polling controls.
TIMEOUT_SECONDS=$((12 * 3600))
POLL_INTERVAL_SECONDS=120

IMAGE_PATH=""
DEBUG=0

usage() {
  cat <<'EOF'
Usage:
  test.sh --image <value>
  test.sh --image=<value>

Required:
  -i, --image, --image-path <value>   imagePath sent to trigger endpoint

Options:
  --debug                              Print request payload and compact responses
  -h, --help                           Show this help
EOF
}

json_escape() {
  # Escapes a raw string value for safe JSON embedding.
  printf '%s' "$1" \
    | sed -e 's/\\/\\\\/g' -e 's/"/\\"/g' -e ':a;N;$!ba;s/\n/\\n/g'
}

build_trigger_payload() {
  if command -v jq >/dev/null 2>&1; then
    jq -nc --arg imagePath "$1" '{imagePath:$imagePath}'
  else
    printf '{"imagePath":"%s"}' "$(json_escape "$1")"
  fi
}

json_get() {
  local key="$1"
  local body="$2"

  if command -v jq >/dev/null 2>&1; then
    printf '%s' "${body}" | jq -r --arg k "${key}" '.[$k] // empty' 2>/dev/null || true
    return 0
  fi

  printf '%s' "${body}" \
    | grep -oE '"'"'"${key}"'"'"[[:space:]]*:[[:space:]]*("[^"]*"|[^,}\r\n]+)' \
    | head -n1 \
    | sed -E 's/^[^:]+:[[:space:]]*"?([^",}\r\n]+)"?$/\1/' || true
}

while [[ $# -gt 0 ]]; do
  case "$1" in
    -i|--image|--image-path)
      IMAGE_PATH="${2:-}"
      shift 2
      ;;
    --image=*|--image-path=*)
      IMAGE_PATH="${1#*=}"
      shift
      ;;
    --debug)
      DEBUG=1
      shift
      ;;
    -h|--help)
      usage
      exit 0
      ;;
    *)
      echo "Unknown argument: $1" >&2
      usage
      exit 2
      ;;
  esac
done

if [[ -z "${IMAGE_PATH}" ]]; then
  echo "--image is required" >&2
  usage
  exit 2
fi

if ! command -v az >/dev/null 2>&1; then
  echo "Azure CLI (az) is required" >&2
  exit 1
fi

az account show >/dev/null 2>&1 || {
  echo "Not logged in to Azure CLI. Run: az login" >&2
  exit 1
}

TOKEN="$(az account get-access-token \
  --scope "${FUNCTION_API_APP_ID_URI}/.default" \
  --query accessToken -o tsv)"

if [[ -z "${TOKEN}" ]]; then
  echo "Failed to acquire bearer token" >&2
  exit 1
fi

AUTH_HEADER=(-H "Authorization: Bearer ${TOKEN}")

echo "Trigger URL: ${TRIGGER_URL}"
echo "Status URL:  ${STATUS_URL}"
echo "Timeout:     ${TIMEOUT_SECONDS}s"
echo "Interval:    ${POLL_INTERVAL_SECONDS}s"

echo "IMAGE_PATH=${IMAGE_PATH}"
TRIGGER_PAYLOAD="$(build_trigger_payload "${IMAGE_PATH}")"
if [[ "${DEBUG}" -eq 1 ]]; then
  echo "TRIGGER_PAYLOAD=${TRIGGER_PAYLOAD}"
fi

TRIGGER_RESPONSE="$(curl -sS -w "\n%{http_code}" \
  --max-time 30 \
  -X POST "${TRIGGER_URL}" \
  "${AUTH_HEADER[@]}" \
  -H "Content-Type: application/json" \
  --data-raw "${TRIGGER_PAYLOAD}")"

TRIGGER_BODY="$(printf '%s' "${TRIGGER_RESPONSE}" | sed '$d')"
TRIGGER_CODE="$(printf '%s' "${TRIGGER_RESPONSE}" | tail -n1)"

echo "TRIGGER_HTTP_CODE=${TRIGGER_CODE}"
if [[ "${DEBUG}" -eq 1 ]]; then
  echo "TRIGGER_RESPONSE=${TRIGGER_BODY}"
fi

if [[ "${TRIGGER_CODE}" -lt 200 || "${TRIGGER_CODE}" -ge 300 ]]; then
  echo "Trigger request failed. response=${TRIGGER_BODY}" >&2
  exit 1
fi

RUN_ID="$(json_get runId "${TRIGGER_BODY}")"
if [[ -z "${RUN_ID}" ]]; then
  RUN_ID="$(json_get id "${TRIGGER_BODY}")"
fi

if [[ -z "${RUN_ID}" ]]; then
  echo "Trigger response missing runId/id. response=${TRIGGER_BODY}" >&2
  exit 1
fi

echo "RUN_ID=${RUN_ID}"

start_epoch="$(date +%s)"
attempt=0

while true; do
  attempt=$((attempt + 1))

  STATUS_RESPONSE="$(curl -sS -w "\n%{http_code}" \
    --max-time 30 \
    -X GET "${STATUS_URL}?runId=${RUN_ID}" \
    "${AUTH_HEADER[@]}")"

  STATUS_BODY="$(printf '%s' "${STATUS_RESPONSE}" | sed '$d')"
  STATUS_CODE="$(printf '%s' "${STATUS_RESPONSE}" | tail -n1)"

  now_epoch="$(date +%s)"
  elapsed="$(( now_epoch - start_epoch ))"

  echo "ATTEMPT=${attempt} ELAPSED_SECONDS=${elapsed} STATUS_HTTP_CODE=${STATUS_CODE}"

  if [[ "${DEBUG}" -eq 1 ]]; then
    echo "STATUS_RESPONSE=${STATUS_BODY}"
  fi

  if [[ "${STATUS_CODE}" -lt 200 || "${STATUS_CODE}" -ge 300 ]]; then
    echo "Status request failed. runId=${RUN_ID} response=${STATUS_BODY}" >&2
    exit 1
  fi

  state="$(json_get state "${STATUS_BODY}")"
  if [[ -z "${state}" ]]; then
    state="$(json_get status "${STATUS_BODY}")"
  fi
  result="$(json_get result "${STATUS_BODY}")"

  echo "RUN_STATE=${state:-unknown} RUN_RESULT=${result:-unknown}"

  state_lc="$(printf '%s' "${state}" | tr '[:upper:]' '[:lower:]')"
  result_lc="$(printf '%s' "${result}" | tr '[:upper:]' '[:lower:]')"

  if [[ "${state_lc}" == "completed" || ( -n "${result_lc}" && "${result_lc}" != "null" ) ]]; then
    case "${result_lc}" in
      succeeded|partiallysucceeded)
        echo "Pipeline completed successfully. runId=${RUN_ID} state=${state} result=${result}"
        exit 0
        ;;
      "")
        echo "Pipeline completed. result is empty. runId=${RUN_ID} state=${state}" >&2
        exit 1
        ;;
      *)
        echo "Pipeline completed with non-success result. runId=${RUN_ID} state=${state} result=${result}" >&2
        exit 1
        ;;
    esac
  fi

  if [[ "${elapsed}" -ge "${TIMEOUT_SECONDS}" ]]; then
    echo "Timed out waiting for pipeline result after ${elapsed}s (limit=${TIMEOUT_SECONDS}s). runId=${RUN_ID}" >&2
    exit 124
  fi

  sleep "${POLL_INTERVAL_SECONDS}"
done
