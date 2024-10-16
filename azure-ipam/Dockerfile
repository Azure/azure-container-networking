ARG ARCH
ARG DROPGZ_VERSION=v0.0.12
ARG OS_VERSION
ARG OS

# skopeo inspect docker://mcr.microsoft.com/oss/go/microsoft/golang:1.23.2-cbl-mariner2.0 --format "{{.Name}}@{{.Digest}}"
FROM --platform=linux/${ARCH} mcr.microsoft.com/oss/go/microsoft/golang@sha256:f8613198423d5cb702961f1547f9cb061f8da1c6ca9ce8da4824eb47db663cd7 AS go

# skopeo inspect docker://mcr.microsoft.com/cbl-mariner/base/core:2.0 --format "{{.Name}}@{{.Digest}}"
FROM --platform=linux/${ARCH} mcr.microsoft.com/cbl-mariner/base/core@sha256:a490e0b0869dc570ae29782c2bc17643aaaad1be102aca83ce0b96e0d0d2d328 AS mariner-core

FROM go AS azure-ipam
ARG OS
ARG VERSION
WORKDIR /azure-ipam
COPY ./azure-ipam .
RUN GOOS=$OS CGO_ENABLED=0 go build -a -o /go/bin/azure-ipam -trimpath -ldflags "-X main.version="$VERSION"" -gcflags="-dwarflocationlists=true" .

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
RUN GOOS=$OS CGO_ENABLED=0 go build -a -o /go/bin/dropgz -trimpath -ldflags "-X github.com/Azure/azure-container-networking/dropgz/internal/buildinfo.Version="$VERSION"" -gcflags="-dwarflocationlists=true" main.go

FROM scratch AS linux
COPY --from=dropgz /go/bin/dropgz dropgz
ENTRYPOINT [ "/dropgz" ]

# skopeo inspect --override-os windows docker://mcr.microsoft.com/windows/nanoserver:ltsc2019 --format "{{.Name}}@{{.Digest}}"
FROM mcr.microsoft.com/windows/nanoserver@sha256:7f6649348a11655e3576463fd6d55c29248f97405f8e643cab2409009339f520 AS ltsc2019

# skopeo inspect --override-os windows docker://mcr.microsoft.com/windows/nanoserver:ltsc2022 --format "{{.Name}}@{{.Digest}}"
FROM mcr.microsoft.com/windows/nanoserver@sha256:244113e50a678a25a63930780f9ccafd22e1a37aa9e3d93295e4cebf0f170a11 AS ltsc2022

# skopeo inspect --override-os windows docker://mcr.microsoft.com/windows/nanoserver:ltsc2025 --format "{{.Name}}@{{.Digest}}" ## 2025 isn't tagged yet
FROM mcr.microsoft.com/windows/nanoserver/insider@sha256:67e0ab7f3a79cd73be4a18bae24659c03b294aed0dbeaa624feb3810931f0bd2 AS ltsc2025

FROM ${OS_VERSION} AS windows
COPY --from=dropgz /go/bin/dropgz dropgz.exe
ENTRYPOINT [ "/dropgz.exe" ]
