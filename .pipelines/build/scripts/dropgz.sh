#!/bin/bash
set -eux

function _remove_exe_extension() {
  local file_path
  file_path="${1}"
  file_dir=$(dirname "$file_path")
  file_dir=$(realpath "$file_dir")
  file_basename=$(basename "$file_path" '.exe')
  mv "$file_path" "$file_dir"/"$file_basename"
}
function files::remove_exe_extensions() {
  local target_dir
  target_dir="${1}"

  for file in $(find "$target_dir" -type f -name '*.exe'); do
    _remove_exe_extension "$file"
  done
}

[[ $OS =~ windows ]] && FILE_EXT='.exe' || FILE_EXT=''

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
  [[ -d "$OUT_DIR"/files ]] && cp "$OUT_DIR"/files/* . || true
  [[ -d "$OUT_DIR"/scripts ]] && cp "$OUT_DIR"/scripts/* . || true
  [[ -d "$OUT_DIR"/bin ]] && cp "$OUT_DIR"/bin/* . || true

  [[ $OS =~ windows ]] && files::remove_exe_extensions .

  sha256sum * > sum.txt
  gzip --verbose --best --recursive .

  for file in $(find . -name '*.gz'); do
    mv "$file" "${file%%.gz}"
  done
popd

echo >&2 "##[section]Download DropGZ ($DROPGZ_VERSION)"
GOPATH="$DROPGZ_BUILD_DIR" \
  go mod download "$DROPGZ_MOD_DOWNLOAD_PATH"

echo >&2 "##[section]Build DropGZ with Embedded Payload"
pushd "$DROPGZ_BUILD_DIR"/pkg/mod/"$DROPGZ_MOD_DOWNLOAD_PATH"
  mv "$PAYLOAD_DIR"/* pkg/embed/fs/
  GOOS="$OS" go build -v -trimpath -a \
    -o "$OUT_DIR"/bin/dropgz"$FILE_EXT" \
    -ldflags "-s -w -X github.com/Azure/azure-container-networking/dropgz/internal/buildinfo.Version="$DROPGZ_VERSION"" \
    -gcflags="-dwarflocationlists=true" \
    main.go
popd
