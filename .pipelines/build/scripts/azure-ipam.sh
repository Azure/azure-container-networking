#!/bin/bash
set -eux

[[ $OS =~ windows ]] && FILE_EXT='.exe' || FILE_EXT=''

export CGO_ENABLED=0 

mkdir -p "$OUT_DIR"/bin
mkdir -p "$OUT_DIR"/files

pushd "$REPO_ROOT"/azure-ipam
  GOOS="$OS" go build -v -a -trimpath \
    -o "$OUT_DIR"/bin/azure-ipam"$FILE_EXT" \
    -ldflags "-X github.com/Azure/azure-container-networking/azure-ipam/internal/buildinfo.Version="$AZURE_IPAM_VERSION" -X main.version="$AZURE_IPAM_VERSION"" \
    -gcflags="-dwarflocationlists=true" \
    .

  cp *.conflist "$OUT_DIR"/files/
popd
