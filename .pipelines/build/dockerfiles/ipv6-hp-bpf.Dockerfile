ARG ARCH


FROM --platform=linux/${ARCH} mcr.microsoft.com/azurelinux/distroless/base:3.0@sha256:60a4f5539feea275365474c3600bba9c426872c5a86f80755acd169618da335e AS linux
ARG ARTIFACT_DIR
COPY ${ARTIFACT_DIR}/lib/* /lib
COPY ${ARTIFACT_DIR}/bin/ipv6-hp-bpf /ipv6-hp-bpf
COPY ${ARTIFACT_DIR}/bin/nft /usr/sbin/nft
COPY ${ARTIFACT_DIR}/bin/ip /sbin/ip
CMD ["/ipv6-hp-bpf"]
