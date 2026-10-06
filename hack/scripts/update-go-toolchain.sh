#!/usr/bin/env bash

set -euo pipefail

if [[ $# -ne 1 ]]; then
    echo "usage: $0 <go-version>" >&2
    exit 2
fi

version="${1#go}"
toolchain="go${version}"

while IFS= read -r -d '' modfile; do
    go mod edit -toolchain="$toolchain" "$modfile"
done < <(find . -name go.mod -not -path '*/vendor/*' -print0)

if [[ -f tools.go.mod ]]; then
    go mod edit -modfile=tools.go.mod -toolchain="$toolchain"
fi
