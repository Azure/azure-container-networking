ARG ARCHIVE_DIR

FROM mcr.microsoft.com/cbl-mariner/distroless/minimal:2.0 AS linux
COPY ${ARCHIVE_DIR}/lib/* /lib
COPY ${ARCHIVE_DIR}/bins/ipv6-hp-bpf /ipv6-hp-bpf
COPY ${ARCHIVE_DIR}/bins/nft /usr/sbin/nft
COPY ${ARCHIVE_DIR}/bins/ip /sbin/ip
CMD ["/ipv6-hp-bpf"]
