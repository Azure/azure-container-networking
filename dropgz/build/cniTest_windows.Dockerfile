ARG ARCH
ARG OS_VERSION

# skopeo inspect docker://mcr.microsoft.com/oss/go/microsoft/golang:1.24.1-cbl-mariner2.0 --format "{{.Name}}@{{.Digest}}"
FROM --platform=linux/${ARCH} mcr.microsoft.com/oss/go/microsoft/golang@sha256:82f110263e6110403fbbef97153f7532780b01afd44d5906753ac31a6b1b9e90 AS azure-vnet
ARG VERSION
WORKDIR /azure-container-networking
COPY . .
RUN GOOS=windows CGO_ENABLED=0 go build -a -o azure-vnet.exe -trimpath -ldflags "-X main.version="$VERSION"" -gcflags="-dwarflocationlists=true" cni/network/plugin/main.go
RUN GOOS=windows CGO_ENABLED=0 go build -a -o azure-vnet-telemetry.exe -trimpath -ldflags "-X main.version="$VERSION"" -gcflags="-dwarflocationlists=true" cni/telemetry/service/telemetrymain.go
RUN GOOS=windows CGO_ENABLED=0 go build -a -o azure-vnet-ipam.exe -trimpath -ldflags "-X main.version="$VERSION"" -gcflags="-dwarflocationlists=true" cni/ipam/plugin/main.go

FROM --platform=linux/${ARCH} mcr.microsoft.com/cbl-mariner/base/core:2.0 AS compressor
ARG OS
WORKDIR /dropgz
COPY dropgz .
COPY --from=azure-vnet /azure-container-networking/azure-vnet.exe pkg/embed/fs
COPY --from=azure-vnet /azure-container-networking/azure-vnet-telemetry.exe pkg/embed/fs
COPY --from=azure-vnet /azure-container-networking/azure-vnet-ipam.exe pkg/embed/fs
COPY --from=azure-vnet /azure-container-networking/telemetry/azure-vnet-telemetry.config pkg/embed/fs
RUN cd pkg/embed/fs/ && sha256sum * > sum.txt
RUN gzip --verbose --best --recursive pkg/embed/fs && for f in pkg/embed/fs/*.gz; do mv -- "$f" "${f%%.gz}"; done

# skopeo inspect docker://mcr.microsoft.com/oss/go/microsoft/golang:1.24.1-cbl-mariner2.0 --format "{{.Name}}@{{.Digest}}"
FROM --platform=linux/${ARCH} mcr.microsoft.com/oss/go/microsoft/golang@sha256:82f110263e6110403fbbef97153f7532780b01afd44d5906753ac31a6b1b9e90 AS dropgz
ARG VERSION
WORKDIR /dropgz
COPY --from=compressor /dropgz .
RUN GOOS=windows CGO_ENABLED=0 go build -a -o bin/dropgz.exe -trimpath -ldflags "-X github.com/Azure/azure-container-networking/dropgz/internal/buildinfo.Version="$VERSION"" -gcflags="-dwarflocationlists=true" main.go

FROM mcr.microsoft.com/windows/nanoserver:${OS_VERSION}
COPY --from=dropgz /dropgz/bin/dropgz.exe dropgz.exe
ENTRYPOINT [ "dropgz.exe" ]
