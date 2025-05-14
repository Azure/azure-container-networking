#!/bin/bash
set -nex

mkdir -p "$OUT_DIR"/files
mkdir -p "$OUT_DIR"/bins

pushd "$ROOT_DIR"/npm
  GOOS=$OS CGO_ENABLED=0 go build -v -o "$OUT_DIR"/bins/azure-npm -ldflags "-X main.version="$VERSION" -X "$NPM_AI_PATH"="$NPM_AI_ID"" -gcflags="-dwarflocationlists=true" ./cmd/*.go

  cp ./examples/windows/kubeconfigtemplate.yaml "$OUT_DIR"/files/kubeconfigtemplate.yaml
  cp ./examples/windows/setkubeconfigpath.ps1 "$OUT_DIR"/files/setkubeconfigpath.ps1
  cp ./examples/windows/setkubeconfigpath-capz.ps1 "$OUT_DIR"/files/setkubeconfigpath-capz.ps1
popd
