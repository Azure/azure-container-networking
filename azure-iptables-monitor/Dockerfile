ARG ARCH

# mcr.microsoft.com/azurelinux/base/core:3.0
FROM mcr.microsoft.com/azurelinux/base/core:3.0@sha256:7292f2bcf52c3916758f1a45aba6e4dcb93f466027d7786900ea96c2d23c48e2 AS mariner-core

# mcr.microsoft.com/azurelinux/distroless/base:3.0
FROM mcr.microsoft.com/azurelinux/distroless/base:3.0@sha256:431ae1dc2924c41ae54c97c25676990f0c61d6295f42f5d07783c6976f58f1ef AS mariner-distroless

# mcr.microsoft.com/oss/go/microsoft/golang:1.27-azurelinux3.0
FROM --platform=linux/${ARCH} mcr.microsoft.com/oss/go/microsoft/golang:1.27-azurelinux3.0@sha256:7698ebf51921051df706c4a050c3d69bdd5b6f48b8c6f6166429f22f18d1ae8a AS go


FROM go AS azure-iptables-monitor
ARG OS
ARG VERSION
WORKDIR /azure-iptables-monitor
COPY ./azure-iptables-monitor .
RUN GOOS=$OS CGO_ENABLED=0 go build -a -o /go/bin/iptables-monitor -trimpath -ldflags "-s -w -X main.version="$VERSION"" -gcflags="-dwarflocationlists=true" .

FROM go AS azure-block-iptables
ARG OS
ARG AZURE_BLOCK_IPTABLES_VERSION
ARG ARCH
WORKDIR /azure-container-networking
COPY ./bpf-prog/azure-block-iptables ./bpf-prog/azure-block-iptables
COPY ./go.mod ./go.sum ./
# Install BPF development dependencies for Azure Linux (mariner)
RUN tdnf install -y llvm clang libbpf-devel gcc binutils glibc
# Set up C include path for BPF
ENV C_INCLUDE_PATH=/usr/include/bpf
# Set up architecture-specific symlinks for cross-compilation support
RUN if [ "$ARCH" = "amd64" ]; then \
        ARCH_DIR=x86_64-linux-gnu; \
    elif [ "$ARCH" = "arm64" ]; then \
        ARCH_DIR=aarch64-linux-gnu; \
    fi && \
    if [ -n "$ARCH_DIR" ] && [ -d "/usr/include/$ARCH_DIR" ]; then \
        for dir in /usr/include/"$ARCH_DIR"/*; do \
            if [ -d "$dir" ]; then \
                ln -sfn "$dir" /usr/include/$(basename "$dir") || echo "Warning: Failed to create symlink for directory $dir" >&2; \
            elif [ -f "$dir" ]; then \
                ln -Tsfn "$dir" /usr/include/$(basename "$dir") || echo "Warning: Failed to create symlink for file $dir" >&2; \
            fi; \
        done; \
    fi
RUN GOOS=$OS CGO_ENABLED=0 go generate ./bpf-prog/azure-block-iptables/...
RUN GOOS=$OS CGO_ENABLED=0 go build -a -o /go/bin/azure-block-iptables -trimpath -ldflags "-s -w -X main.version="$AZURE_BLOCK_IPTABLES_VERSION"" -gcflags="-dwarflocationlists=true" ./bpf-prog/azure-block-iptables/cmd/azure-block-iptables

FROM mariner-core AS iptools
RUN tdnf install -y iptables iproute

FROM mariner-distroless AS linux
COPY --from=iptools /usr/sbin/*tables* /usr/sbin/
COPY --from=iptools /usr/sbin/ip /usr/sbin/
COPY --from=iptools /usr/lib /usr/lib
COPY --from=iptools /usr/lib64 /usr/lib64
COPY --from=azure-iptables-monitor /go/bin/iptables-monitor azure-iptables-monitor
COPY --from=azure-block-iptables /go/bin/azure-block-iptables azure-block-iptables

ENTRYPOINT ["/azure-iptables-monitor"]
