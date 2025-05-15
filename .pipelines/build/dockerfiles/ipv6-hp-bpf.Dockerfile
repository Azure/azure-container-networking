ARG ARTIFACT_DIR

FROM mcr.microsoft.com/cbl-mariner/distroless/minimal:2.0 AS linux
COPY ${ARTIFACT_DIR}/lib/* /lib
COPY ${ARTIFACT_DIR}/bin/ipv6-hp-bpf.exe /ipv6-hp-bpf
COPY ${ARTIFACT_DIR}/bin/nft.exe /usr/sbin/nft
COPY ${ARTIFACT_DIR}/bin/ip.exe /sbin/ip
CMD ["/ipv6-hp-bpf"]
