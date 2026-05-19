#!/bin/bash
set -eux

# Install crane (google/go-containerregistry) for daemonless container image extraction.
# crane can pull and export image filesystems without a Docker daemon.
# Go is pre-installed in the build container, so we use go install.
# Rely on go install for supply chain security and reproducibility
if ! command -v crane &> /dev/null; then
  go install github.com/google/go-containerregistry/cmd/crane@v0.21.3
  sudo mv "$(go env GOPATH)/bin/crane" /usr/local/bin/crane
fi

crane version
