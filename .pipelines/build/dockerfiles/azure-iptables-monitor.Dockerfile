ARG ARCH

# mcr.microsoft.com/azurelinux/base/core:3.0
FROM mcr.microsoft.com/azurelinux/base/core:3.0@sha256:bd88359666e3f20296e7fe639a480dfd3c3c8eba299b21f26e4560353e21a656 AS mariner-core

# mcr.microsoft.com/azurelinux/distroless/base:3.0
FROM mcr.microsoft.com/azurelinux/distroless/base:3.0@sha256:6b85a86416a29cdcfd404e9fbd9c5ef49cc0612882cecc702535dcac39c9446c AS mariner-distroless

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
