ARG ARCH
ARG OS_VERSION
ARG OS

# skopeo inspect docker://mcr.microsoft.com/oss/go/microsoft/golang:1.23.2-cbl-mariner2.0 --format "{{.Name}}@{{.Digest}}"
FROM --platform=linux/${ARCH} mcr.microsoft.com/oss/go/microsoft/golang@sha256:f8613198423d5cb702961f1547f9cb061f8da1c6ca9ce8da4824eb47db663cd7 AS go

# skopeo inspect docker://mcr.microsoft.com/cbl-mariner/base/core:2.0 --format "{{.Name}}@{{.Digest}}"
FROM mcr.microsoft.com/cbl-mariner/base/core@sha256:a490e0b0869dc570ae29782c2bc17643aaaad1be102aca83ce0b96e0d0d2d328 AS mariner-core

# skopeo inspect docker://mcr.microsoft.com/cbl-mariner/distroless/minimal:2.0 --format "{{.Name}}@{{.Digest}}"
FROM mcr.microsoft.com/cbl-mariner/distroless/minimal@sha256:d28cbaa097167b4f5fdea02aac5404e3c9ec6c37499df1e115765e38b0a21660 AS mariner-distroless

FROM --platform=linux/${ARCH} go AS builder
ARG OS
ARG CNS_AI_ID
ARG CNS_AI_PATH
ARG VERSION
WORKDIR /azure-container-networking
COPY . .
RUN GOOS=$OS CGO_ENABLED=0 go build -a -o /go/bin/azure-cns -ldflags "-X main.version="$VERSION" -X "$CNS_AI_PATH"="$CNS_AI_ID"" -gcflags="-dwarflocationlists=true" cns/service/*.go

FROM mariner-core AS iptables
RUN tdnf install -y iptables

FROM mariner-distroless AS linux
COPY --from=iptables /usr/sbin/*tables* /usr/sbin/
COPY --from=iptables /usr/lib /usr/lib
COPY --from=builder /go/bin/azure-cns /usr/local/bin/azure-cns
ENTRYPOINT [ "/usr/local/bin/azure-cns" ]
EXPOSE 10090

# skopeo inspect --override-os windows docker://mcr.microsoft.com/windows/nanoserver:ltsc2019 --format "{{.Name}}@{{.Digest}}"
FROM mcr.microsoft.com/windows/nanoserver@sha256:7f6649348a11655e3576463fd6d55c29248f97405f8e643cab2409009339f520 AS ltsc2019

# skopeo inspect --override-os windows docker://mcr.microsoft.com/windows/nanoserver:ltsc2022 --format "{{.Name}}@{{.Digest}}"
FROM mcr.microsoft.com/windows/nanoserver@sha256:244113e50a678a25a63930780f9ccafd22e1a37aa9e3d93295e4cebf0f170a11 AS ltsc2022

# skopeo inspect --override-os windows docker://mcr.microsoft.com/windows/nanoserver:ltsc2025 --format "{{.Name}}@{{.Digest}}" ## 2025 isn't tagged yet
FROM mcr.microsoft.com/windows/nanoserver/insider@sha256:67e0ab7f3a79cd73be4a18bae24659c03b294aed0dbeaa624feb3810931f0bd2 AS ltsc2025

FROM ${OS_VERSION} AS windows
COPY --from=builder /azure-container-networking/cns/kubeconfigtemplate.yaml kubeconfigtemplate.yaml
COPY --from=builder /azure-container-networking/npm/examples/windows/setkubeconfigpath.ps1 setkubeconfigpath.ps1
COPY --from=builder /go/bin/azure-cns /azure-cns.exe
ENTRYPOINT ["azure-cns.exe"]
EXPOSE 10090
