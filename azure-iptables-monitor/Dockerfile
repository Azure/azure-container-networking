ARG ARCH

# mcr.microsoft.com/azurelinux/base/core:3.0
FROM mcr.microsoft.com/azurelinux/base/core@sha256:9948138108a3d69f1dae62104599ac03132225c3b7a5ac57b85a214629c8567d AS mariner-core

# mcr.microsoft.com/azurelinux/distroless/minimal:3.0
FROM mcr.microsoft.com/azurelinux/distroless/minimal@sha256:0801b80a0927309572b9adc99bd1813bc680473175f6e8175cd4124d95dbd50c AS mariner-distroless

# skopeo inspect docker://mcr.microsoft.com/oss/go/microsoft/golang:1.23.2-azurelinux3.0 --format "{{.Name}}@{{.Digest}}"
FROM --platform=linux/${ARCH} mcr.microsoft.com/oss/go/microsoft/golang@sha256:f1f0cbd464ae4cd9d41176d47f1f9fe16a6965425871f817587314e3a04576ec AS go


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

FROM mariner-core AS iptables
RUN tdnf install -y iptables

FROM mariner-distroless AS linux
COPY --from=iptables /usr/sbin/*tables* /usr/sbin/
COPY --from=iptables /usr/lib /usr/lib
COPY --from=azure-iptables-monitor /go/bin/iptables-monitor azure-iptables-monitor
COPY --from=azure-block-iptables /go/bin/azure-block-iptables azure-block-iptables

ENTRYPOINT ["/azure-iptables-monitor"]
