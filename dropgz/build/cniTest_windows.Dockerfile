ARG ARCH
ARG OS_VERSION
FROM --platform=linux/${ARCH} mcr.microsoft.com/oss/go/microsoft/golang:1.20 AS azure-vnet
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

FROM --platform=linux/${ARCH} mcr.microsoft.com/oss/go/microsoft/golang:1.20 AS dropgz
ARG VERSION
WORKDIR /dropgz
COPY --from=compressor /dropgz .
RUN GOOS=windows CGO_ENABLED=0 go build -a -o bin/dropgz.exe -trimpath -ldflags "-X github.com/Azure/azure-container-networking/dropgz/internal/buildinfo.Version="$VERSION"" -gcflags="-dwarflocationlists=true" main.go

FROM mcr.microsoft.com/windows/nanoserver:${OS_VERSION}
COPY --from=dropgz /dropgz/bin/dropgz.exe dropgz.exe
ENTRYPOINT [ "dropgz.exe" ]
