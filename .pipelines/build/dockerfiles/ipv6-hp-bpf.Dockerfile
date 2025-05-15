ARG ARTIFACT_DIR

FROM mcr.microsoft.com/cbl-mariner/distroless/minimal:2.0 AS linux
COPY ${ARTIFACT_DIR}/lib/* /lib
COPY ${ARTIFACT_DIR}/bin/ipv6-hp-bpf.bin /ipv6-hp-bpf
COPY ${ARTIFACT_DIR}/bin/nft.bin /usr/sbin/nft
COPY ${ARTIFACT_DIR}/bin/ip.bin /sbin/ip
CMD ["/ipv6-hp-bpf"]
