ARG ARTIFACT_DIR

# skopeo inspect docker://mcr.microsoft.com/cbl-mariner/base/core:2.0 --format "{{.Name}}@{{.Digest}}"
FROM --platform=linux/${ARCH} mcr.microsoft.com/cbl-mariner/base/core@sha256:a490e0b0869dc570ae29782c2bc17643aaaad1be102aca83ce0b96e0d0d2d328 AS linux
ARG ARTIFACT_DIR .

ADD . .
#COPY ${ARTIFACT_DIR}/root_artifact.tar .
#RUN tar xvf root_artifact.tar /artifacts/

#FROM --platform=linux/${ARCH} mcr.microsoft.com/cbl-mariner/distroless/minimal:2.0 AS linux
#COPY --from=archive-helper /artifacts/lib/* /lib
#COPY --from=archive-helper /artifacts/bin/ipv6-hp-bpf /ipv6-hp-bpf
#COPY --from=archive-helper /artifacts/bin/nft /usr/sbin/nft
#COPY --from=archive-helper /artifacts/bin/ip /sbin/ip
#CMD ["/ipv6-hp-bpf"]
