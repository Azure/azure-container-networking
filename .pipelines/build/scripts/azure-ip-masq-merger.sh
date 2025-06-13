#!/bin/bash
set -eux

[[ $OS =~ windows ]] && { echo "azure-ip-masq-merger is not supported on Windows"; exit 1; }
FILE_EXT=''

export CGO_ENABLED=0 

mkdir -p "$OUT_DIR"/bin
mkdir -p "$OUT_DIR"/files

pushd "$REPO_ROOT"/azure-ip-masq-merger
  GOOS="$OS" go build -v -a -trimpath \
    -o "$OUT_DIR"/bin/azure-ip-masq-merger"$FILE_EXT" \
    -ldflags "-X github.com/Azure/azure-container-networking/azure-ip-masq-merger/internal/buildinfo.Version=$AZURE_IP_MASQ_MERGER_VERSION -X main.version=$AZURE_IP_MASQ_MERGER_VERSION" \
    -gcflags="-dwarflocationlists=true" \
    .
popd
