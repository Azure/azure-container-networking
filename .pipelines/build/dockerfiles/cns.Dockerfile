ARG ARCH
ARG ARTIFACT_DIR

# mcr.microsoft.com/cbl-mariner/base/core:2.0
FROM mcr.microsoft.com/cbl-mariner/base/core@sha256:961bfedbbbdc0da51bc664f51d959da292eced1ad46c3bf674aba43b9be8c703 AS iptables
RUN tdnf install -y iptables

# mcr.microsoft.com/cbl-mariner/distroless/minimal:2.0
FROM mcr.microsoft.com/cbl-mariner/distroless/minimal@sha256:7778a86d86947d5f64c1280a7ee0cf36c6c6d76b5749dd782fbcc14f113961bf AS linux
COPY --from=iptables /usr/sbin/*tables* /usr/sbin/
COPY --from=iptables /usr/lib /usr/lib
COPY ${ARTIFACT_DIR}/bin/azure-cns.exe /usr/local/bin/azure-cns
ENTRYPOINT [ "/usr/local/bin/azure-cns" ]
EXPOSE 10090


# mcr.microsoft.com/oss/kubernetes/windows-host-process-containers-base-image:v1.0.0
FROM --platform=windows/${ARCH} mcr.microsoft.com/oss/kubernetes/windows-host-process-containers-base-image@sha256:b4c9637e032f667c52d1eccfa31ad8c63f1b035e8639f3f48a510536bf34032b AS windows
COPY ${ARTIFACT_DIR}/files/kubeconfigtemplate.yaml kubeconfigtemplate.yaml
COPY ${ARTIFACT_DIR}/scripts/setkubeconfigpath.ps1 setkubeconfigpath.ps1
COPY ${ARTIFACT_DIR}/bin/azure-cns.exe /azure-cns.exe
ENTRYPOINT ["azure-cns.exe"]
EXPOSE 10090
