ARG ARCH


# intermediate for win-ltsc2022
FROM --platform=windows/${ARCH} mcr.microsoft.com/windows/servercore:ltsc2022@sha256:f1a914079973258061ddbc95a9af579eab2ee252a6fddb254b1be5d769761acd as windows
ARG ARTIFACT_DIR

COPY ${ARTIFACT_DIR}/files/kubeconfigtemplate.yaml kubeconfigtemplate.yaml
COPY ${ARTIFACT_DIR}/scripts/setkubeconfigpath.ps1 setkubeconfigpath.ps1
COPY ${ARTIFACT_DIR}/scripts/setkubeconfigpath-capz.ps1 setkubeconfigpath-capz.ps1
COPY ${ARTIFACT_DIR}/bin/azure-npm.exe npm.exe

CMD ["npm.exe", "start", "--kubeconfig=.\\kubeconfig"]


FROM --platform=linux/${ARCH} mcr.microsoft.com/azurelinux/base/core:3.0 as linux
ARG ARTIFACT_DIR

RUN tdnf install -y \
        SymCrypt \
        SymCrypt-OpenSSL \
        bash \
        ca-certificates \
        grep \
        ipset \
        iptables \
        openssl-libs \
    && tdnf clean all

COPY ${ARTIFACT_DIR}/bin/azure-npm /usr/bin/azure-npm
ENTRYPOINT ["/usr/bin/azure-npm", "start"]
