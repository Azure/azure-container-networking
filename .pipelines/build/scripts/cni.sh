#!/bin/bash
set -eux

mkdir -p "$OUT_DIR"/files
mkdir -p "$OUT_DIR"/bin

export GOOS=$OS
export GOARCH=$ARCH
export CGO_ENABLED=0 


CNI_NET_DIR="$REPO_ROOT"/cni/network/plugin
pushd "$CNI_NET_DIR"
  go build -v -a -trimpath \
    -o "$OUT_DIR"/bin/azure-vnet.exe \
    -ldflags "-X main.version="$CNI_VERSION"" \
    -gcflags="-dwarflocationlists=true" \
    ./main.go
popd

STATELESS_CNI_BUILD_DIR="$REPO_ROOT"/cni/network/stateless
pushd "$STATELESS_CNI_BUILD_DIR"
  go build -v -a -trimpath \
    -o "$OUT_DIR"/bin/azure-vnet-stateless.exe \
    -ldflags "-X main.version="$CNI_VERSION"" \
    -gcflags="-dwarflocationlists=true" \
    ./main.go
popd

CNI_IPAM_DIR="$REPO_ROOT"/cni/ipam/plugin
pushd "$CNI_IPAM_DIR"
  go build -v -a -trimpath \
    -o "$OUT_DIR"/bin/azure-vnet-ipam.exe \
    -ldflags "-X main.version="$CNI_VERSION"" \
    -gcflags="-dwarflocationlists=true" \
    ./main.go
popd

CNI_IPAMV6_DIR="$REPO_ROOT"/cni/ipam/pluginv6
pushd "$CNI_IPAMV6_DIR"
  go build -v -a -trimpath \
    -o "$OUT_DIR"/bin/azure-vnet-ipamv6.exe \
    -ldflags "-X main.version="$CNI_VERSION"" \
    -gcflags="-dwarflocationlists=true" \
    ./main.go
popd

CNI_TELEMETRY_DIR="$REPO_ROOT"/cni/telemetry/service
pushd "$CNI_TELEMETRY_DIR"
  go build -v -a -trimpath \
    -o "$OUT_DIR"/bin/azure-vnet-telemetry.exe \
    -ldflags "-X main.version="$CNI_VERSION" -X "$CNI_AI_PATH"="$CNI_AI_ID"" \
    -gcflags="-dwarflocationlists=true" \
   ./telemetrymain.go
popd

pushd "$REPO_ROOT"/cni
  cp azure-$OS.conflist "$OUT_DIR"/files/azure.conflist
  cp azure-$OS-swift.conflist "$OUT_DIR"/files/azure-swift.conflist
  cp azure-linux-multitenancy-transparent-vlan.conflist "$OUT_DIR"/files/azure-multitenancy-transparent-vlan.conflist
  cp azure-$OS-swift-overlay.conflist "$OUT_DIR"/files/azure-swift-overlay.conflist
  cp azure-$OS-swift-overlay-dualstack.conflist "$OUT_DIR"/files/azure-swift-overlay-dualstack.conflist
  cp azure-$OS-multitenancy.conflist "$OUT_DIR"/files/multitenancy.conflist
  cp "$REPO_ROOT"/telemetry/azure-vnet-telemetry.config "$OUT_DIR"/files/azure-vnet-telemetry.config
popd
