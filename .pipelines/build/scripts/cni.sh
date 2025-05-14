#!/bin/bash

#ARG CNI_AI_PATH
#ARG CNI_AI_ID
# WORKDIR /azure-container-networking

CNI_BUILD_DIR=$(mktemp -d -p "$GEN_DIR")
pushd "$REPO_ROOT"
  GOOS=$OS CGO_ENABLED=0 go build -a -o "$CNI_BUILD_DIR"/azure-vnet -trimpath -ldflags "-X main.version="$VERSION"" -gcflags="-dwarflocationlists=true" cni/network/plugin/main.go
  GOOS=$OS CGO_ENABLED=0 go build -a -o "$CNI_BUILD_DIR"/azure-vnet-telemetry -trimpath -ldflags "-X main.version="$VERSION" -X "$CNI_AI_PATH"="$CNI_AI_ID"" -gcflags="-dwarflocationlists=true" cni/telemetry/service/telemetrymain.go
  GOOS=$OS CGO_ENABLED=0 go build -a -o "$CNI_BUILD_DIR"/azure-vnet-ipam -trimpath -ldflags "-X main.version="$VERSION"" -gcflags="-dwarflocationlists=true" cni/ipam/plugin/main.go
  GOOS=$OS CGO_ENABLED=0 go build -a -o "$CNI_BUILD_DIR"/azure-vnet-stateless -trimpath -ldflags "-X main.version="$VERSION"" -gcflags="-dwarflocationlists=true" cni/network/stateless/main.go

  cp cni/azure-$OS.conflist "$CNI_BUILD_DIR"/azure.conflist
  cp cni/azure-$OS-swift.conflist "$CNI_BUILD_DIR"/azure-swift.conflist
  cp cni/azure-linux-multitenancy-transparent-vlan.conflist "$CNI_BUILD_DIR"/azure-multitenancy-transparent-vlan.conflist
  cp cni/azure-$OS-swift-overlay.conflist "$CNI_BUILD_DIR"/azure-swift-overlay.conflist
  cp cni/azure-$OS-swift-overlay-dualstack.conflist "$CNI_BUILD_DIR"/azure-swift-overlay-dualstack.conflist
  cp cni/azure-$OS-multitenancy.conflist "$CNI_BUILD_DIR"/azure-multitenancy.conflist
  cp telemetry/azure-vnet-telemetry.config "$CNI_BUILD_DIR"/azure-vnet-telemetry.config
  sha256sum * > sum.txt
  gzip --verbose --best --recursive "$CNI_BUILD_DIR" && for f in *.gz; do mv -- "$f" "${f%%.gz}"; done
popd

go mod download github.com/azure/azure-container-networking/dropgz@$DROPGZ_VERSION
pushd "$GOPATH"/pkg/mod/github.com/azure/azure-container-networking/dropgz\@$DROPGZ_VERSION
  cp "$CNI_BUILD_DIR"/* pkg/embed/fs/
  GOOS=$OS CGO_ENABLED=0 go build -a -o "$OUT_DIR"/bins/dropgz -trimpath -ldflags "-X github.com/Azure/azure-container-networking/dropgz/internal/buildinfo.Version="$VERSION"" -gcflags="-dwarflocationlists=true" main.go
popd
