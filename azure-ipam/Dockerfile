ARG ARCH
ARG DROPGZ_VERSION=v0.0.12
ARG OS_VERSION
ARG OS

# mcr.microsoft.com/oss/go/microsoft/golang:1.27-azurelinux3.0
FROM --platform=linux/${ARCH} mcr.microsoft.com/oss/go/microsoft/golang:1.27-azurelinux3.0@sha256:7698ebf51921051df706c4a050c3d69bdd5b6f48b8c6f6166429f22f18d1ae8a AS go

# mcr.microsoft.com/azurelinux/base/core:3.0
FROM --platform=linux/${ARCH} mcr.microsoft.com/azurelinux/base/core:3.0@sha256:7292f2bcf52c3916758f1a45aba6e4dcb93f466027d7786900ea96c2d23c48e2 AS mariner-core

# mcr.microsoft.com/azurelinux/distroless/base:3.0
FROM --platform=linux/${ARCH} mcr.microsoft.com/azurelinux/distroless/base:3.0@sha256:431ae1dc2924c41ae54c97c25676990f0c61d6295f42f5d07783c6976f58f1ef AS mariner-distroless

FROM go AS azure-ipam
ARG OS
ARG VERSION
WORKDIR /azure-ipam
COPY ./azure-ipam .
RUN GOOS=$OS CGO_ENABLED=0 go build -a -o /go/bin/azure-ipam -trimpath -ldflags "-s -w -X main.version="$VERSION" -X github.com/Azure/azure-container-networking/azure-ipam/internal/buildinfo.Version="$VERSION"" -gcflags="-dwarflocationlists=true" .

FROM mariner-core AS compressor
ARG OS
WORKDIR /payload
COPY --from=azure-ipam /go/bin/* /payload
COPY --from=azure-ipam /azure-ipam/*.conflist /payload
RUN cd /payload && sha256sum * > sum.txt
RUN gzip --verbose --best --recursive /payload && for f in /payload/*.gz; do mv -- "$f" "${f%%.gz}"; done

FROM go AS dropgz
ARG DROPGZ_VERSION
ARG OS
ARG VERSION
RUN go mod download github.com/azure/azure-container-networking/dropgz@$DROPGZ_VERSION
WORKDIR /go/pkg/mod/github.com/azure/azure-container-networking/dropgz\@$DROPGZ_VERSION
COPY --from=compressor /payload/* pkg/embed/fs/
RUN GOOS=$OS CGO_ENABLED=0 go build -a -o /go/bin/dropgz -trimpath -ldflags "-s -w -X github.com/Azure/azure-container-networking/dropgz/internal/buildinfo.Version="$VERSION"" -gcflags="-dwarflocationlists=true" main.go

FROM mariner-distroless AS linux
COPY --from=dropgz /go/bin/dropgz /dropgz
ENTRYPOINT [ "/dropgz" ]

# mcr.microsoft.com/oss/kubernetes/windows-host-process-containers-base-image:v1.0.0
FROM --platform=windows/${ARCH} mcr.microsoft.com/oss/kubernetes/windows-host-process-containers-base-image:v1.0.0@sha256:b4c9637e032f667c52d1eccfa31ad8c63f1b035e8639f3f48a510536bf34032b AS hpc
FROM hpc AS windows
COPY --from=azure-ipam /go/bin/azure-ipam /Windows/System32/azure-ipam.exe
COPY --from=azure-ipam /azure-ipam/azilium.conflist /Windows/System32/azilium.conflist
ENTRYPOINT [ "powershell.exe" ]
