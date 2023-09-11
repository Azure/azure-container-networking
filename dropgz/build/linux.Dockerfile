FROM mcr.microsoft.com/cbl-mariner/base/core:2.0 AS tar
RUN tdnf install -y tar
RUN tdnf upgrade -y && tdnf install -y ca-certificates

FROM tar AS azure-ipam
ARG AZIPAM_VERSION=v0.0.3
ARG VERSION
ARG OS
ARG ARCH
WORKDIR /azure-ipam
COPY ./azure-ipam .
RUN curl -LO --cacert /etc/ssl/certs/ca-certificates.crt https://github.com/Azure/azure-container-networking/releases/download/azure-ipam%2F$AZIPAM_VERSION/azure-ipam-$OS-$ARCH-$AZIPAM_VERSION.tgz && tar -xvf azure-ipam-$OS-$ARCH-$AZIPAM_VERSION.tgz

FROM tar AS azure-vnet
ARG AZCNI_VERSION=v1.5.11
ARG VERSION
ARG OS
ARG ARCH
WORKDIR /azure-container-networking
COPY . .
RUN curl -LO --cacert /etc/ssl/certs/ca-certificates.crt https://github.com/Azure/azure-container-networking/releases/download/$AZCNI_VERSION/azure-vnet-cni-swift-$OS-$ARCH-$AZCNI_VERSION.tgz && tar -xvf azure-vnet-cni-swift-$OS-$ARCH-$AZCNI_VERSION.tgz

FROM mcr.microsoft.com/cbl-mariner/base/core:2.0 AS compressor
ARG OS
WORKDIR /dropgz
COPY dropgz .
COPY --from=azure-ipam /azure-ipam/*.conflist pkg/embed/fs
COPY --from=azure-ipam /azure-ipam/azure-ipam pkg/embed/fs
COPY --from=azure-vnet /azure-container-networking/cni/azure-$OS.conflist pkg/embed/fs/azure.conflist
COPY --from=azure-vnet /azure-container-networking/cni/azure-$OS-swift.conflist pkg/embed/fs/azure-swift.conflist
COPY --from=azure-vnet /azure-container-networking/cni/azure-$OS-swift-overlay.conflist pkg/embed/fs/azure-swift-overlay.conflist
COPY --from=azure-vnet /azure-container-networking/cni/azure-$OS-swift-overlay-dualstack.conflist pkg/embed/fs/azure-swift-overlay-dualstack.conflist
COPY --from=azure-vnet /azure-container-networking/azure-vnet pkg/embed/fs
COPY --from=azure-vnet /azure-container-networking/azure-vnet-telemetry pkg/embed/fs
COPY --from=azure-vnet /azure-container-networking/azure-vnet-ipam pkg/embed/fs
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
