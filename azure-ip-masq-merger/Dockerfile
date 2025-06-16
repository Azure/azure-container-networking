ARG ARCH
ARG OS_VERSION
ARG OS

# skopeo inspect docker://mcr.microsoft.com/oss/go/microsoft/golang:1.23.2-azurelinux3.0 --format "{{.Name}}@{{.Digest}}"
FROM --platform=linux/${ARCH} mcr.microsoft.com/oss/go/microsoft/golang@sha256:f1f0cbd464ae4cd9d41176d47f1f9fe16a6965425871f817587314e3a04576ec AS go

# skopeo inspect docker://mcr.microsoft.com/azurelinux/base/core:3.0 --format "{{.Name}}@{{.Digest}}"
FROM --platform=linux/${ARCH} mcr.microsoft.com/azurelinux/base/core@sha256:b46476be0b5c9691ad20f78871819950c01433bdfad81d72c61618f4a6202b25 AS mariner-core

FROM go AS azure-ip-masq-merger
ARG OS
ARG VERSION
WORKDIR /azure-ip-masq-merger
COPY ./azure-ip-masq-merger .
RUN GOOS=$OS CGO_ENABLED=0 go build -a -o /go/bin/ip-masq-merger -trimpath -ldflags "-X main.version="$VERSION"" -gcflags="-dwarflocationlists=true" .

# TODO: Replace with scratch later
FROM --platform=linux/${ARCH} mcr.microsoft.com/azurelinux/base/core@sha256:b46476be0b5c9691ad20f78871819950c01433bdfad81d72c61618f4a6202b25 AS linux
COPY --from=azure-ip-masq-merger /go/bin/ip-masq-merger ip-masq-merger
ENTRYPOINT [ "/ip-masq-merger" ]

# skopeo inspect docker://mcr.microsoft.com/oss/kubernetes/windows-host-process-containers-base-image:v1.0.0 --format "{{.Name}}@{{.Digest}}"
FROM mcr.microsoft.com/oss/kubernetes/windows-host-process-containers-base-image@sha256:b4c9637e032f667c52d1eccfa31ad8c63f1b035e8639f3f48a510536bf34032b as windows
COPY --from=azure-ip-masq-merger /go/bin/ip-masq-merger ip-masq-merger.exe
ENTRYPOINT [ "/ip-masq-merger.exe" ]
