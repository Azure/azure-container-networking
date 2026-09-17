#!/bin/bash
set -eux

[[ $OS =~ windows ]] && FILE_EXT='.exe' || FILE_EXT=''

export CGO_ENABLED=0

mkdir -p "$OUT_DIR"/files
mkdir -p "$OUT_DIR"/bin
mkdir -p "$OUT_DIR"/scripts

pushd "$REPO_ROOT"/cns
  GOOS="$OS" go build -v -a \
    -o "$OUT_DIR"/bin/azure-cns"$FILE_EXT" \
    -ldflags "-s -w -X main.version="$CNS_VERSION" -X "$CNS_AI_PATH"="$CNS_AI_ID"" \
    -gcflags="-dwarflocationlists=true" \
    service/*.go
  cp kubeconfigtemplate.yaml "$OUT_DIR"/files/kubeconfigtemplate.yaml
  cp configuration/cns_config.json "$OUT_DIR"/files/cns_config.json
  cp ../npm/examples/windows/setkubeconfigpath.ps1 "$OUT_DIR"/scripts/setkubeconfigpath.ps1
popd
