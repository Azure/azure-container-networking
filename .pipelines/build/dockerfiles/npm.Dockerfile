ARG ARTIFACT_DIR

FROM mcr.microsoft.com/mirror/docker/library/ubuntu:20.04 as linux

RUN apt-get update && \
  apt-get install -y libc-bin=2.31-0ubuntu9.17 libc6=2.31-0ubuntu9.17 libtasn1-6=4.16.0-2ubuntu0.1 libgnutls30=3.6.13-2ubuntu1.12 iptables ipset ca-certificates && \
  apt-get autoremove -y && \
  apt-get clean

COPY ${ARTIFACT_DIR}/bin/azure-npm /usr/bin/azure-npm
RUN chmod +x /usr/bin/azure-npm
ENTRYPOINT ["/usr/bin/azure-npm", "start"]


# intermediate for win-ltsc2022
FROM mcr.microsoft.com/windows/servercore@sha256:45952938708fbde6ec0b5b94de68bcdec3f8c838be018536b1e9e5bd95e6b943 as windows

COPY ${ARTIFACT_DIR}/files/kubeconfigtemplate.yaml kubeconfigtemplate.yaml
COPY ${ARTIFACT_DIR}/files/setkubeconfigpath.ps1 setkubeconfigpath.ps1
COPY ${ARTIFACT_DIR}/files/setkubeconfigpath-capz.ps1 setkubeconfigpath-capz.ps1
COPY ${ARTIFACT_DIR}/bin/azure-npm npm.exe

CMD ["npm.exe", "start" "--kubeconfig=.\\kubeconfig"]
