FROM mcr.microsoft.com/oss/go/microsoft/golang:1.21 AS builder
ARG VERSION
WORKDIR /bpf-prog/bpf-tc
COPY ./bpf-prog/bpf-tc .
COPY ./bpf-prog/bpf-tc/cmd/bpf-tc/*.go /bpf-prog/bpf-tc/
RUN apt-get clean
RUN apt-get update && apt-get install -y llvm clang linux-libc-dev linux-headers-generic libbpf-dev libc6-dev gcc-multilib nftables
RUN for dir in /usr/include/x86_64-linux-gnu/*; do ln -s "$dir" /usr/include/$(basename "$dir"); done
ENV C_INCLUDE_PATH=/usr/include/bpf
RUN GOOS=$OS CGO_ENABLED=0 go generate ./...
RUN GOOS=$OS CGO_ENABLED=0 go build -a -o /go/bin/bpf-tc -trimpath -ldflags "-X main.version="$VERSION"" -gcflags="-dwarflocationlists=true" .

CMD ["./bpftc"]