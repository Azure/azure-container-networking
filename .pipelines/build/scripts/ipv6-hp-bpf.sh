#!/bin/bash
set -eux

export GOOS=$OS
export GOARCH=$ARCH
export CGO_ENABLED=0 
export C_INCLUDE_PATH=/usr/include/bpf

mkdir -p "$OUT_DIR"/bin
mkdir -p "$OUT_DIR"/lib

# Package up Needed C Files
if [[ -f /etc/debian_version ]];then
  apt-get update -y
  if [[ $GOARCH =~ amd64 ]]; then
    apt-get install -y llvm clang linux-libc-dev linux-headers-generic libbpf-dev libc6-dev nftables iproute2 gcc-multilib tree
    cp /lib/"$ARCH"/ld-linux-x86-64.so.2 "$OUT_DIR"/lib/
    for dir in /usr/include/x86_64-linux-gnu/*; do 
      ln -sfn "$dir" /usr/include/$(basename "$dir") 
    done
  
  elif [[ $GOARCH =~ arm64 ]]; then
    apt-get install -y llvm clang linux-libc-dev linux-headers-generic libbpf-dev libc6-dev nftables iproute2 gcc-aarch64-linux-gnu tree
    cp /lib/"$ARCH"/ld-linux-aarch64.so.1 "$OUT_DIR"/lib/
    for dir in /usr/include/aarch64-linux-gnu/*; do 
      ln -sfn "$dir" /usr/include/$(basename "$dir")
    done
  fi
# Mariner
else
  tdnf install -y llvm clang libbpf-devel nftables tree
  cp /lib/"$ARCH"/ld-linux-x86-64.so.2 "$OUT_DIR"/lib/
  for dir in /usr/include/x86_64-linux-gnu/*; do 
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
cp /usr/sbin/nft "$OUT_DIR"/bin/nft
cp /sbin/ip "$OUT_DIR"/bin/ip


# Build IPv6 HP BPF
pushd "$REPO_ROOT"/bpf-prog/ipv6-hp-bpf
  cp ./cmd/ipv6-hp-bpf/*.go .

  if [ "$DEBUG" = "true" ]; then 
    echo "\n#define DEBUG" >> ./include/helper.h
  fi

  go generate ./...
  go build -v -a -trimpath \
    -o "$OUT_DIR"/bin/ipv6-hp-bpf \
     -ldflags "-X main.version="$IPV6_HP_BPF_VERSION"" \
     -gcflags="-dwarflocationlists=true" .
popd
