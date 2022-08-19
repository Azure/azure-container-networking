FROM mcr.microsoft.com/oss/cilium/cilium:1.12.1.1 as cilium

FROM mcr.microsoft.com/oss/go/microsoft/golang:1.18 AS azure-ipam
ARG VERSION
WORKDIR /azure-ipam
COPY ./azure-ipam .
RUN CGO_ENABLED=0 go build -a -o bin/azure-ipam -trimpath -ldflags "-X main.version="$VERSION"" -gcflags="-dwarflocationlists=true" .

FROM mcr.microsoft.com/oss/go/microsoft/golang:1.18 AS azure-vnet
ARG VERSION
ARG OS
ARG ARCH
WORKDIR /azure-container-networking
COPY . .
RUN curl -LO https://github.com/Azure/azure-container-networking/releases/download/v1.4.29/azure-vnet-cni-swift-$OS-$ARCH-v1.4.29.tgz && tar -xvf azure-vnet-cni-swift-$OS-$ARCH-v1.4.29.tgz

FROM mcr.microsoft.com/cbl-mariner/base/core:2.0 AS compressor
ARG OS
WORKDIR /dropgz
COPY dropgz .
COPY --from=azure-ipam /azure-ipam/*.conflist pkg/embed/fs
COPY --from=azure-ipam /azure-ipam/bin/* pkg/embed/fs
COPY --from=azure-vnet /azure-container-networking/cni/azure-$OS-swift.conflist pkg/embed/fs/azure-swift.conflist
COPY --from=azure-vnet /azure-container-networking/azure-vnet pkg/embed/fs
COPY --from=azure-vnet /azure-container-networking/azure-vnet-telemetry pkg/embed/fs
COPY --from=azure-vnet /azure-container-networking/azure-vnet-ipam pkg/embed/fs
COPY --from=cilium /opt/cni/bin/cilium-cni pkg/embed/fs
RUN cd pkg/embed/fs/ && sha256sum * > sum.txt
RUN gzip --verbose --best --recursive pkg/embed/fs && for f in pkg/embed/fs/*.gz; do mv -- "$f" "${f%%.gz}"; done

FROM mcr.microsoft.com/oss/go/microsoft/golang:1.18 AS dropgz
ARG VERSION
WORKDIR /dropgz
COPY --from=compressor /dropgz .
RUN CGO_ENABLED=0 go build -a -o bin/dropgz -trimpath -ldflags "-X github.com/Azure/azure-container-networking/dropgz/internal/buildinfo.Version="$VERSION"" -gcflags="-dwarflocationlists=true" main.go

FROM scratch
COPY --from=dropgz /dropgz/bin/dropgz /dropgz
ENTRYPOINT [ "/dropgz" ]
