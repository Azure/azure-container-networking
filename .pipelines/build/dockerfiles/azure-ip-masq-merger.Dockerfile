ARG ARCH

# mcr.microsoft.com/azurelinux/distroless/base:3.0
FROM --platform=linux/${ARCH} mcr.microsoft.com/azurelinux/distroless/base:3.0@sha256:6b85a86416a29cdcfd404e9fbd9c5ef49cc0612882cecc702535dcac39c9446c AS linux
ARG ARTIFACT_DIR

COPY ${ARTIFACT_DIR}/bin/azure-ip-masq-merger /azure-ip-masq-merger
ENTRYPOINT ["/azure-ip-masq-merger"]
