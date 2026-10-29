ARG ARCH


# mcr.microsoft.com/azurelinux/distroless/base:3.0
FROM --platform=linux/${ARCH} mcr.microsoft.com/azurelinux/distroless/base:3.0@sha256:e952865122598043cfb54f0257f016e28e02c21904e172b47dcb2148e095693a AS linux
ARG ARTIFACT_DIR
COPY ${ARTIFACT_DIR}/lib/* /lib
COPY ${ARTIFACT_DIR}/bin/ipv6-hp-bpf /ipv6-hp-bpf
COPY ${ARTIFACT_DIR}/bin/nft /usr/sbin/nft
COPY ${ARTIFACT_DIR}/bin/ip /sbin/ip
CMD ["/ipv6-hp-bpf"]
