ARG ARCH

# mcr.microsoft.com/azurelinux/base/core:3.0
FROM mcr.microsoft.com/azurelinux/base/core:3.0@sha256:af39b0587a0fd132e574faabf9aaf56833d69908b9ff227131d519ee219759e7 AS mariner-core

# mcr.microsoft.com/azurelinux/distroless/base:3.0
FROM mcr.microsoft.com/azurelinux/distroless/base:3.0@sha256:431ae1dc2924c41ae54c97c25676990f0c61d6295f42f5d07783c6976f58f1ef AS mariner-distroless

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
