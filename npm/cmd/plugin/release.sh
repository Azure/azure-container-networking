#!/bin/bash
set -euo pipefail
shopt -s inherit_errexit
rm -rf ./bin; mkdir -p ./bin
go mod download; go build -o ./bin/azure-npm
tar -zcvf ./deploy/kubectl-azure-npm.tar.gz ./bin/* 
sha=$(sha256sum ./deploy/kubectl-azure-npm.tar.gz | awk '{ print $1 }')
echo $sha
sed "s/sha256string/$sha/" ./deploy/.template.yaml > ./deploy/kubectl-azure-npm.yaml
kubectl krew install --manifest=./deploy/kubectl-azure-npm.yaml --archive=./deploy/kubectl-azure-npm.tar.gz -v=4
