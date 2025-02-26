ARG VARIANT="1.24"
FROM mcr.microsoft.com/vscode/devcontainers/go:${VARIANT}
RUN apt-get update && \
    export DEBIAN_FRONTEND=noninteractive && \
    apt-get -y install --no-install-recommends \
    podman \
    buildah  \
    skopeo \
    vim \
    fzf \
    ripgrep \
    jq \
    curl
