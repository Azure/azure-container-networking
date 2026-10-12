ARG ARCH
ARG OS_VERSION
ARG OS

# mcr.microsoft.com/oss/go/microsoft/golang:1.27-azurelinux3.0
FROM --platform=linux/${ARCH} mcr.microsoft.com/oss/go/microsoft/golang:1.27-azurelinux3.0@sha256:6e700813d3afac4e2e02f485eb0b0f5148ad09b2a7113eab1b3ec6ba4e5c92c1 AS go

# mcr.microsoft.com/azurelinux/base/core:3.0
FROM mcr.microsoft.com/azurelinux/base/core:3.0@sha256:7292f2bcf52c3916758f1a45aba6e4dcb93f466027d7786900ea96c2d23c48e2 AS mariner-core

# mcr.microsoft.com/azurelinux/distroless/base:3.0
FROM mcr.microsoft.com/azurelinux/distroless/base:3.0@sha256:431ae1dc2924c41ae54c97c25676990f0c61d6295f42f5d07783c6976f58f1ef AS mariner-distroless

FROM --platform=linux/${ARCH} go AS builder
ARG OS
ARG CNS_AI_ID
ARG CNS_AI_PATH
ARG VERSION
WORKDIR /azure-container-networking
COPY . .
RUN GOOS=$OS CGO_ENABLED=0 go build -a -o /go/bin/azure-cns -ldflags "-s -w -X main.version="$VERSION" -X "$CNS_AI_PATH"="$CNS_AI_ID"" -gcflags="-dwarflocationlists=true" cns/service/*.go

FROM mariner-core AS iptables
RUN tdnf install -y iptables

FROM mariner-distroless AS linux
COPY --from=iptables /usr/sbin/*tables* /usr/sbin/
COPY --from=iptables /usr/lib /usr/lib
COPY --from=builder /go/bin/azure-cns /usr/local/bin/azure-cns
ENTRYPOINT [ "/usr/local/bin/azure-cns" ]
EXPOSE 10090

# mcr.microsoft.com/oss/kubernetes/windows-host-process-containers-base-image:v1.0.0
FROM --platform=windows/${ARCH} mcr.microsoft.com/oss/kubernetes/windows-host-process-containers-base-image:v1.0.0@sha256:b4c9637e032f667c52d1eccfa31ad8c63f1b035e8639f3f48a510536bf34032b as hpc

FROM hpc as windows
COPY --from=builder /go/bin/azure-cns /azure-cns.exe
ENTRYPOINT ["azure-cns.exe"]
EXPOSE 10090
