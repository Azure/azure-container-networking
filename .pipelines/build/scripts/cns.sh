#!/bin/bash
set -nex

export GOOS=$OS
export GOARCH=$ARCH
export CGO_ENABLED=0 

mkdir -p "$OUT_DIR"/files
mkdir -p "$OUT_DIR"/bins

pushd "$REPO_ROOT"/cns
  go build -v -a \
    -o "$OUT_DIR"/bins/azure-cns \
    -ldflags "-X main.version="$CNS_VERSION" -X "$CNS_AI_PATH"="$CNS_AI_ID"" \
    -gcflags="-dwarflocationlists=true" \
    service/*.go
  cp kubeconfigtemplate.yaml "$OUT_DIR"/files/kubeconfigtemplate.yaml
  cp ../npm/examples/windows/setkubeconfigpath.ps1 "$OUT_DIR"/files/setkubeconfigpath.ps1
  cp configuration/cns_config.json "$OUT_DIR"/files/cns_config.json
popd
