ARG ARCH
ARG OS_VERSION
ARG OS

FROM --platform=linux/${ARCH} mcr.microsoft.com/oss/go/microsoft/golang:1.21 AS builder
ARG OS
ARG CNS_AI_ID
ARG CNS_AI_PATH
ARG VERSION
WORKDIR /azure-container-networking
COPY . .
RUN GOOS=$OS CGO_ENABLED=0 go build -a -o /go/bin/azure-cns -ldflags "-X main.version="$VERSION" -X "$CNS_AI_PATH"="$CNS_AI_ID"" -gcflags="-dwarflocationlists=true" cns/service/*.go

FROM mcr.microsoft.com/cbl-mariner/base/core:1.0 AS certs
RUN tdnf upgrade -y && tdnf install -y ca-certificates

FROM mcr.microsoft.com/cbl-mariner/base/core:2.0 as linux
RUN tdnf install -y iptables
COPY --from=builder /etc/passwd /etc/passwd
COPY --from=builder /etc/group /etc/group
COPY --from=builder /go/bin/azure-cns /azure-cns
RUN ln -s /azure-cns /usr/local/bin/azure-cns
COPY --from=certs /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/ca-certificates.crt
ENTRYPOINT [ "/azure-cns" ]
EXPOSE 10090

FROM mcr.microsoft.com/windows/servercore:${OS_VERSION} as windows
COPY --from=builder /azure-container-networking/cns/kubeconfigtemplate.yaml kubeconfigtemplate.yaml
COPY --from=builder /azure-container-networking/npm/examples/windows/setkubeconfigpath.ps1 setkubeconfigpath.ps1
COPY --from=builder /go/bin/azure-cns /azure-cns.exe
ENTRYPOINT ["azure-cns.exe"]
EXPOSE 10090
