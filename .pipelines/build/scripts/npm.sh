#!/bin/bash
set -nex

export GOOS=$OS
export GOARCH=$ARCH
export CGO_ENABLED=0 

mkdir -p "$OUT_DIR"/files
mkdir -p "$OUT_DIR"/bin

pushd "$ROOT_DIR"/npm
  go build -a -v -trimpath \
    -o "$OUT_DIR"/bin/azure-npm \
    -ldflags "-X main.version="$NPM_VERSION" -X "$NPM_AI_PATH"="$NPM_AI_ID"" \
    -gcflags="-dwarflocationlists=true" \
    ./cmd/*.go

  cp ./examples/windows/kubeconfigtemplate.yaml "$OUT_DIR"/files/kubeconfigtemplate.yaml
  cp ./examples/windows/setkubeconfigpath.ps1 "$OUT_DIR"/files/setkubeconfigpath.ps1
  cp ./examples/windows/setkubeconfigpath-capz.ps1 "$OUT_DIR"/files/setkubeconfigpath-capz.ps1
popd
