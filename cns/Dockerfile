ARG ARCH
ARG OS_VERSION
ARG OS

FROM --platform=linux/${ARCH} mcr.microsoft.com/oss/go/microsoft/golang:1.22-cbl-mariner2.0  AS builder
ARG OS
ARG CNS_AI_ID
ARG CNS_AI_PATH
ARG VERSION
WORKDIR /azure-container-networking
COPY . .
RUN GOOS=$OS CGO_ENABLED=0 go build -a -o /go/bin/azure-cns -ldflags "-X main.version="$VERSION" -X "$CNS_AI_PATH"="$CNS_AI_ID"" -gcflags="-dwarflocationlists=true" cns/service/*.go

FROM mcr.microsoft.com/cbl-mariner/base/core:2.0 as iptables
RUN tdnf install -y iptables

FROM mcr.microsoft.com/cbl-mariner/distroless/minimal:2.0 as linux
COPY --from=iptables /usr/sbin/*tables* /usr/sbin/
COPY --from=iptables /usr/lib /usr/lib
COPY --from=builder /go/bin/azure-cns /usr/local/bin/azure-cns
ENTRYPOINT [ "/usr/local/bin/azure-cns" ]
EXPOSE 10090


# intermediate for win-ltsc2019
FROM mcr.microsoft.com/windows/servercore@sha256:6fdf140282a2f809dae9b13fe441635867f0a27c33a438771673b8da8f3348a4 as ltsc2019

# intermediate for win-ltsc2022
FROM mcr.microsoft.com/windows/servercore@sha256:45952938708fbde6ec0b5b94de68bcdec3f8c838be018536b1e9e5bd95e6b943 as ltsc2022

FROM ${OS_VERSION} as windows
COPY --from=builder /azure-container-networking/cns/kubeconfigtemplate.yaml kubeconfigtemplate.yaml
COPY --from=builder /azure-container-networking/npm/examples/windows/setkubeconfigpath.ps1 setkubeconfigpath.ps1
COPY --from=builder /go/bin/azure-cns /azure-cns.exe
ENTRYPOINT ["azure-cns.exe"]
EXPOSE 10090
