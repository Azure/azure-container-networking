#!/usr/bin/env bash

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
. "${SCRIPT_DIR}/parse_override_image.sh"

assert_equal() {
  local expected="$1"
  local actual="$2"
  local description="$3"

  if [ "$actual" != "$expected" ]; then
    echo "${description}: expected '${expected}', got '${actual}'" >&2
    exit 1
  fi
}

assert_equal "" "$(resolve_optional_override_image "__use-default__")" "default optional override"
assert_equal "" "$(resolve_optional_override_image "")" "empty optional override"
assert_equal \
  "mcr.microsoft.com/containernetworking/v2/retina-agent:v1.2.9" \
  "$(resolve_optional_override_image "mcr.microsoft.com/containernetworking/v2/retina-agent:v1.2.9")" \
  "tag override"
assert_equal \
  "mcr.microsoft.com/containernetworking/v2/retina-agent@sha256:abc123" \
  "$(resolve_optional_override_image "mcr.microsoft.com/containernetworking/v2/retina-agent@sha256:abc123")" \
  "digest override"
assert_equal \
  "mcr.microsoft.com/containernetworking/v2/retina-agent:v1.2.9@sha256:abc123" \
  "$(resolve_optional_override_image "mcr.microsoft.com/containernetworking/v2/retina-agent:v1.2.9@sha256:abc123")" \
  "tag and digest override"

export RETINA_AGENT_IMAGE="registry.example/retina-agent:v1.2.9"
export RETINA_INIT_IMAGE="registry.example/retina-init@sha256:abc123"
export RETINA_OPERATOR_IMAGE="registry.example/retina-operator:v1.2.9@sha256:def456"

rendered="$(
  printf '%s\n' \
    'image: retina-agent.invalid/override:replace' \
    'initImage: retina-init.invalid/override:replace' \
    'operatorImage: retina-operator.invalid/override:replace' |
    "${SCRIPT_DIR}/../../.pipelines/networkobservability/retina-image-post-renderer.sh"
)"

for expected in "$RETINA_AGENT_IMAGE" "$RETINA_INIT_IMAGE" "$RETINA_OPERATOR_IMAGE"; do
  if [[ "$rendered" != *"$expected"* ]]; then
    echo "post-renderer output did not contain '${expected}'" >&2
    exit 1
  fi
done

echo "parse_override_image tests passed"
