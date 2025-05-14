ARG ARTIFACT_DIR

FROM mcr.microsoft.com/cbl-mariner/distroless/minimal:2.0 AS linux
COPY ${ARTIFACT_DIR}/lib/* /lib
COPY ${ARTIFACT_DIR}/bins/ipv6-hp-bpf /ipv6-hp-bpf
COPY ${ARTIFACT_DIR}/bins/nft /usr/sbin/nft
COPY ${ARTIFACT_DIR}/bins/ip /sbin/ip
CMD ["/ipv6-hp-bpf"]
