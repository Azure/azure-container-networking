ARG ARCH
ARG DROPGZ_VERSION=v0.0.12
ARG OS
ARG OS_VERSION

# skopeo inspect docker://mcr.microsoft.com/oss/go/microsoft/golang:1.24.1-cbl-mariner2.0 --format "{{.Name}}@{{.Digest}}"
FROM --platform=linux/${ARCH} mcr.microsoft.com/oss/go/microsoft/golang@sha256:82f110263e6110403fbbef97153f7532780b01afd44d5906753ac31a6b1b9e90 AS azure-vnet
ARG OS
ARG VERSION
WORKDIR /azure-container-networking
COPY . .
RUN GOOS=$OS CGO_ENABLED=0 go build -a -o /go/bin/azure-vnet -trimpath -ldflags "-X main.version="$VERSION"" -gcflags="-dwarflocationlists=true" cni/network/plugin/main.go
RUN GOOS=$OS CGO_ENABLED=0 go build -a -o /go/bin/azure-vnet-telemetry -trimpath -ldflags "-X main.version="$VERSION"" -gcflags="-dwarflocationlists=true" cni/telemetry/service/telemetrymain.go
RUN GOOS=$OS CGO_ENABLED=0 go build -a -o /go/bin/azure-vnet-ipam -trimpath -ldflags "-X main.version="$VERSION"" -gcflags="-dwarflocationlists=true" cni/ipam/plugin/main.go

FROM --platform=linux/${ARCH} mcr.microsoft.com/cbl-mariner/base/core:2.0 AS compressor
ARG OS
WORKDIR /payload
COPY --from=azure-vnet /go/bin/* /payload/
COPY --from=azure-vnet /azure-container-networking/cni/azure-$OS.conflist /payload/azure.conflist
COPY --from=azure-vnet /azure-container-networking/cni/azure-$OS-swift.conflist /payload/azure-swift.conflist
COPY --from=azure-vnet /azure-container-networking/cni/azure-$OS-swift-overlay.conflist /payload/azure-swift-overlay.conflist
COPY --from=azure-vnet /azure-container-networking/telemetry/azure-vnet-telemetry.config /payload/azure-vnet-telemetry.config
RUN cd /payload && sha256sum * > sum.txt
RUN gzip --verbose --best --recursive /payload && for f in /payload/*.gz; do mv -- "$f" "${f%%.gz}"; done

# skopeo inspect docker://mcr.microsoft.com/oss/go/microsoft/golang:1.24.1-cbl-mariner2.0 --format "{{.Name}}@{{.Digest}}"
FROM --platform=linux/${ARCH} mcr.microsoft.com/oss/go/microsoft/golang@sha256:82f110263e6110403fbbef97153f7532780b01afd44d5906753ac31a6b1b9e90 AS dropgz
ARG DROPGZ_VERSION
ARG OS
ARG VERSION
RUN go mod download github.com/azure/azure-container-networking/dropgz@$DROPGZ_VERSION
WORKDIR /go/pkg/mod/github.com/azure/azure-container-networking/dropgz\@$DROPGZ_VERSION
COPY --from=compressor /payload/* pkg/embed/fs/
RUN GOOS=$OS CGO_ENABLED=0 go build -a -o /go/bin/dropgz -trimpath -ldflags "-X github.com/Azure/azure-container-networking/dropgz/internal/buildinfo.Version="$VERSION"" -gcflags="-dwarflocationlists=true" main.go

FROM mcr.microsoft.com/windows/nanoserver:${OS_VERSION}
COPY --from=dropgz /go/bin/dropgz dropgz
ENTRYPOINT [ "/dropgz" ]
