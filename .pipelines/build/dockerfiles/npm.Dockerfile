ARG ARCH


# intermediate for win-ltsc2022
FROM --platform=windows/${ARCH} mcr.microsoft.com/windows/servercore@sha256:45952938708fbde6ec0b5b94de68bcdec3f8c838be018536b1e9e5bd95e6b943 as windows
ARG ARTIFACT_DIR

COPY ${ARTIFACT_DIR}/files/kubeconfigtemplate.yaml kubeconfigtemplate.yaml
COPY ${ARTIFACT_DIR}/scripts/setkubeconfigpath.ps1 setkubeconfigpath.ps1
COPY ${ARTIFACT_DIR}/scripts/setkubeconfigpath-capz.ps1 setkubeconfigpath-capz.ps1
COPY ${ARTIFACT_DIR}/bin/azure-npm.exe npm.exe

CMD ["npm.exe", "start" "--kubeconfig=.\\kubeconfig"]


FROM --platform=linux/${ARCH} mcr.microsoft.com/mirror/docker/library/ubuntu:24.04 as linux
ARG ARTIFACT_DIR

RUN apt-get update && apt-get install -y iptables ipset ca-certificates && apt-get autoremove -y && apt-get clean
#RUN apt-get update && \
#    apt-get install -y \
#      linux-libc-dev \
#      libc6-dev \
#      libtasn1-6 \
#      gnutls30 iptables ipset ca-certificates
#RUN apt-get autoremove -y && apt-get clean

COPY ${ARTIFACT_DIR}/bin/azure-npm /usr/bin/azure-npm
ENTRYPOINT ["/usr/bin/azure-npm", "start"]
