ARG ARCH

# mcr.microsoft.com/oss/go/microsoft/golang:1.23-cbl-mariner2.0
FROM --platform=linux/${ARCH} mcr.microsoft.com/oss/go/microsoft/golang@sha256:b06999cae63b9b6f43bcb16bd16bcbedae847684515317e15607a601ed108030 AS go

# skopeo inspect docker://mcr.microsoft.com/oss/go/microsoft/golang:1.23.2 --format "{{.Name}}@{{.Digest}}"
FROM --platform=linux/${ARCH} mcr.microsoft.com/oss/go/microsoft/golang@sha256:86c5b00bbed2a6e7157052d78bf4b45c0bf26545ed6e8fd7dbad51ac9415f534 AS builder-ipv6-hp-bpf
ARG VERSION
ARG DEBUG
ARG OS
ARG ARCH

RUN apt-get update && apt-get install -y llvm clang linux-libc-dev linux-headers-generic libbpf-dev libc6-dev nftables iproute2
RUN mkdir -p /tmp/lib
RUN if [ "$ARCH" = "arm64" ]; then \
    apt-get install -y gcc-aarch64-linux-gnu && \
    ARCH=aarch64-linux-gnu && \
    cp /lib/"$ARCH"/ld-linux-aarch64.so.1 /tmp/lib/ && \
    for dir in /usr/include/"$ARCH"/*; do ln -s "$dir" /usr/include/$(basename "$dir"); done; \
    elif [ "$ARCH" = "amd64" ]; then \
    apt-get install -y gcc-multilib && \
    ARCH=x86_64-linux-gnu && \
    cp /lib/"$ARCH"/ld-linux-x86-64.so.2 /tmp/lib/ && \
    for dir in /usr/include/"$ARCH"/*; do ln -s "$dir" /usr/include/$(basename "$dir"); done; \
    fi && \
    ln -sfn /usr/include/"$ARCH"/asm /usr/include/asm && \
    cp /lib/"$ARCH"/libnftables.so.1 /tmp/lib/ && \
    cp /lib/"$ARCH"/libedit.so.2 /tmp/lib/ && \
    cp /lib/"$ARCH"/libc.so.6 /tmp/lib/ && \
    cp /lib/"$ARCH"/libmnl.so.0 /tmp/lib/ && \
    cp /lib/"$ARCH"/libnftnl.so.11 /tmp/lib/ && \
    cp /lib/"$ARCH"/libxtables.so.12 /tmp/lib/ && \
    cp /lib/"$ARCH"/libjansson.so.4 /tmp/lib/ && \
    cp /lib/"$ARCH"/libgmp.so.10 /tmp/lib/ && \
    cp /lib/"$ARCH"/libtinfo.so.6 /tmp/lib/ && \
    cp /lib/"$ARCH"/libbsd.so.0 /tmp/lib/ && \
    cp /lib/"$ARCH"/libmd.so.0 /tmp/lib/
ENV C_INCLUDE_PATH=/usr/include/bpf
