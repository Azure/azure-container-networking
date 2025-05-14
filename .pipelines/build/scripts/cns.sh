#!/bin/bash
#ARG CNS_AI_ID
#ARG CNS_AI_PATH

mkdir -p "$OUT_DIR"/files
mkdir -p "$OUT_DIR"/bins

pushd "$REPO_ROOT"
  GOOS=$OS CGO_ENABLED=0 go build -a -o "$OUT_DIR"/bins/azure-cns -ldflags "-X main.version="$VERSION" -X "$CNS_AI_PATH"="$CNS_AI_ID"" -gcflags="-dwarflocationlists=true" cns/service/*.go
  cp cns/kubeconfigtemplate.yaml "$OUT_DIR"/files/kubeconfigtemplate.yaml
  cp npm/examples/windows/setkubeconfigpath.ps1 "$OUT_DIR"/files/setkubeconfigpath.ps1
popd
