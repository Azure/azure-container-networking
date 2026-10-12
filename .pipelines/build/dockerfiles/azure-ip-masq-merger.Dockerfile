ARG ARCH

# mcr.microsoft.com/azurelinux/distroless/base:3.0
FROM --platform=linux/${ARCH} mcr.microsoft.com/azurelinux/distroless/base:3.0@sha256:431ae1dc2924c41ae54c97c25676990f0c61d6295f42f5d07783c6976f58f1ef AS linux
ARG ARTIFACT_DIR

COPY ${ARTIFACT_DIR}/bin/azure-ip-masq-merger /azure-ip-masq-merger
ENTRYPOINT ["/azure-ip-masq-merger"]
