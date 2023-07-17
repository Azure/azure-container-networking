FROM mcr.microsoft.com/oss/go/microsoft/golang:1.20 AS azure-ipam
ARG VERSION
WORKDIR /azure-ipam
COPY ./azure-ipam .
RUN CGO_ENABLED=0 go build -a -o bin/azure-ipam -trimpath -ldflags "-X main.version="$VERSION"" -gcflags="-dwarflocationlists=true" .

FROM mcr.microsoft.com/oss/go/microsoft/golang:1.20 AS azure-vnet
ARG VERSION
WORKDIR /azure-container-networking
COPY . .
RUN CGO_ENABLED=0 go build -a -o bin/azure-vnet -trimpath -ldflags "-X main.version="$VERSION"" -gcflags="-dwarflocationlists=true" cni/network/plugin/main.go
RUN CGO_ENABLED=0 go build -a -o bin/azure-vnet-telemetry -trimpath -ldflags "-X main.version="$VERSION"" -gcflags="-dwarflocationlists=true" cni/telemetry/service/telemetrymain.go
RUN CGO_ENABLED=0 go build -a -o bin/azure-vnet-ipam -trimpath -ldflags "-X main.version="$VERSION"" -gcflags="-dwarflocationlists=true" cni/ipam/plugin/main.go

FROM mcr.microsoft.com/cbl-mariner/base/core:2.0 AS compressor
ARG OS
WORKDIR /dropgz
COPY dropgz .
COPY --from=azure-ipam /azure-ipam/*.conflist pkg/embed/fs
COPY --from=azure-ipam /azure-ipam/bin/* pkg/embed/fs
COPY --from=azure-vnet /azure-container-networking/cni/azure-$OS.conflist pkg/embed/fs/azure.conflist
COPY --from=azure-vnet /azure-container-networking/cni/azure-$OS-swift.conflist pkg/embed/fs/azure-swift.conflist
COPY --from=azure-vnet /azure-container-networking/cni/azure-$OS-swift-overlay.conflist pkg/embed/fs/azure-swift-overlay.conflist
COPY --from=azure-vnet /azure-container-networking/cni/azure-$OS-swift-overlay-dualstack.conflist pkg/embed/fs/azure-swift-overlay-dualstack.conflist
COPY --from=azure-vnet /azure-container-networking/bin/* pkg/embed/fs
COPY --from=azure-vnet /azure-container-networking/telemetry/azure-vnet-telemetry.config pkg/embed/fs

RUN cd pkg/embed/fs/ && sha256sum * > sum.txt
RUN gzip --verbose --best --recursive pkg/embed/fs && for f in pkg/embed/fs/*.gz; do mv -- "$f" "${f%%.gz}"; done

FROM mcr.microsoft.com/oss/go/microsoft/golang:1.20 AS dropgz
ARG VERSION
WORKDIR /dropgz
COPY --from=compressor /dropgz .
RUN CGO_ENABLED=0 go build -a -o bin/dropgz -trimpath -ldflags "-X github.com/Azure/azure-container-networking/dropgz/internal/buildinfo.Version="$VERSION"" -gcflags="-dwarflocationlists=true" main.go

FROM scratch
COPY --from=dropgz /dropgz/bin/dropgz /dropgz
ENTRYPOINT [ "/dropgz" ]
