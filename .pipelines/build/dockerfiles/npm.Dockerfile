ARG ARCH


# intermediate for win-ltsc2022
FROM --platform=windows/${ARCH} mcr.microsoft.com/windows/servercore:ltsc2022@sha256:b52799a67fa596d4bddcd82ae1dab0077a84767fbc33c7342b72b53ecc71ad09 as windows
ARG ARTIFACT_DIR

COPY ${ARTIFACT_DIR}/files/kubeconfigtemplate.yaml kubeconfigtemplate.yaml
COPY ${ARTIFACT_DIR}/scripts/setkubeconfigpath.ps1 setkubeconfigpath.ps1
COPY ${ARTIFACT_DIR}/scripts/setkubeconfigpath-capz.ps1 setkubeconfigpath-capz.ps1
COPY ${ARTIFACT_DIR}/bin/azure-npm.exe npm.exe

CMD ["npm.exe", "start", "--kubeconfig=.\\kubeconfig"]


FROM --platform=linux/${ARCH} mcr.microsoft.com/azurelinux/base/core:3.0@sha256:bd88359666e3f20296e7fe639a480dfd3c3c8eba299b21f26e4560353e21a656 as linux
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
    && tdnf clean all && rm -rf /var/cache/tdnf

COPY ${ARTIFACT_DIR}/bin/azure-npm /usr/bin/azure-npm
ENTRYPOINT ["/usr/bin/azure-npm", "start"]
