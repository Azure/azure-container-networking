ARG ARCH

FROM scratch AS linux
ARG ARTIFACT_DIR

COPY ${ARTIFACT_DIR}/bin/azure-ip-masq-merger /azure-ip-masq-merger
ENTRYPOINT ["/azure-ip-masq-merger"]
