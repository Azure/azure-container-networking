#!/bin/bash
set -eux

[[ $OS =~ windows ]] && FILE_EXT='.exe' || FILE_EXT=''

export CGO_ENABLED=0
# npm ships on the Ubuntu base image (it needs iptables/ipset at runtime), which
# does not provide Microsoft's FIPS-capable OpenSSL. GOEXPERIMENT=ms_nocgo_openssl
# crypto would make the binary require that OpenSSL and crash-loop on FIPS-enabled
# clusters, so use the standard Go crypto backend (matches npm/*.Dockerfile and
# the shipped release/v1.6 image). Components on the AzureLinux distroless base
# use ms_nocgo_opensslcrypto instead.
export MS_GO_NOSYSTEMCRYPTO=1

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
