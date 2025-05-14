#!/bin/bash
set -nex

DROPGZ_VERSION="${DROPGZ_VERSION:-v0.0.12}"
IPAM_BUILD_DIR=$(mktemp -d -p "$GEN_DIR")

pushd "$ROOT_DIR"/azure-ipam
  GOOS=$OS CGO_ENABLED=0 go build -v -a -o "$IPAM_BUILD_DIR"/azure-ipam -trimpath -ldflags "-X github.com/Azure/azure-container-networking/azure-ipam/internal/buildinfo.Version="$AZURE_IPAM_VERSION" main.version="$VERSION"" -gcflags="-dwarflocationlists=true"
  cp *.conflist "$IPAM_BUILD_DIR"
  sha256sum * > sum.txt
  gzip --verbose --best --recursive . && for f in *.gz; do mv -- "$f" "${f%%.gz}"; done
popd

go mod download github.com/azure/azure-container-networking/dropgz@$DROPGZ_VERSION
pushd "$GOPATH"/pkg/mod/github.com/azure/azure-container-networking/dropgz\@$DROPGZ_VERSION
  cp "$IPAM_BUILD_DIR"/* pkg/embed/fs/
  GOOS=$OS CGO_ENABLED=0 go build -a -o "$OUT_DIR"/bins/dropgz -trimpath -ldflags "-X github.com/Azure/azure-container-networking/dropgz/internal/buildinfo.Version="$VERSION"" -gcflags="-dwarflocationlists=true" main.go
popd
