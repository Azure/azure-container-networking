#!/bin/bash
set -eux

[[ $OS =~ windows ]] && FILE_EXT='.exe' || FILE_EXT=''

export GOOS=$OS
export GOARCH=$ARCH
export CGO_ENABLED=0 

mkdir -p "$GEN_DIR"
mkdir -p "$OUT_DIR"/bin

DROPGZ_BUILD_DIR=$(mktemp -d -p "$GEN_DIR")
PAYLOAD_DIR=$(mktemp -d -p "$GEN_DIR")
DROPGZ_VERSION="${DROPGZ_VERSION:-v0.0.12}"
DROPGZ_MOD_DOWNLOAD_PATH=""$ACN_PACKAGE_PATH"/dropgz@"$DROPGZ_VERSION""
DROPGZ_MOD_DOWNLOAD_PATH=$(echo "$DROPGZ_MOD_DOWNLOAD_PATH" | tr '[:upper:]' '[:lower:]')

mkdir -p "$DROPGZ_BUILD_DIR"

echo >&2 "##[section]Construct DropGZ Embedded Payload"
pushd "$PAYLOAD_DIR"
  [[ -n $(stat "$OUT_DIR"/files 2>/dev/null || true) ]] && cp "$OUT_DIR"/files/* .
  [[ -n $(stat "$OUT_DIR"/scripts 2>/dev/null || true) ]] && cp "$OUT_DIR"/scripts/* .
  [[ -n $(stat "$OUT_DIR"/bin 2>/dev/null || true) ]] && cp "$OUT_DIR"/bin/* .
  
  sha256sum * > sum.txt
  gzip --verbose --best --recursive .

  for file in $(find . -name '*.gz'); do 
    mv "$file" "${file%%.gz}"
  done
popd

echo >&2 "##[section]Download DropGZ ($DROPGZ_VERSION)"
GOPATH="$DROPGZ_BUILD_DIR" \
  go mod download "$DROPGZ_MOD_DOWNLOAD_PATH"

ls -la
ls -la "$GEN_DIR"
ls -la "$DROPGZ_BUILD_DIR"
apt-get install -y tree || tdnf install -y tree
tree "$GEN_DIR"
echo >&2 "##[section]Build DropGZ with Embedded Payload"
pushd "$DROPGZ_BUILD_DIR"/pkg/mod/"$DROPGZ_MOD_DOWNLOAD_PATH"
  mv "$PAYLOAD_DIR"/* pkg/embed/fs/
  go build -v -trimpath -a \
    -o "$OUT_DIR"/bin/dropgz"$FILE_EXT" \
    -ldflags "-X github.com/Azure/azure-container-networking/dropgz/internal/buildinfo.Version="$DROPGZ_VERSION"" \
    -gcflags="-dwarflocationlists=true" \
    main.go
popd
