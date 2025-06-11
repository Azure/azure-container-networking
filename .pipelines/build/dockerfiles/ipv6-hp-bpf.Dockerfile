ARG ARCH


FROM --platform=linux/${ARCH} mcr.microsoft.com/azurelinux/distroless/minimal:3.0 AS linux
ARG ARTIFACT_DIR
COPY ${ARTIFACT_DIR}/lib/* /lib
COPY ${ARTIFACT_DIR}/bin/ipv6-hp-bpf /ipv6-hp-bpf
COPY ${ARTIFACT_DIR}/bin/nft /usr/sbin/nft
COPY ${ARTIFACT_DIR}/bin/ip /sbin/ip
CMD ["/ipv6-hp-bpf"]
