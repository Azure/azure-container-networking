#!/usr/bin/env bash
if [[ -z $1 ]] || [[ -z $2 ]]; then
    echo "Usage: $0 <os> <architecture>"
    echo "       Example: $0 linux amd64"
    exit 1
fi
GOOS=$1 GOARCH=$2 make all-binaries
