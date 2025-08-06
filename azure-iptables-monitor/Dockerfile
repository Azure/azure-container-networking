ARG ARCH

# mcr.microsoft.com/azurelinux/base/core:3.0
FROM mcr.microsoft.com/azurelinux/base/core@sha256:9948138108a3d69f1dae62104599ac03132225c3b7a5ac57b85a214629c8567d AS mariner-core

# mcr.microsoft.com/azurelinux/distroless/minimal:3.0
FROM mcr.microsoft.com/azurelinux/distroless/minimal@sha256:0801b80a0927309572b9adc99bd1813bc680473175f6e8175cd4124d95dbd50c AS mariner-distroless

# skopeo inspect docker://mcr.microsoft.com/oss/go/microsoft/golang:1.23.2-azurelinux3.0 --format "{{.Name}}@{{.Digest}}"
FROM --platform=linux/${ARCH} mcr.microsoft.com/oss/go/microsoft/golang@sha256:f1f0cbd464ae4cd9d41176d47f1f9fe16a6965425871f817587314e3a04576ec AS go


FROM go AS azure-iptables-monitor
ARG OS
ARG VERSION
WORKDIR /azure-iptables-monitor
COPY ./azure-iptables-monitor .
RUN GOOS=$OS CGO_ENABLED=0 go build -a -o /go/bin/iptables-monitor -trimpath -ldflags "-s -w -X main.version="$VERSION"" -gcflags="-dwarflocationlists=true" .

FROM mariner-core AS iptables
RUN tdnf install -y iptables

FROM mariner-distroless AS linux
COPY --from=iptables /usr/sbin/*tables* /usr/sbin/
COPY --from=iptables /usr/lib /usr/lib
COPY --from=azure-iptables-monitor /go/bin/iptables-monitor azure-iptables-monitor

ENTRYPOINT ["/azure-iptables-monitor"]
