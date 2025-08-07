ARG ARCH


# mcr.microsoft.com/oss/kubernetes/windows-host-process-containers-base-image:v1.0.0
FROM --platform=windows/${ARCH} mcr.microsoft.com/oss/kubernetes/windows-host-process-containers-base-image@sha256:b4c9637e032f667c52d1eccfa31ad8c63f1b035e8639f3f48a510536bf34032b AS windows
ARG ARTIFACT_DIR .

COPY ${ARTIFACT_DIR}/files/kubeconfigtemplate.yaml kubeconfigtemplate.yaml
COPY ${ARTIFACT_DIR}/scripts/setkubeconfigpath.ps1 setkubeconfigpath.ps1
COPY ${ARTIFACT_DIR}/bin/azure-cns.exe /azure-cns.exe
ENTRYPOINT ["azure-cns.exe"]
EXPOSE 10090


# mcr.microsoft.com/cbl-mariner/base/core:2.0
# skopeo inspect docker://mcr.microsoft.com/cbl-mariner/base/core:2.0 --format "{{.Name}}@{{.Digest}}"
FROM --platform=linux/${ARCH} mcr.microsoft.com/cbl-mariner/base/core@sha256:61b8c8e5c769784be2137cba8612c3a0f0c1752a66276b3b1b5306014a1e20e0 AS build-helper
RUN tdnf install -y iptables

# mcr.microsoft.com/cbl-mariner/distroless/minimal:2.0
FROM --platform=linux/${ARCH} mcr.microsoft.com/cbl-mariner/distroless/minimal@sha256:16d7c214232ee1db683e767a7a30a47f9976801c929b0f2f300521c595eb33ff AS linux
ARG ARTIFACT_DIR .

COPY --from=build-helper /usr/sbin/*tables* /usr/sbin/
COPY --from=build-helper /usr/lib /usr/lib
COPY ${ARTIFACT_DIR}/bin/azure-cns /usr/local/bin/azure-cns
ENTRYPOINT [ "/usr/local/bin/azure-cns" ]
EXPOSE 10090
