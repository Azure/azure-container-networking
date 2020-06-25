#!/usr/bin/env bash
if [[ -z $1 ]] || [[ -z $2 ]]; then
    echo "Usage: $0 <os> <architecture>"
    echo "       Example: $0 linux amd64"
    exit 1
fi

BUILD_CONTAINER_NAME=acn-builder

if [ ! "$(docker ps -q -f name=$BUILD_CONTAINER_NAME)" ]; then
    if [ "$(docker ps -aq -f status=exited -f name=$BUILD_CONTAINER_NAME)" ]; then
        docker rm -f $BUILD_CONTAINER_NAME
    fi
fi

GOOS=$1 GOARCH=$2 make all-containerized
