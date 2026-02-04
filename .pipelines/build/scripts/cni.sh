#!/bin/bash
set -eux

[[ $OS =~ windows ]] && FILE_EXT='.exe' || FILE_EXT=''

mkdir -p "$OUT_DIR"/files
mkdir -p "$OUT_DIR"/bin

export CGO_ENABLED=0


CNI_NET_DIR="$REPO_ROOT"/cni/network/plugin
pushd "$CNI_NET_DIR"
  GOOS="$OS" go build -v -a -trimpath \
    -o "$OUT_DIR"/bin/azure-vnet"$FILE_EXT" \
    -ldflags "-s -w -X main.version="$CNI_VERSION"" \
    -gcflags="-dwarflocationlists=true" \
    ./main.go
popd

STATELESS_CNI_BUILD_DIR="$REPO_ROOT"/cni/network/stateless
pushd "$STATELESS_CNI_BUILD_DIR"
  GOOS="$OS" go build -v -a -trimpath \
    -o "$OUT_DIR"/bin/azure-vnet-stateless"$FILE_EXT" \
    -ldflags "-s -w -X main.version="$CNI_VERSION"" \
    -gcflags="-dwarflocationlists=true" \
    ./main.go
popd

CNI_IPAM_DIR="$REPO_ROOT"/cni/ipam/plugin
pushd "$CNI_IPAM_DIR"
  GOOS="$OS" go build -v -a -trimpath \
    -o "$OUT_DIR"/bin/azure-vnet-ipam"$FILE_EXT" \
    -ldflags "-s -w -X main.version="$CNI_VERSION"" \
    -gcflags="-dwarflocationlists=true" \
    ./main.go
popd

CNI_IPAMV6_DIR="$REPO_ROOT"/cni/ipam/pluginv6
pushd "$CNI_IPAMV6_DIR"
  GOOS="$OS" go build -v -a -trimpath \
    -o "$OUT_DIR"/bin/azure-vnet-ipamv6"$FILE_EXT" \
    -ldflags "-s -w -X main.version="$CNI_VERSION"" \
    -gcflags="-dwarflocationlists=true" \
    ./main.go
popd

CNI_TELEMETRY_DIR="$REPO_ROOT"/cni/telemetry/service
pushd "$CNI_TELEMETRY_DIR"
  GOOS="$OS" go build -v -a -trimpath \
    -o "$OUT_DIR"/bin/azure-vnet-telemetry"$FILE_EXT" \
    -ldflags "-s -w -X main.version="$CNI_VERSION" -X "$CNI_AI_PATH"="$CNI_AI_ID"" \
    -gcflags="-dwarflocationlists=true" \
   ./telemetrymain.go
popd

pushd "$REPO_ROOT"/cni
  cp azure-$OS.conflist "$OUT_DIR"/files/azure.conflist
  cp azure-$OS-swift.conflist "$OUT_DIR"/files/azure-swift.conflist
  cp azure-linux-multitenancy-transparent-vlan.conflist "$OUT_DIR"/files/azure-multitenancy-transparent-vlan.conflist
  cp azure-$OS-swift-overlay.conflist "$OUT_DIR"/files/azure-swift-overlay.conflist
  cp azure-$OS-swift-overlay-dualstack.conflist "$OUT_DIR"/files/azure-swift-overlay-dualstack.conflist
  cp azure-windows-multitenancy.conflist "$OUT_DIR"/files/multitenancy.conflist
  cp "$REPO_ROOT"/telemetry/azure-vnet-telemetry.config "$OUT_DIR"/files/azure-vnet-telemetry.config
popd
