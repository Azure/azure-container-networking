#!/usr/bin/env bash

set -euo pipefail

: "${RETINA_AGENT_IMAGE:?RETINA_AGENT_IMAGE must be set}"
: "${RETINA_INIT_IMAGE:?RETINA_INIT_IMAGE must be set}"
: "${RETINA_OPERATOR_IMAGE:?RETINA_OPERATOR_IMAGE must be set}"

manifest="$(cat)"

replace_image() {
  local placeholder="$1"
  local image="$2"

  if [[ "$manifest" != *"$placeholder"* ]]; then
    echo "Retina chart did not render expected image placeholder: ${placeholder}" >&2
    exit 1
  fi
  manifest="${manifest//"$placeholder"/"$image"}"
}

replace_image "retina-agent.invalid/override:replace" "$RETINA_AGENT_IMAGE"
replace_image "retina-init.invalid/override:replace" "$RETINA_INIT_IMAGE"
replace_image "retina-operator.invalid/override:replace" "$RETINA_OPERATOR_IMAGE"

printf '%s\n' "$manifest"
