#!/bin/bash
set -eux

[[ $OS =~ windows ]] && { echo "azure-iptables-monitor is not supported on Windows"; exit 1; }
FILE_EXT=''

export CGO_ENABLED=0

mkdir -p "$OUT_DIR"/bin
mkdir -p "$OUT_DIR"/files

pushd "$REPO_ROOT"/azure-iptables-monitor
  GOOS="$OS" go build -v -a -trimpath \
    -o "$OUT_DIR"/bin/azure-iptables-monitor"$FILE_EXT" \
    -ldflags "-s -w -X github.com/Azure/azure-container-networking/azure-iptables-monitor/internal/buildinfo.Version=$AZURE_IPTABLES_MONITOR_VERSION -X main.version=$AZURE_IPTABLES_MONITOR_VERSION" \
    -gcflags="-dwarflocationlists=true" \
    .
popd
