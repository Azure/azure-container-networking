ARG ARCH

# mcr.microsoft.com/azurelinux/base/core:3.0
FROM mcr.microsoft.com/azurelinux/base/core:3.0@sha256:a333b3233076d8b47d63237760e7b36576c0fe47fbf670d5d3517d2fd6fe5030 AS mariner-core

# mcr.microsoft.com/azurelinux/distroless/base:3.0
FROM mcr.microsoft.com/azurelinux/distroless/base:3.0@sha256:11ecd8f7cd1cad0e4fbb565ab98768f1a2ce35cb2730c867217baac011ae8645 AS mariner-distroless

FROM mariner-core AS iptools
RUN tdnf install -y iptables iproute

FROM mariner-distroless AS linux
ARG ARTIFACT_DIR
COPY --from=iptools /usr/sbin/*tables* /usr/sbin/
COPY --from=iptools /usr/sbin/ip /usr/sbin/
COPY --from=iptools /usr/lib /usr/lib
COPY --from=iptools /usr/lib64 /usr/lib64
COPY ${ARTIFACT_DIR}/bin/azure-iptables-monitor /azure-iptables-monitor
COPY ${ARTIFACT_DIR}/bin/azure-block-iptables /azure-block-iptables

ENTRYPOINT ["/azure-iptables-monitor"]
