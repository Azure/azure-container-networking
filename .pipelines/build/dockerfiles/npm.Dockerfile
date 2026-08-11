ARG ARCH


# intermediate for win-ltsc2022
FROM --platform=windows/${ARCH} mcr.microsoft.com/windows/servercore:ltsc2022@sha256:3a2a2fdfbae2f720f6fe26f2d7680146712ce330f605b02a61d624889735c72e as windows
ARG ARTIFACT_DIR

COPY ${ARTIFACT_DIR}/files/kubeconfigtemplate.yaml kubeconfigtemplate.yaml
COPY ${ARTIFACT_DIR}/scripts/setkubeconfigpath.ps1 setkubeconfigpath.ps1
COPY ${ARTIFACT_DIR}/scripts/setkubeconfigpath-capz.ps1 setkubeconfigpath-capz.ps1
COPY ${ARTIFACT_DIR}/bin/azure-npm.exe npm.exe

CMD ["npm.exe", "start", "--kubeconfig=.\\kubeconfig"]


FROM --platform=linux/${ARCH} mcr.microsoft.com/mirror/docker/library/ubuntu:24.04 as linux
ARG ARTIFACT_DIR

# Manually patch Ubuntu CVEs:
# gpgv:           CVE-2025-68973 (HIGH)
# libc-bin:       CVE-2025-15281, CVE-2026-0861, CVE-2026-0915 (MEDIUM)
# libc6:          CVE-2025-15281, CVE-2026-0861, CVE-2026-0915 (MEDIUM)
# libtasn1-6:     CVE-2025-13151 (MEDIUM)
# dpkg:           CVE-2026-2219 (MEDIUM)
# libcap2:        CVE-2026-4878 (MEDIUM)
# libgcrypt20:    CVE-2026-41989 (MEDIUM)
# libgnutls30t64: CVE-2026-33845, CVE-2026-33846, CVE-2026-3832, CVE-2026-3833,
#                 CVE-2026-42009, CVE-2026-42010, CVE-2026-42011, CVE-2026-42012,
#                 CVE-2026-42013, CVE-2026-42014, CVE-2026-42015, CVE-2026-5260,
#                 CVE-2026-5419 (MEDIUM)
# libsystemd0:    CVE-2026-29111, CVE-2026-40225 (MEDIUM)
# libudev1:       CVE-2026-29111, CVE-2026-40225 (MEDIUM)
# liblzma5:       CVE-2026-34743 (LOW)
# sed:            CVE-2026-5958 (MEDIUM)
# gzip:           CVE-2026-41991, CVE-2026-41992 (LOW)
# libncursesw6:   CVE-2025-69720 (MEDIUM)
# libtinfo6:      CVE-2025-69720 (MEDIUM)
# libpam-modules: CVE-2026-54411 (MEDIUM)
# perl-base:      CVE-2026-42496, CVE-2026-8376 (MEDIUM)
# tar:            CVE-2025-45582, CVE-2026-5704 (MEDIUM)
RUN apt-get update && apt-get install -y \
    iptables ipset ca-certificates \
    gpgv=2.4.4-2ubuntu17.4 \
    libc-bin=2.39-0ubuntu8.8 \
    libc6=2.39-0ubuntu8.8 \
    libtasn1-6=4.19.0-3ubuntu0.24.04.2 \
    dpkg=1.22.6ubuntu6.6 \
    libcap2=1:2.66-5ubuntu2.4 \
    libgcrypt20=1.10.3-2ubuntu0.1 \
    libgnutls30t64=3.8.3-1.1ubuntu3.6 \
    libsystemd0=255.4-1ubuntu8.16 \
    libudev1=255.4-1ubuntu8.16 \
    liblzma5=5.6.1+really5.4.5-1ubuntu0.3 \
    sed=4.9-2ubuntu0.24.04.1 \
    gzip=1.12-1ubuntu3.2 \
    libncursesw6=6.4+20240113-1ubuntu2.1 \
    libtinfo6=6.4+20240113-1ubuntu2.1 \
    libpam-modules=1.5.3-5ubuntu5.6 \
    perl-base=5.38.2-3.2ubuntu0.3 \
    tar=1.35+dfsg-3ubuntu0.4 \
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
