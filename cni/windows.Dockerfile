ARG ARCH
ARG DROPGZ_VERSION=v0.0.12
ARG OS
ARG OS_VERSION

FROM --platform=linux/${ARCH} mcr.microsoft.com/oss/go/microsoft/golang:1.23 AS azure-vnet
ARG OS
ARG VERSION
ARG CNI_AI_PATH
ARG CNI_AI_ID
WORKDIR /azure-container-networking
COPY . .
RUN GOOS=$OS CGO_ENABLED=0 go build -a -o /go/bin/azure-vnet -trimpath -ldflags "-X main.version="$VERSION"" -gcflags="-dwarflocationlists=true" cni/network/plugin/main.go
RUN GOOS=$OS CGO_ENABLED=0 go build -a -o /go/bin/azure-vnet-telemetry -trimpath -ldflags "-X main.version="$VERSION" -X "$CNI_AI_PATH"="$CNI_AI_ID"" -gcflags="-dwarflocationlists=true" cni/telemetry/service/telemetrymain.go
RUN GOOS=$OS CGO_ENABLED=0 go build -a -o /go/bin/azure-vnet-ipam -trimpath -ldflags "-X main.version="$VERSION"" -gcflags="-dwarflocationlists=true" cni/ipam/plugin/main.go
RUN GOOS=$OS CGO_ENABLED=0 go build -a -o /go/bin/azure-vnet-stateless -trimpath -ldflags "-X main.version="$VERSION"" -gcflags="-dwarflocationlists=true" cni/network/stateless/main.go

FROM --platform=linux/${ARCH} mcr.microsoft.com/cbl-mariner/base/core:2.0 AS compressor
ARG OS
WORKDIR /payload
COPY --from=azure-vnet /go/bin/* /payload/
COPY --from=azure-vnet /azure-container-networking/cni/azure-$OS.conflist /payload/azure.conflist
COPY --from=azure-vnet /azure-container-networking/cni/azure-$OS-swift.conflist /payload/azure-swift.conflist
COPY --from=azure-vnet /azure-container-networking/cni/azure-$OS-swift-overlay.conflist /payload/azure-swift-overlay.conflist
COPY --from=azure-vnet /azure-container-networking/cni/azure-$OS-swift-overlay-dualstack.conflist /payload/azure-swift-overlay-dualstack.conflist
COPY --from=azure-vnet /azure-container-networking/cni/azure-$OS-multitenancy.conflist /payload/azure-multitenancy.conflist
COPY --from=azure-vnet /azure-container-networking/telemetry/azure-vnet-telemetry.config /payload/azure-vnet-telemetry.config
RUN cd /payload && sha256sum * > sum.txt
RUN gzip --verbose --best --recursive /payload && for f in /payload/*.gz; do mv -- "$f" "${f%%.gz}"; done

FROM --platform=linux/${ARCH} mcr.microsoft.com/oss/go/microsoft/golang:1.23 AS dropgz
ARG DROPGZ_VERSION
ARG OS
ARG VERSION
RUN go mod download github.com/azure/azure-container-networking/dropgz@$DROPGZ_VERSION
WORKDIR /go/pkg/mod/github.com/azure/azure-container-networking/dropgz\@$DROPGZ_VERSION
COPY --from=compressor /payload/* pkg/embed/fs/
RUN GOOS=$OS CGO_ENABLED=0 go build -a -o /go/bin/dropgz -trimpath -ldflags "-X github.com/Azure/azure-container-networking/dropgz/internal/buildinfo.Version="$VERSION"" -gcflags="-dwarflocationlists=true" main.go

# skopeo inspect --override-os windows docker://mcr.microsoft.com/windows/nanoserver:ltsc2019 --format "{{.Name}}@{{.Digest}}"
FROM mcr.microsoft.com/windows/nanoserver@sha256:d96a6c2108e1449c872cc2b224a9b3444fa1415b4d6947280aba2d2bb3bd0025 AS ltsc2019

# skopeo inspect --override-os windows docker://mcr.microsoft.com/windows/nanoserver:ltsc2022 --format "{{.Name}}@{{.Digest}}"
FROM mcr.microsoft.com/windows/nanoserver@sha256:55474b390b2d25e98450356b381d6dc422e15fb79808970f724c0f34df0b217e AS ltsc2022

# skopeo inspect --override-os windows docker://mcr.microsoft.com/windows/nanoserver:ltsc2025 --format "{{.Name}}@{{.Digest}}" ## 2025 isn't tagged yet
FROM mcr.microsoft.com/windows/nanoserver@sha256:d584aae93e84d61c8a3280ed3a5d5a6d397c0214a2902acadb8b17b0b00c70e8 AS ltsc2025

FROM ${OS_VERSION} AS windows
COPY --from=dropgz /go/bin/dropgz dropgz
ENTRYPOINT [ "/dropgz" ]
