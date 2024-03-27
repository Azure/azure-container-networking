FROM mcr.microsoft.com/oss/go/microsoft/golang:1.22 AS builder
ARG VERSION
ARG DEBUG
ARG OS
ARG ARCH
WORKDIR /bpf-prog/ipv6-hp-bpf
COPY ./bpf-prog/ipv6-hp-bpf .
COPY ./bpf-prog/ipv6-hp-bpf/cmd/ipv6-hp-bpf/*.go /bpf-prog/ipv6-hp-bpf/
COPY ./bpf-prog/ipv6-hp-bpf/include/helper.h /bpf-prog/ipv6-hp-bpf/include/helper.h
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
RUN if [ "$DEBUG" = "true" ]; then echo "\n#define DEBUG" >> /bpf-prog/ipv6-hp-bpf/include/helper.h; fi
RUN GOOS=$OS CGO_ENABLED=0 go generate ./...
RUN GOOS=$OS CGO_ENABLED=0 go build -a -o /go/bin/ipv6-hp-bpf -trimpath -ldflags "-X main.version="$VERSION"" -gcflags="-dwarflocationlists=true" .

FROM mcr.microsoft.com/cbl-mariner/distroless/minimal:2.0 AS linux
COPY --from=builder /go/bin/ipv6-hp-bpf /ipv6-hp-bpf
COPY --from=builder /usr/sbin/nft /usr/sbin/nft
COPY --from=builder /sbin/ip /sbin/ip
COPY --from=builder /tmp/lib/* /lib
CMD ["/ipv6-hp-bpf"]
