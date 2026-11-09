ARG ARCH


# mcr.microsoft.com/azurelinux/distroless/base:3.0
FROM --platform=linux/${ARCH} mcr.microsoft.com/azurelinux/distroless/base:3.0@sha256:370fb2c55e7f0474fed019623cce6a8db65a4d70dedfca9219fc69d6c7627f89 AS linux
ARG ARTIFACT_DIR
COPY ${ARTIFACT_DIR}/lib/* /lib
COPY ${ARTIFACT_DIR}/bin/ipv6-hp-bpf /ipv6-hp-bpf
COPY ${ARTIFACT_DIR}/bin/nft /usr/sbin/nft
COPY ${ARTIFACT_DIR}/bin/ip /sbin/ip
CMD ["/ipv6-hp-bpf"]
