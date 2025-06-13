ARG ARCH

FROM --platform=linux/${ARCH} mcr.microsoft.com/azurelinux/distroless/minimal:3.0 AS linux
ARG ARTIFACT_DIR

COPY ${ARTIFACT_DIR}/bin/azure-ip-masq-merger /azure-ip-masq-merger
ENTRYPOINT ["/azure-ip-masq-merger"]
