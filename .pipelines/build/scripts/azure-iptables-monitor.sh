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

# Build azure-block-iptables binary
echo "Building azure-block-iptables binary..."

# Install BPF development dependencies for Ubuntu build agent
if [ "$ARCH" = "amd64" ]; then
    sudo apt-get update && sudo apt-get install -y llvm clang linux-libc-dev linux-headers-generic libbpf-dev libc6-dev nftables iproute2 gcc-multilib
    for dir in /usr/include/x86_64-linux-gnu/*; do sudo ln -sfn "$dir" /usr/include/$(basename "$dir")" || echo "Warning: Failed to create symlink for $dir" >&2; done
elif [ "$ARCH" = "arm64" ]; then
    sudo apt-get update && sudo apt-get install -y llvm clang linux-libc-dev linux-headers-generic libbpf-dev libc6-dev nftables iproute2 gcc-aarch64-linux-gnu
    for dir in /usr/include/aarch64-linux-gnu/*; do sudo ln -sfn "$dir" /usr/include/$(basename "$dir")" || echo "Warning: Failed to create symlink for $dir" >&2; done
else
    echo "Warning: Unknown architecture $ARCH, skipping BPF dependency installation"
fi

# Set up C include path for BPF
export C_INCLUDE_PATH=/usr/include/bpf

pushd "$REPO_ROOT"
  # Generate BPF objects
  GOOS="$OS" CGO_ENABLED=0 go generate ./bpf-prog/azure-block-iptables/...
  
  # Build the binary
  GOOS="$OS" CGO_ENABLED=0 go build -a \
    -o "$OUT_DIR"/bin/azure-block-iptables"$FILE_EXT" \
    -trimpath \
    -ldflags "-s -w -X main.version=$AZURE_BLOCK_IPTABLES_VERSION" \
    -gcflags="-dwarflocationlists=true" \
    ./bpf-prog/azure-block-iptables/cmd/azure-block-iptables
popd
