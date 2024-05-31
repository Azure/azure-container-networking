FROM mcr.microsoft.com/oss/go/microsoft/golang:1.21 AS builder
ARG VERSION
ARG DEBUG
ARG OS
WORKDIR /bpf-prog/ipv6-hp-bpf
COPY ./bpf-prog/ipv6-hp-bpf .
COPY ./bpf-prog/ipv6-hp-bpf/cmd/ipv6-hp-bpf/*.go /bpf-prog/ipv6-hp-bpf/
COPY ./bpf-prog/ipv6-hp-bpf/include/helper.h /bpf-prog/ipv6-hp-bpf/include/helper.h
RUN apt-get update && apt-get install -y llvm clang linux-libc-dev linux-headers-generic libbpf-dev libc6-dev gcc-multilib nftables iproute2
RUN for dir in /usr/include/x86_64-linux-gnu/*; do ln -s "$dir" /usr/include/$(basename "$dir"); done
ENV C_INCLUDE_PATH=/usr/include/bpf
RUN if [ "$DEBUG" = "true" ]; then echo "\n#define DEBUG" >> /bpf-prog/ipv6-hp-bpf/include/helper.h; fi
RUN GOOS=$OS CGO_ENABLED=0 go generate ./...
RUN GOOS=$OS CGO_ENABLED=0 go build -a -o /go/bin/ipv6-hp-bpf -trimpath -ldflags "-X main.version="$VERSION"" -gcflags="-dwarflocationlists=true" .

FROM mcr.microsoft.com/cbl-mariner/distroless/minimal:2.0
COPY --from=builder /go/bin/ipv6-hp-bpf /ipv6-hp-bpf
COPY --from=builder /usr/sbin/nft /usr/sbin/nft
COPY --from=builder /sbin/ip /sbin/ip
COPY --from=builder /lib/x86_64-linux-gnu/libnftables.so.1 /lib/x86_64-linux-gnu/
COPY --from=builder /lib/x86_64-linux-gnu/libedit.so.2 /lib/x86_64-linux-gnu/
COPY --from=builder /lib/x86_64-linux-gnu/libc.so.6 /lib/x86_64-linux-gnu/
COPY --from=builder /lib/x86_64-linux-gnu/libmnl.so.0 /lib/x86_64-linux-gnu/
COPY --from=builder /lib/x86_64-linux-gnu/libnftnl.so.11 /lib/x86_64-linux-gnu/
COPY --from=builder /lib/x86_64-linux-gnu/libxtables.so.12 /lib/x86_64-linux-gnu/
COPY --from=builder /lib/x86_64-linux-gnu/libjansson.so.4 /lib/x86_64-linux-gnu/
COPY --from=builder /lib/x86_64-linux-gnu/libgmp.so.10 /lib/x86_64-linux-gnu/
COPY --from=builder /lib/x86_64-linux-gnu/libtinfo.so.6 /lib/x86_64-linux-gnu/
COPY --from=builder /lib/x86_64-linux-gnu/libbsd.so.0 /lib/x86_64-linux-gnu/
COPY --from=builder /lib64/ld-linux-x86-64.so.2 /lib64/
COPY --from=builder /lib/x86_64-linux-gnu/libmd.so.0 /lib/x86_64-linux-gnu/
CMD ["/ipv6-hp-bpf"]