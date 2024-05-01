ARG ARCH
ARG DROPGZ_VERSION=v0.0.12
ARG OS_VERSION
ARG OS

FROM mcr.microsoft.com/oss/go/microsoft/golang:1.21 AS bpf-tc
ARG OS
ARG VERSION
WORKDIR /bpf-tc
COPY ./bpf-tc .
RUN GOOS=$OS CGO_ENABLED=0 go build -a -o /go/bin/bpf-tc -trimpath -ldflags "-X main.version="$VERSION"" -gcflags="-dwarflocationlists=true" .

FROM mcr.microsoft.com/cbl-mariner/base/core:2.0 AS compressor
ARG OS
WORKDIR /payload
COPY --from=bpf-tc /go/bin/* /payload
COPY --from=bpf-tc /bpf-tc/*.conflist /payload
RUN cd /payload && sha256sum * > sum.txt
RUN gzip --verbose --best --recursive /payload && for f in /payload/*.gz; do mv -- "$f" "${f%%.gz}"; done

FROM mcr.microsoft.com/oss/go/microsoft/golang:1.21 AS dropgz
ARG DROPGZ_VERSION
ARG OS
ARG VERSION
RUN go mod download github.com/azure/azure-container-networking/dropgz@$DROPGZ_VERSION
WORKDIR /go/pkg/mod/github.com/azure/azure-container-networking/dropgz\@$DROPGZ_VERSION
COPY --from=compressor /payload/* pkg/embed/fs/
RUN GOOS=$OS CGO_ENABLED=0 go build -a -o /go/bin/dropgz -trimpath -ldflags "-X github.com/Azure/azure-container-networking/dropgz/internal/buildinfo.Version="$VERSION"" -gcflags="-dwarflocationlists=true" main.go

FROM scratch
COPY --from=dropgz /go/bin/dropgz dropgz
ENTRYPOINT [ "/dropgz" ]
