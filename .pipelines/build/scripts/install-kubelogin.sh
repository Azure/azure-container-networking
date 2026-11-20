#!/bin/bash
set -eux

# Install kubelogin, which kubectl uses to authenticate to Microsoft Entra ID-enabled AKS clusters.
# Go is pre-installed in the build container, so we use go install.
if ! command -v kubelogin &> /dev/null; then
  go install github.com/Azure/kubelogin@v0.2.22
  sudo mv "$(go env GOPATH)/bin/kubelogin" /usr/local/bin/kubelogin
fi

kubelogin --version
