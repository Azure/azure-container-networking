ARG ARCH
ARG DROPGZ_VERSION=v0.0.12
ARG OS_VERSION
ARG OS

# skopeo inspect docker://mcr.microsoft.com/oss/go/microsoft/golang:1.23.2-cbl-mariner2.0 --format "{{.Name}}@{{.Digest}}"
FROM --platform=linux/${ARCH} mcr.microsoft.com/oss/go/microsoft/golang@sha256:f8613198423d5cb702961f1547f9cb061f8da1c6ca9ce8da4824eb47db663cd7 AS go

# skopeo inspect docker://mcr.microsoft.com/cbl-mariner/base/core:2.0 --format "{{.Name}}@{{.Digest}}"
FROM --platform=linux/${ARCH} mcr.microsoft.com/cbl-mariner/base/core@sha256:a490e0b0869dc570ae29782c2bc17643aaaad1be102aca83ce0b96e0d0d2d328 AS mariner-core

FROM go AS azure-vnet
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

FROM mariner-core AS compressor
ARG OS
WORKDIR /payload
COPY --from=azure-vnet /go/bin/* /payload/
COPY --from=azure-vnet /azure-container-networking/cni/azure-$OS.conflist /payload/azure.conflist
COPY --from=azure-vnet /azure-container-networking/cni/azure-$OS-swift.conflist /payload/azure-swift.conflist
COPY --from=azure-vnet /azure-container-networking/cni/azure-linux-multitenancy-transparent-vlan.conflist /payload/azure-multitenancy-transparent-vlan.conflist
COPY --from=azure-vnet /azure-container-networking/cni/azure-$OS-swift-overlay.conflist /payload/azure-swift-overlay.conflist
COPY --from=azure-vnet /azure-container-networking/cni/azure-$OS-swift-overlay-dualstack.conflist /payload/azure-swift-overlay-dualstack.conflist
COPY --from=azure-vnet /azure-container-networking/cni/azure-$OS-multitenancy.conflist /payload/azure-multitenancy.conflist
COPY --from=azure-vnet /azure-container-networking/telemetry/azure-vnet-telemetry.config /payload/azure-vnet-telemetry.config
RUN cd /payload && sha256sum * > sum.txt
RUN gzip --verbose --best --recursive /payload && for f in /payload/*.gz; do mv -- "$f" "${f%%.gz}"; done

FROM go AS dropgz
ARG DROPGZ_VERSION
ARG OS
ARG VERSION
RUN go mod download github.com/azure/azure-container-networking/dropgz@$DROPGZ_VERSION
WORKDIR /go/pkg/mod/github.com/azure/azure-container-networking/dropgz\@$DROPGZ_VERSION
COPY --from=compressor /payload/* pkg/embed/fs/
RUN GOOS=$OS CGO_ENABLED=0 go build -a -o /go/bin/dropgz -trimpath -ldflags "-X github.com/Azure/azure-container-networking/dropgz/internal/buildinfo.Version="$VERSION"" -gcflags="-dwarflocationlists=true" main.go

FROM scratch AS bins
COPY --from=azure-vnet /go/bin/* /

FROM scratch AS linux
COPY --from=dropgz /go/bin/dropgz dropgz
ENTRYPOINT [ "/dropgz" ]

# skopeo inspect docker://mcr.microsoft.com/oss/kubernetes/windows-host-process-containers-base-image:v1.0.0 --format "{{.Name}}@{{.Digest}}"
FROM mcr.microsoft.com/oss/kubernetes/windows-host-process-containers-base-image@sha256:b4c9637e032f667c52d1eccfa31ad8c63f1b035e8639f3f48a510536bf34032b as windows
COPY --from=dropgz /go/bin/dropgz dropgz.exe
ENTRYPOINT [ "/dropgz.exe" ]
