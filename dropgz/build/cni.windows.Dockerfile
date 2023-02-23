FROM mcr.microsoft.com/windows/servercore:ltsc2019 as Builder

# Run as admin
USER ContainerAdministrator

SHELL ["powershell", "-command"]

RUN Write-Host $($env:VERSION)

## build azure cni for windows
WORKDIR /azure-container-networking/cni/network/plugin/
RUN go build -a -o azure-vnet.exe -ldflags "-X main.version=%VERSION%"

FROM mcr.microsoft.com/cbl-mariner/base/core:2.0 AS compressor
ARG OS
WORKDIR /dropgz
COPY dropgz .
COPY --from=azure-vnet /azure-container-networking/cni/azure-$OS-swift-multitenancy.conflist pkg/embed/fs/azure-swift-multitenancy.conflist
COPY --from=azure-vnet /azure-container-networking/azure-vnet pkg/embed/fs
RUN cd pkg/embed/fs/ && sha256sum * > sum.txt
RUN gzip --verbose --best --recursive pkg/embed/fs && for f in pkg/embed/fs/*.gz; do mv -- "$f" "${f%%.gz}"; done

FROM mcr.microsoft.com/oss/go/microsoft/golang:1.19 AS dropgz
ARG VERSION
WORKDIR /dropgz
COPY --from=compressor /dropgz .
RUN CGO_ENABLED=0 go build -a -o bin/dropgz -trimpath -ldflags "-X github.com/Azure/azure-container-networking/dropgz/internal/buildinfo.Version="$VERSION"" -gcflags="-dwarflocationlists=true" main.go

FROM scratch
COPY --from=dropgz /dropgz/bin/dropgz /dropgz
ENTRYPOINT [ "/dropgz" ]