#!/bin/bash
set -eux

[[ $OS =~ windows ]] && FILE_EXT='.exe' || FILE_EXT=''

# npm ships on an Ubuntu base that does not carry the Microsoft FIPS OpenSSL
# build, so use the standard Go crypto backend (matches npm/*.Dockerfile).
# Components on the AzureLinux distroless base use the default system crypto
# backend instead.
export MS_GO_NOSYSTEMCRYPTO=1
export CGO_ENABLED=0

mkdir -p "$OUT_DIR"/files
mkdir -p "$OUT_DIR"/bin
mkdir -p "$OUT_DIR"/scripts

pushd "$REPO_ROOT"/npm
  GOOS="$OS" go build -a -v -trimpath \
    -o "$OUT_DIR"/bin/azure-npm"$FILE_EXT" \
    -ldflags "-s -w -X main.version="$NPM_VERSION" -X "$NPM_AI_PATH"="$NPM_AI_ID"" \
    -gcflags="-dwarflocationlists=true" \
    ./cmd/*.go

  cp ./examples/windows/kubeconfigtemplate.yaml "$OUT_DIR"/files/kubeconfigtemplate.yaml
  cp ./examples/windows/setkubeconfigpath.ps1 "$OUT_DIR"/scripts/setkubeconfigpath.ps1
  cp ./examples/windows/setkubeconfigpath-capz.ps1 "$OUT_DIR"/scripts/setkubeconfigpath-capz.ps1
popd
