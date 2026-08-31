#!/usr/bin/env bash

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
. "${SCRIPT_DIR}/parse_override_image.sh"

assert_equal() {
  local expected="$1"
  local actual="$2"

  if [[ "$actual" != "$expected" ]]; then
    echo "expected '${expected}', got '${actual}'" >&2
    exit 1
  fi
}

assert_equal \
  "ACN azure-cni v1.2.3" \
  "$(parse_override_image "__use-default__" "azure-cni" "v1.2.3")"
assert_equal \
  "MCR azure-cni v1.2.3@sha256:abc" \
  "$(parse_override_image "mcr.microsoft.com/containernetworking/azure-cni:v1.2.3@sha256:abc" "unused" "unused")"
assert_equal \
  "ACN custom/path @sha256:abc" \
  "$(parse_override_image "acnpublic.azurecr.io/custom/path@sha256:abc" "unused" "unused")"

assert_equal \
  "acnpublic.azurecr.io/azure-cni:v1.2.3" \
  "$(format_override_image "ACN" "azure-cni" "v1.2.3")"
assert_equal \
  "mcr.microsoft.com/containernetworking/azure-cni@sha256:abc" \
  "$(format_override_image "MCR" "azure-cni" "@sha256:abc")"
assert_equal \
  "registry.example.com/custom/path:v1.2.3@sha256:abc" \
  "$(format_image "registry.example.com" "custom/path" "v1.2.3@sha256:abc")"
assert_equal \
  "registry.example.com/custom/path@sha256:abc" \
  "$(format_image "registry.example.com" "custom/path" "@sha256:abc")"
