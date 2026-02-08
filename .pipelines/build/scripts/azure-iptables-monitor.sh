#!/bin/bash
set -eux

[[ $OS =~ windows ]] && { echo "azure-iptables-monitor is not supported on Windows"; exit 1; }
FILE_EXT=''

export CGO_ENABLED=0
export C_INCLUDE_PATH=/usr/include/bpf

mkdir -p "$OUT_DIR"/bin
mkdir -p "$OUT_DIR"/files

pushd "$REPO_ROOT"/azure-iptables-monitor
  GOOS="$OS" go build -v -a -trimpath \
    -o "$OUT_DIR"/bin/azure-iptables-monitor"$FILE_EXT" \
    -ldflags "-s -w -X github.com/Azure/azure-container-networking/azure-iptables-monitor/internal/buildinfo.Version=$AZURE_IPTABLES_MONITOR_VERSION -X main.version=$AZURE_IPTABLES_MONITOR_VERSION" \
    -gcflags="-dwarflocationlists=true" \
    .
popd

echo "Building azure-block-iptables binary..."

# Debian/Ubuntu
if [[ -f /etc/debian_version ]]; then

  apt-get update -y
  apt-get install -y --no-install-recommends llvm clang linux-libc-dev libbpf-dev libc6-dev nftables iproute2
  
  if [[ $ARCH =~ amd64 ]]; then
    apt-get install -y --no-install-recommends gcc-multilib
    ARCH_GNU=x86_64-linux-gnu
  elif [[ $ARCH =~ arm64 ]]; then
    apt-get install -y --no-install-recommends gcc-aarch64-linux-gnu
    ARCH_GNU=aarch64-linux-gnu
  fi

  # Create symlinks for architecture-specific includes
  for dir in /usr/include/"$ARCH_GNU"/*; do
    if [[ -d "$dir" || -f "$dir" ]]; then
      ln -sfn "$dir" /usr/include/$(basename "$dir")
    fi
  done

# Mariner
else
  tdnf install -y llvm clang libbpf-devel nftables gcc binutils iproute glibc
  
  if [[ $ARCH =~ amd64 ]]; then
    ARCH_GNU=x86_64-linux-gnu
  elif [[ $ARCH =~ arm64 ]]; then
    ARCH_GNU=aarch64-linux-gnu
  fi

  # Create symlinks for architecture-specific includes
  for dir in /usr/include/"$ARCH_GNU"/*; do
    if [[ -d "$dir" || -f "$dir" ]]; then
      ln -sfn "$dir" /usr/include/$(basename "$dir")
    fi
  done
fi

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
