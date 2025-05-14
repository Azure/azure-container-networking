#!/bin/bash
set -nex
pwd
ls -la

mkdir -p "$OUT_DIR"/files
mkdir -p "$OUT_DIR"/bins

export GOOS=$OS
export GOARCH=$ARCH
export CGO_ENABLED=0 

CNI_BUILD_DIR="$REPO_ROOT"/cni
STATELESS_CNI_BUILD_DIR="$CNI_BUILD_DIR"/stateless
CNI_MULTITENANCY_BUILD_DIR="$REPO_ROOT"/cni-multitenancy
CNI_MULTITENANCY_TRANSPARENT_VLAN_BUILD_DIR="$REPO_ROOT"/cni-multitenancy-transparent-vlan
CNI_SWIFT_BUILD_DIR="$REPO_ROOT"/cni-swift
CNI_OVERLAY_BUILD_DIR="$REPO_ROOT"/cni-overlay
CNI_BAREMETAL_BUILD_DIR="$REPO_ROOT"/cni-baremetal
CNI_DUALSTACK_BUILD_DIR="$REPO_ROOT"/cni-dualstack

CNI_TEMP_DIR=$(mktemp -d -p "$GEN_DIR")

CNI_NET_DIR="$REPO_ROOT"/cni/network/plugin
pushd "$CNI_NET_DIR"
  go build -v -a -trimpath \
    -o "$OUT_DIR"/bins/azure-vnet \
    -ldflags "-X main.version="$CNI_VERSION"" \
    -gcflags="-dwarflocationlists=true" \
    ./main.go
popd

STATELESS_CNI_NET_DIR="$REPO_ROOT"/cni/network/stateless
pushd "$STATELESS_CNI_NET_DIR"
  go build -v -a -trimpath \
    -o "$OUT_DIR"/bins/azure-vnet-stateless \
    -ldflags "-X main.version="$CNI_VERSION"" \
    -gcflags="-dwarflocationlists=true" \
    ./main.go
popd

CNI_IPAM_DIR="$REPO_ROOT"/cni/ipam/plugin
pushd "$CNI_IPAM_DIR"
  go build -v -a -trimpath \
    -o "$OUT_DIR"/bins/azure-vnet-ipam \
    -ldflags "-X main.version="$CNI_VERSION"" \
    -gcflags="-dwarflocationlists=true" \
    ./main.go
popd

CNI_IPAMV6_DIR="$REPO_ROOT"/cni/ipam/pluginv6
pushd "$CNI_IPAMV6_DIR"
  go build -v -a -trimpath \
    -o "$OUT_DIR"/bins/azure-vnet-ipamv6 
    -ldflags "-X main.version="$CNI_VERSION"" \
    -gcflags="-dwarflocationlists=true" \
    ./main.go
popd

CNI_TELEMETRY_DIR="$REPO_ROOT"/cni/telemetry/service
pushd "$CNI_TELEMETRY_DIR"
  go build -v -a -trimpath \
    -o "$OUT_DIR"/bins/azure-vnet-telemetry \
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
  cp ../telemetry/azure-vnet-telemetry.config "$OUT_DIR"/files/azure-vnet-telemetry.config
  sha256sum * > sum.txt
  #gzip --verbose --best --recursive "$OUT_DIR" && for f in *.gz; do mv -- "$f" "${f%%.gz}"; done
popd


mkdir -p "$CNI_TEMP_DIR"
GOPATH="$CNI_TEMP_DIR" go mod download github.com/azure/azure-container-networking/dropgz@$DROPGZ_VERSION

pushd "$CNI_TEMP_DIR"/pkg/mod/github.com/azure/azure-container-networking/dropgz\@$DROPGZ_VERSION
  cp "$OUT_DIR"/files/* pkg/embed/fs/
  cp "$OUT_DIR"/bins/* pkg/embed/fs/
  go build -a \
    -o "$OUT_DIR"/bins/dropgz \
    -ldflags "-X github.com/Azure/azure-container-networking/dropgz/internal/buildinfo.Version="$VERSION"" \
    -gcflags="-dwarflocationlists=true" \
    ./main.go
popd
