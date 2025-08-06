ARG ARCH

# mcr.microsoft.com/azurelinux/base/core:3.0
FROM mcr.microsoft.com/azurelinux/base/core@sha256:9948138108a3d69f1dae62104599ac03132225c3b7a5ac57b85a214629c8567d AS mariner-core

# mcr.microsoft.com/azurelinux/distroless/minimal:3.0
FROM mcr.microsoft.com/azurelinux/distroless/minimal@sha256:0801b80a0927309572b9adc99bd1813bc680473175f6e8175cd4124d95dbd50c AS mariner-distroless

FROM mariner-core AS iptables
RUN tdnf install -y iptables

FROM mariner-distroless AS linux
ARG ARTIFACT_DIR
COPY --from=iptables /usr/sbin/*tables* /usr/sbin/
COPY --from=iptables /usr/lib /usr/lib
COPY ${ARTIFACT_DIR}/bin/azure-iptables-monitor /azure-iptables-monitor

ENTRYPOINT ["/azure-iptables-monitor"]
