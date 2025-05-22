
FROM --platform=linux/${ARCH} mcr.microsoft.com/cbl-mariner/distroless/minimal:2.0 AS linux
COPY /artifacts/lib/* /lib
COPY /artifacts/bin/ipv6-hp-bpf /ipv6-hp-bpf
COPY /artifacts/bin/nft /usr/sbin/nft
COPY /artifacts/bin/ip /sbin/ip
CMD ["/ipv6-hp-bpf"]
