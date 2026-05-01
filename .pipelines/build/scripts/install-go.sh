#!/bin/bash
set -eux

# Install Go by extracting it from the msft-go container image.
# The golang image reference is read directly from the source Dockerfile for the
# current image (identified by $name), keeping the pipeline in sync with the build.
#
# Priority:
#   1. MSFT_GO_IMAGE env var (explicit override)
#   2. Parsed from the source Dockerfile for $name
#   3. Hardcoded fallback image below
#
# Fallback to latest msft-go image.
DEFAULT_IMAGE="mcr.microsoft.com/oss/go/microsoft/golang:latest"

# Resolves the golang image from the source Dockerfile for the given $name.
# Echoes the image reference, or empty string if it cannot be determined.
resolve_go_image() {
  if [[ "${name:-}" == "npm" ]]; then
    # npm uses OS-specific Dockerfiles with a tag-based reference.
    # The image may be field 2 (no --platform) or field 3 (with --platform),
    # so extract the mcr.* token directly.
    # e.g. FROM mcr.../golang:1.25.5 AS builder
    # e.g. FROM --platform=linux/amd64 mcr.../golang:1.25.5 AS builder
    local buildfile="${REPO_ROOT}/npm/${OS:-linux}.Dockerfile"
    grep -m1 '^FROM.*golang' "${buildfile}" | grep -o 'mcr[^ ]*'

  else
    # All other images use a digest-pinned reference and always have --platform,
    # making the image consistently field 3: FROM --platform=X IMAGE AS alias
    local buildfile
    if [[ "${name:-}" == "ipv6-hp-bpf" ]]; then
      buildfile="${REPO_ROOT}/bpf-prog/ipv6-hp-bpf/linux.Dockerfile"
    elif [[ -n "${name:-}" ]]; then
      buildfile="${REPO_ROOT}/${name}/Dockerfile"
    fi

    if [[ -n "${buildfile:-}" && -f "${buildfile}" ]]; then
      grep -m1 '^FROM.*golang' "${buildfile}" | awk '{print $3}'
    fi
  fi
}

if [[ -z "${MSFT_GO_IMAGE:-}" ]]; then
  MSFT_GO_IMAGE="$(resolve_go_image)"
  MSFT_GO_IMAGE="${MSFT_GO_IMAGE:-$DEFAULT_IMAGE}"
fi

ARCH="${ARCH:-amd64}"

# Extract /usr/local/go from the image without needing a Docker daemon.
# crane export streams the full image filesystem; we extract just usr/local/go.
crane export --platform "linux/${ARCH}" "$MSFT_GO_IMAGE" - | sudo tar -xf - -C / usr/local/go

echo "##vso[task.prependpath]/usr/local/go/bin"
