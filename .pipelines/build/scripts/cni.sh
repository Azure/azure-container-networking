#!/bin/bash
set -nex
pwd
ls -la

mkdir -p "$OUT_DIR"/files
mkdir -p "$OUT_DIR"/bins

#CNI_BUILD_DIR=$(mktemp -d -p "$GEN_DIR")

pushd "$REPO_ROOT"/cni
  GOOS=$OS CGO_ENABLED=0 go build -a -o "$OUT_DIR"/bins/azure-vnet -trimpath -ldflags "-X main.version="$VERSION"" -gcflags="-dwarflocationlists=true" network/plugin/main.go
  GOOS=$OS CGO_ENABLED=0 go build -a -o "$OUT_DIR"/bins/azure-vnet-telemetry -trimpath -ldflags "-X main.version="$VERSION" -X "$CNI_AI_PATH"="$CNI_AI_ID"" -gcflags="-dwarflocationlists=true" ../telemetry/service/telemetrymain.go
  GOOS=$OS CGO_ENABLED=0 go build -a -o "$OUT_DIR"/bins/azure-vnet-ipam -trimpath -ldflags "-X main.version="$VERSION"" -gcflags="-dwarflocationlists=true" ipam/plugin/main.go
  GOOS=$OS CGO_ENABLED=0 go build -a -o "$OUT_DIR"/bins/azure-vnet-stateless -trimpath -ldflags "-X main.version="$VERSION"" -gcflags="-dwarflocationlists=true" network/stateless/main.go

  cp azure-$OS.conflist "$OUT_DIR"/files/azure.conflist
  cp azure-$OS-swift.conflist "$OUT_DIR"/files/azure-swift.conflist
  cp azure-linux-multitenancy-transparent-vlan.conflist "$OUT_DIR"/files/azure-multitenancy-transparent-vlan.conflist
  cp azure-$OS-swift-overlay.conflist "$OUT_DIR"/files/azure-swift-overlay.conflist
  cp azure-$OS-swift-overlay-dualstack.conflist "$OUT_DIR"/files/azure-swift-overlay-dualstack.conflist
  cp azure-$OS-multitenancy.conflist "$OUT_DIR"/files/multitenancy.conflist
  cp ../telemetry/azure-vnet-telemetry.config "$OUT_DIR"/files/azure-vnet-telemetry.config
  sha256sum * > sum.txt
  gzip --verbose --best --recursive "$OUT_DIR" && for f in *.gz; do mv -- "$f" "${f%%.gz}"; done
popd

go mod download github.com/azure/azure-container-networking/dropgz@$DROPGZ_VERSION
pushd "$GOPATH"/pkg/mod/github.com/azure/azure-container-networking/dropgz\@$DROPGZ_VERSION
  cp "$OUT_DIR"/files/* pkg/embed/fs/
  cp "$OUT_DIR"/bins/* pkg/embed/fs/
  GOOS=$OS CGO_ENABLED=0 go build -a -o "$OUT_DIR"/bins/dropgz -trimpath -ldflags "-X github.com/Azure/azure-container-networking/dropgz/internal/buildinfo.Version="$VERSION"" -gcflags="-dwarflocationlists=true" main.go
popd
