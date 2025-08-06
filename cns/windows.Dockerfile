ARG OS_VERSION
FROM --platform=linux/amd64 mcr.microsoft.com/oss/go/microsoft/golang:1.23 AS builder
ARG VERSION
ARG CNS_AI_PATH
ARG CNS_AI_ID
WORKDIR /usr/local/src
COPY . .
RUN GOOS=windows CGO_ENABLED=0 go build -a -o /usr/local/bin/azure-cns.exe -ldflags "-X main.version="$VERSION" -X "$CNS_AI_PATH"="$CNS_AI_ID"" -gcflags="-dwarflocationlists=true" cns/service/*.go

# skopeo inspect --override-os windows docker://mcr.microsoft.com/windows/nanoserver:ltsc2019 --format "{{.Name}}@{{.Digest}}"
FROM mcr.microsoft.com/windows/nanoserver@sha256:c357ed591af7b24f2d4a12b1947da5e6ebe559d89f41471f6928902c7cda7206 AS ltsc2019

# skopeo inspect --override-os windows docker://mcr.microsoft.com/windows/nanoserver:ltsc2022 --format "{{.Name}}@{{.Digest}}"
FROM mcr.microsoft.com/windows/nanoserver@sha256:9a57174ce85e979529e4f0cd58dff2e837b65fc7832b7585b4882f6cce0e255d AS ltsc2022

# skopeo inspect --override-os windows docker://mcr.microsoft.com/windows/nanoserver:ltsc2025 --format "{{.Name}}@{{.Digest}}" ## 2025 isn't tagged yet
FROM mcr.microsoft.com/windows/nanoserver@sha256:83bd374c854fddd084ea9bf4864dd00b3810c34ea172892263958c2d738d0fdd AS ltsc2025

FROM ${OS_VERSION} AS windows
COPY --from=builder /usr/local/src/cns/kubeconfigtemplate.yaml kubeconfigtemplate.yaml
COPY --from=builder /usr/local/src/npm/examples/windows/setkubeconfigpath.ps1 setkubeconfigpath.ps1
COPY --from=builder /usr/local/bin/azure-cns.exe azure-cns.exe
ENTRYPOINT ["azure-cns.exe"]
EXPOSE 10090
