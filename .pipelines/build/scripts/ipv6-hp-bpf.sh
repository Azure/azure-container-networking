#!/bin/bash
set -eux

function findcp::shared_library() {
  local filename
  filename="${1}"
  local copy_to
  copy_to="${2}"
  local search_dirs
  search_dirs="${@:3}"

  for dir in $search_dirs; do
    if [[ -d "$dir" ]]; then
      if [[ "$filename" =~ ^.*\.so.*$ ]]; then
        found=$(find "$dir" -name "$filename")
      else
        found=$(find "$dir" -name ""$filename".so*")
      fi

      if [[ -n $found ]]; then
        break;
      fi
    else
      echo >&2 "##[debug]Not a directory. Skipping..."
      echo >&2 "##[debug]Dir: "$dir""
    fi
  done

  echo -e >&2 "##[debug]Found: \n$found"
  select=$(echo "$found" | head -n1)

  echo -e >&2 "##[debug]Selected: \n$select"
  echo >&2 "##[debug]cp "$select" "$copy_to""
  cp "$select" "$copy_to"
}


[[ $OS =~ windows ]] && FILE_EXT='.exe' || FILE_EXT=''

export CGO_ENABLED=0
export C_INCLUDE_PATH=/usr/include/bpf

mkdir -p "$OUT_DIR"/bin
mkdir -p "$OUT_DIR"/lib

# Package up Needed C Files
if [[ -f /etc/debian_version ]];then
  apt-get update -y
  apt-get install -y --no-install-recommends llvm clang linux-libc-dev libbpf-dev libc6-dev nftables iproute2
  if [[ $ARCH =~ amd64 ]]; then
    apt-get install -y --no-install-recommends gcc-multilib

    ARCH=x86_64-linux-gnu
    cp /usr/lib/"$ARCH"/ld-linux-x86-64.so.2 "$OUT_DIR"/lib/

  elif [[ $ARCH =~ arm64 ]]; then
    apt-get install -y --no-install-recommends gcc-aarch64-linux-gnu

    ARCH=aarch64-linux-gnu
    cp /usr/lib/"$ARCH"/ld-linux-aarch64.so.1 "$OUT_DIR"/lib/
  fi

  for dir in /usr/include/"$ARCH"/*; do
    ln -sfn "$dir" /usr/include/$(basename "$dir")
  done

  echo >&2 "##[group]lib $ARCH directory list"
    ls -la /lib/"$ARCH" || true
  echo >&2 "##[endgroup]"
  echo >&2 "##[group]usr lib directory list"
    ls -la /usr/lib || true
  echo >&2 "##[endgroup]"
  echo >&2 "##[group]usr lib $ARCH directory list"
    ls -la /usr/lib/"$ARCH" || true
  echo >&2 "##[endgroup]"

  # Copy Shared Library Files
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


# Mariner
else
  tdnf install -y llvm clang libbpf-devel nftables gcc binutils iproute glibc
  if [[ $ARCH =~ amd64 ]]; then
    ARCH=x86_64-linux-gnu
    if [[ -f '/usr/lib/ld-linux-x86-64.so.2' ]]; then
      cp /usr/lib/ld-linux-x86-64.so.2 "$OUT_DIR"/lib/
    fi
  elif [[ $ARCH =~ arm64 ]]; then
    ARCH=aarch64-linux-gnu
    #tdnf install -y glibc-devel.i386
    if [[ -f '/usr/lib/ld-linux-aarch64.so.1' ]]; then
      cp /usr/lib/ld-linux-aarch64.so.1 "$OUT_DIR"/lib/
    fi
  fi
  for dir in /usr/include/"$ARCH"/*; do
    if [[ -d $dir ]]; then
      ln -sfn "$dir" /usr/include/$(basename "$dir")
    elif [[ -f "$dir" ]]; then
      ln -Tsfn "$dir" /usr/include/$(basename "$dir")
    fi
  done

  echo >&2 "##[group]usr include $ARCH directory list"
    ls -la /usr/include/"$ARCH" || true
  echo >&2 "##[endgroup]"

  echo >&2 "##[group]usr lib directory list"
    ls -la /usr/lib || true
  echo >&2 "##[endgroup]"

  echo >&2 "##[group]usr lib ldscripts directory list"
    ls -la /usr/lib/ldscripts || true
  echo >&2 "##[endgroup]"

  # Copy Shared Library Files
  ln -sfn /usr/include/"$ARCH"/asm /usr/include/asm
  cp /usr/lib/libnftables.so.1 "$OUT_DIR"/lib/
  cp /usr/lib/libedit.so.0 "$OUT_DIR"/lib/
  cp /usr/lib/libc.so.6 "$OUT_DIR"/lib/
  cp /usr/lib/libmnl.so.0 "$OUT_DIR"/lib/
  cp /usr/lib/libnftnl.so.11 "$OUT_DIR"/lib/
  cp /usr/lib/libxtables.so.12 "$OUT_DIR"/lib/
  cp /usr/lib/libjansson.so.4 "$OUT_DIR"/lib/
  cp /usr/lib/libgmp.so.10 "$OUT_DIR"/lib/
  cp /usr/lib/libtinfo.so.6 "$OUT_DIR"/lib/

  cp /usr/lib/libbsd.so.0 "$OUT_DIR"/lib/ || tdnf install -y libbsd-devel
  findcp::shared_library libbsd.so "$OUT_DIR"/lib/ /usr/lib /lib /lib32 /lib64
  cp /usr/lib/libmd.so.0 "$OUT_DIR"/lib/ || tdnf install -y libmd-devel
  findcp::shared_library libmd.so "$OUT_DIR"/lib/ /usr/lib /lib /lib32 /lib64
fi


# Add Needed Binararies
cp /usr/sbin/nft "$OUT_DIR"/bin/nft"$FILE_EXT"
cp /sbin/ip "$OUT_DIR"/bin/ip"$FILE_EXT"


# Build IPv6 HP BPF
pushd "$REPO_ROOT"/bpf-prog/ipv6-hp-bpf
  cp ./cmd/ipv6-hp-bpf/*.go .

  if [[ "$DEBUG" =~ ^[T|t]rue$ ]]; then
    echo -e "\n#define DEBUG" >> ./include/helper.h
  fi

  go generate ./...
  GOOS="$OS" go build -v -a -trimpath \
    -o "$OUT_DIR"/bin/ipv6-hp-bpf"$FILE_EXT" \
    -ldflags "-s -w -X main.version="$IPV6_HP_BPF_VERSION"" \
    -gcflags="-dwarflocationlists=true" .
popd
