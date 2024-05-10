FROM mcr.microsoft.com/oss/go/microsoft/golang:1.21 AS builder
ARG VERSION
ARG DEBUG
ARG OS
WORKDIR /bpf-prog/ipv6-healthprobe-tc
COPY ./bpf-prog/ipv6-healthprobe-tc .
COPY ./bpf-prog/ipv6-healthprobe-tc/cmd/ipv6-healthprobe-tc/*.go /bpf-prog/ipv6-healthprobe-tc/
COPY ./bpf-prog/ipv6-healthprobe-tc/include/helper.h /bpf-prog/ipv6-healthprobe-tc/include/helper.h
RUN apt-get clean
RUN apt-get update && apt-get install -y llvm clang linux-libc-dev linux-headers-generic libbpf-dev libc6-dev gcc-multilib nftables iproute2
RUN for dir in /usr/include/x86_64-linux-gnu/*; do ln -s "$dir" /usr/include/$(basename "$dir"); done
ENV C_INCLUDE_PATH=/usr/include/bpf
RUN if [ "$DEBUG" = "true" ]; then echo "\n#define DEBUG" >> /bpf-prog/ipv6-healthprobe-tc/include/helper.h; fi
RUN GOOS=$OS CGO_ENABLED=0 go generate ./...
RUN GOOS=$OS CGO_ENABLED=0 go build -a -o /go/bin/ipv6-healthprobe-tc -trimpath -ldflags "-X main.version="$VERSION"" -gcflags="-dwarflocationlists=true" .

CMD ["./ipv6-healthprobe-tc"]