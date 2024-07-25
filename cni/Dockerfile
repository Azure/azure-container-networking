ARG ARCH
ARG DROPGZ_VERSION=v0.0.12
ARG OS_VERSION
ARG OS

FROM --platform=linux/${ARCH} mcr.microsoft.com/oss/go/microsoft/golang:1.21 AS azure-vnet
ARG OS
ARG VERSION
WORKDIR /azure-container-networking
COPY . .
RUN GOOS=$OS CGO_ENABLED=0 go build -a -o /go/bin/azure-vnet -trimpath -ldflags "-X main.version="$VERSION"" -gcflags="-dwarflocationlists=true" cni/network/plugin/main.go
RUN GOOS=$OS CGO_ENABLED=0 go build -a -o /go/bin/azure-vnet-telemetry -trimpath -ldflags "-X main.version="$VERSION"" -gcflags="-dwarflocationlists=true" cni/telemetry/service/telemetrymain.go
RUN GOOS=$OS CGO_ENABLED=0 go build -a -o /go/bin/azure-vnet-ipam -trimpath -ldflags "-X main.version="$VERSION"" -gcflags="-dwarflocationlists=true" cni/ipam/plugin/main.go
RUN GOOS=$OS CGO_ENABLED=0 go build -a -o /go/bin/azure-vnet-stateless -trimpath -ldflags "-X main.version="$VERSION"" -gcflags="-dwarflocationlists=true" cni/network/stateless/main.go

FROM scratch as bins
COPY --from=azure-vnet /go/bin/* /

FROM --platform=linux/${ARCH} mcr.microsoft.com/cbl-mariner/base/core:2.0 AS compressor
ARG OS
WORKDIR /payload
COPY --from=azure-vnet /go/bin/* /payload/
COPY --from=azure-vnet /azure-container-networking/cni/azure-$OS.conflist /payload/azure.conflist
COPY --from=azure-vnet /azure-container-networking/cni/azure-$OS-swift.conflist /payload/azure-swift.conflist
COPY --from=azure-vnet /azure-container-networking/cni/azure-$OS-multitenancy-transparent-vlan.conflist /payload/azure-multitenancy-transparent-vlan.conflist
COPY --from=azure-vnet /azure-container-networking/cni/azure-$OS-swift-overlay.conflist /payload/azure-swift-overlay.conflist
COPY --from=azure-vnet /azure-container-networking/cni/azure-$OS-swift-overlay-dualstack.conflist /payload/azure-swift-overlay-dualstack.conflist
COPY --from=azure-vnet /azure-container-networking/cni/azure-$OS-multitenancy.conflist /payload/azure-multitenancy.conflist
COPY --from=azure-vnet /azure-container-networking/telemetry/azure-vnet-telemetry.config /payload/azure-vnet-telemetry.config
RUN cd /payload && sha256sum * > sum.txt
RUN gzip --verbose --best --recursive /payload && for f in /payload/*.gz; do mv -- "$f" "${f%%.gz}"; done

FROM --platform=linux/${ARCH} mcr.microsoft.com/oss/go/microsoft/golang:1.21 AS dropgz
ARG DROPGZ_VERSION
ARG OS
ARG VERSION
RUN go mod download github.com/azure/azure-container-networking/dropgz@$DROPGZ_VERSION
WORKDIR /go/pkg/mod/github.com/azure/azure-container-networking/dropgz\@$DROPGZ_VERSION
COPY --from=compressor /payload/* pkg/embed/fs/
RUN GOOS=$OS CGO_ENABLED=0 go build -a -o /go/bin/dropgz -trimpath -ldflags "-X github.com/Azure/azure-container-networking/dropgz/internal/buildinfo.Version="$VERSION"" -gcflags="-dwarflocationlists=true" main.go

FROM scratch as linux
COPY --from=dropgz /go/bin/dropgz dropgz
ENTRYPOINT [ "/dropgz" ]

# intermediate for win-ltsc2019
FROM mcr.microsoft.com/windows/nanoserver@sha256:736f8845da1c7bbc9510b419abddb870230485e74644a96e0001a21e1ca4c172 as ltsc2019

# intermediate for win-ltsc2022
FROM mcr.microsoft.com/windows/nanoserver@sha256:cc9f6a1334190c1f18a905f56d285132e562a9a1ce18ff9476cfaf95194aa0b7 as ltsc2022

# intermediate for win-ltsc2025
FROM mcr.microsoft.com/windows/nanoserver/insider@sha256:67e0ab7f3a79cd73be4a18bae24659c03b294aed0dbeaa624feb3810931f0bd2 as ltsc2025

FROM ${OS_VERSION} as windows
COPY --from=dropgz /go/bin/dropgz dropgz.exe
ENTRYPOINT [ "/dropgz.exe" ]
