ARG ARCH


# intermediate for win-ltsc2022
FROM --platform=windows/${ARCH} mcr.microsoft.com/windows/servercore:ltsc2022@sha256:76cf422c98ca437b308374d0498280541fa42ac7061bb44015a6c8b70cf4db6a as windows
ARG ARTIFACT_DIR

COPY ${ARTIFACT_DIR}/files/kubeconfigtemplate.yaml kubeconfigtemplate.yaml
COPY ${ARTIFACT_DIR}/scripts/setkubeconfigpath.ps1 setkubeconfigpath.ps1
COPY ${ARTIFACT_DIR}/scripts/setkubeconfigpath-capz.ps1 setkubeconfigpath-capz.ps1
COPY ${ARTIFACT_DIR}/bin/azure-npm.exe npm.exe

CMD ["npm.exe", "start", "--kubeconfig=.\\kubeconfig"]


FROM --platform=linux/${ARCH} mcr.microsoft.com/mirror/docker/library/ubuntu:24.04 as linux
ARG ARTIFACT_DIR

# CVE remediation: pin Ubuntu 24.04 packages to the latest patched security
# versions. Some fixed packages are not flagged by the pipeline's automated
# image patching (Copacetic), so pin them explicitly to guarantee the fixes are
# present at build time. Refresh these pins when newer Ubuntu security updates
# ship (see npm-cve-status tracker). Keep this list in sync with npm/linux.Dockerfile.
RUN apt-get update && apt-get install -y \
        iptables ipset ca-certificates \
        gpgv=2.4.4-2ubuntu17.6 \
        libc-bin=2.39-0ubuntu8.9 \
        libc6=2.39-0ubuntu8.9 \
        libtasn1-6=4.19.0-3ubuntu0.24.04.2 \
        dpkg=1.22.6ubuntu6.6 \
        libcap2=1:2.66-5ubuntu2.4 \
        libgcrypt20=1.10.3-2ubuntu0.2 \
        libgnutls30t64=3.8.3-1.1ubuntu3.6 \
        libsystemd0=255.4-1ubuntu8.17 \
        libudev1=255.4-1ubuntu8.17 \
        liblzma5=5.6.1+really5.4.5-1ubuntu0.3 \
        sed=4.9-2ubuntu0.24.04.1 \
        gzip=1.12-1ubuntu3.2 \
        libncursesw6=6.4+20240113-1ubuntu2.2 \
        libtinfo6=6.4+20240113-1ubuntu2.2 \
        libpam-modules=1.5.3-5ubuntu5.7 \
        perl-base=5.38.2-3.2ubuntu0.6 \
        tar=1.35+dfsg-3ubuntu0.4 \
        util-linux=2.39.3-9ubuntu6.6 \
        mount=2.39.3-9ubuntu6.6 \
        bsdutils=1:2.39.3-9ubuntu6.6 \
        libblkid1=2.39.3-9ubuntu6.6 \
        libmount1=2.39.3-9ubuntu6.6 \
        libsmartcols1=2.39.3-9ubuntu6.6 \
        libuuid1=2.39.3-9ubuntu6.6 \
        coreutils=9.4-3ubuntu6.3 \
        diffutils=1:3.10-1ubuntu0.1 \
        libattr1=1:2.5.2-1ubuntu0.1 \
        libbz2-1.0=1.0.8-5.1ubuntu0.1 \
        libp11-kit0=0.25.3-4ubuntu2.2 \
        zlib1g=1:1.3.dfsg-3.1ubuntu2.2 \
    && apt-get autoremove -y && apt-get clean
#RUN apt-get update && \
#    apt-get install -y \
#      linux-libc-dev \
#      libc6-dev \
#      libtasn1-6 \
#      gnutls30 iptables ipset ca-certificates
#RUN apt-get autoremove -y && apt-get clean

COPY ${ARTIFACT_DIR}/bin/azure-npm /usr/bin/azure-npm
ENTRYPOINT ["/usr/bin/azure-npm", "start"]
