#!/bin/bash
set -eux

[[ $OS =~ windows ]] && { echo "cilium-log-collector is not supported on Windows"; exit 1; }
FILE_EXT=''
# enable cgo for -buildmode=c-shared
export CGO_ENABLED=1

mkdir -p "$OUT_DIR"/bin
mkdir -p "$OUT_DIR"/files

echo "Building cilium-log-collector version: $CILIUM_LOG_COLLECTOR_VERSION"

pushd "$REPO_ROOT"/cilium-log-collector
  go build -buildmode=c-shared -a \
    -o "$OUT_DIR"/bin/out_azure_app_insights.so \
    -trimpath \
    -ldflags "-X main.version=$CILIUM_LOG_COLLECTOR_VERSION" \
    -gcflags="-dwarflocationlists=true" \
    .
popd
