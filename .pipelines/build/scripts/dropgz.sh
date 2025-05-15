export GOOS=$OS
export GOARCH=$ARCH
export CGO_ENABLED=0 

DROPGZ_VERSION="${DROPGZ_VERSION:-v0.0.12}"
DROPGZ_BUILD_DIR=$(mktemp -d -p "$GEN_DIR")
DROPGZ_MOD_DOWNLOAD_PATH=""$ACN_PACKAGE_PATH"/dropgz@"$DROPGZ_VERSION""

mkdir -p "$OUT_DIR"/bin
mkdir -p "$DROPGZ_BUILD_DIR"

GOPATH="$DROPGZ_BUILD_DIR" \
  go mod download "$DROPGZ_MOD_DOWNLOAD_PATH"

pushd "$DROPGZ_BUILD_DIR"/pkg/mod/"$DROPGZ_MOD_DOWNLOAD_PATH"
  [[ -n $(stat "$OUT_DIR"/files 2>/dev/null || true) ]] && cp "$OUT_DIR"/files/* pkg/embed/fs/
  [[ -n $(stat "$OUT_DIR"/bin 2>/dev/null || true) ]] && cp "$OUT_DIR"/bin/* pkg/embed/fs/
  go build -v -trimpath -a \
    -o "$OUT_DIR"/bin/dropgz \
    -ldflags "-X github.com/Azure/azure-container-networking/dropgz/internal/buildinfo.Version="$DROPGZ_VERSION"" \
    -gcflags="-dwarflocationlists=true" \
    main.go
popd
