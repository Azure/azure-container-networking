# !! AUTOGENERATED - DO NOT EDIT !!
# SOURCE: cns/Dockerfile.tmpl
ARG ARCH
ARG OS_VERSION
ARG OS

# mcr.microsoft.com/oss/go/microsoft/golang:1.23-cbl-mariner2.0
FROM --platform=linux/${ARCH} mcr.microsoft.com/oss/go/microsoft/golang@sha256:b06999cae63b9b6f43bcb16bd16bcbedae847684515317e15607a601ed108030 AS go

# mcr.microsoft.com/cbl-mariner/base/core:2.0
FROM mcr.microsoft.com/cbl-mariner/base/core@sha256:961bfedbbbdc0da51bc664f51d959da292eced1ad46c3bf674aba43b9be8c703 AS mariner-core

# mcr.microsoft.com/cbl-mariner/distroless/minimal:2.0
FROM mcr.microsoft.com/cbl-mariner/distroless/minimal@sha256:7778a86d86947d5f64c1280a7ee0cf36c6c6d76b5749dd782fbcc14f113961bf AS mariner-distroless

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

# mcr.microsoft.com/oss/kubernetes/windows-host-process-containers-base-image:v1.0.0
FROM --platform=windows/${ARCH} mcr.microsoft.com/oss/kubernetes/windows-host-process-containers-base-image@sha256:b4c9637e032f667c52d1eccfa31ad8c63f1b035e8639f3f48a510536bf34032b as hpc

FROM hpc as windows
COPY --from=builder /azure-container-networking/cns/kubeconfigtemplate.yaml kubeconfigtemplate.yaml
COPY --from=builder /azure-container-networking/npm/examples/windows/setkubeconfigpath.ps1 setkubeconfigpath.ps1
COPY --from=builder /go/bin/azure-cns /azure-cns.exe
ENTRYPOINT ["azure-cns.exe"]
EXPOSE 10090
