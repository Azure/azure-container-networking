#!/usr/bin/env bash

BUILD_CONTAINER_NAME=acn-builder

if [ ! "$(docker ps -q -f name=$BUILD_CONTAINER_NAME)" ]; then
    if [ "$(docker ps -aq -f status=exited -f name=$BUILD_CONTAINER_NAME)" ]; then
        docker rm -f $BUILD_CONTAINER_NAME
    fi
fi

GOOS=$1 GOARCH=$2 make all-containerized
