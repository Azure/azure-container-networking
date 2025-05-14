#!/bin/bash
set -nex
pwd
ls -la

mkdir -p "$OUT_DIR"/bins
mkdir -p "$OUT_DIR"/lib

# Package up Needed C Files
if [[ -f /etc/debian_version ]];then
  apt-get update -y
  if [[ $GOARCH =~ amd64 ]]; then
    apt-get install -y llvm clang linux-libc-dev linux-headers-generic libbpf-dev libc6-dev nftables iproute2
    #apt-get install -y llvm clang linux-libc-dev linux-headers-generic libbpf-dev libc6-dev nftables iproute2 gcc-multilib tree
    for dir in /usr/include/x86_64-linux-gnu/*; do 
      ln -sfn "$dir" /usr/include/$(basename "$dir") 
    done
  
  elif [[ $GOARCH =~ arm64 ]]; then
    apt-get install -y llvm clang linux-libc-dev linux-headers-generic libbpf-dev libc6-dev nftables iproute2 gcc-aarch64-linux-gnu tree
    for dir in /usr/include/aarch64-linux-gnu/*; do 
      ln -sfn "$dir" /usr/include/$(basename "$dir")
    done
  fi
# Mariner
else
  tdnf install -y llvm clang libbpf-devel nftables tree
  for dir in /usr/include/aarch64-linux-gnu/*; do 
    if [[ -d $dir ]]; then
      ln -sfn "$dir" /usr/include/$(basename "$dir") 
    elif [[ -f "$dir" ]]; then
      ln -Tsfn "$dir" /usr/include/$(basename "$dir") 
    fi
  done
fi

ln -sfn /usr/include/"$ARCH"/asm /usr/include/asm
cp /lib/"$ARCH"/libnftables.so.1 "$OUT_DIR"/lib/
cp /lib/"$ARCH"/libedit.so.2 "$OUT_DIR"/lib/
cp /lib/"$ARCH"/libc.so.6 "$OUT_DIR"/lib/
cp /lib/"$ARCH"/libmnl.so.0 "$OUT_DIR"/lib/
cp /lib/"$ARCH"/libnftnl.so.11 "$OUT_DIR"/lib/
cp /lib/"$ARCH"/libxtables.so.12 "$OUT_DIR"/lib/
cp /lib/"$ARCH"/libjansson.so.4 "$OUT_DIR"/lib/
cp /lib/"$ARCH"/libgmp.so.10 "$OUT_DIR"/lib/
cp /lib/"$ARCH"/libtinfo.so.6 "$OUT_DIR"/lib/
cp /lib/"$ARCH"/libbsd.so.0 "$OUT_DIR"/lib/
cp /lib/"$ARCH"/libmd.so.0 "$OUT_DIR"/lib/

# Add Needed Binararies
cp /usr/sbin/nft "$OUT_DIR"/bins/nft
cp /sbin/ip "$OUT_DIR"/bins/ip


# Build IPv6 HP BPF
export C_INCLUDE_PATH=/usr/include/bpf
pushd "$REPO_ROOT"/bpf-prog/ipv6-hp-bpf
  cp ./cmd/ipv6-hp-bpf/*.go ./

  if [ "$DEBUG" = "true" ]; then 
    echo "\n#define DEBUG" >> ./include/helper.h
  fi

  GOOS=$OS CGO_ENABLED=0 go generate ./...
  GOOS=$OS CGO_ENABLED=0 go build -a -o "$OUT_DIR"/bins/ipv6-hp-bpf -trimpath -ldflags "-X main.version="$VERSION"" -gcflags="-dwarflocationlists=true" .
popd
