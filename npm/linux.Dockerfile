FROM mcr.microsoft.com/oss/go/microsoft/golang:1.26.7 AS builder
ARG VERSION
ARG NPM_AI_PATH
ARG NPM_AI_ID
WORKDIR /usr/local/src
COPY . .
RUN MS_GO_NOSYSTEMCRYPTO=1 CGO_ENABLED=0 go build -v -o /usr/local/bin/azure-npm -ldflags "-s -w -X main.version="$VERSION" -X "$NPM_AI_PATH"="$NPM_AI_ID"" -gcflags="-dwarflocationlists=true" npm/cmd/*.go

FROM mcr.microsoft.com/mirror/docker/library/ubuntu:24.04 as linux
COPY --from=builder /usr/local/bin/azure-npm /usr/bin/azure-npm
# CVE remediation: pin these Ubuntu 24.04 packages to patched security versions.
# Installing iptables/ipset/ca-certificates alone leaves these base packages at
# their vulnerable image versions, and the pipeline's automated image patching
# (Copacetic) does not flag all of them, so pin each explicitly. Pinning
# libpam-modules also upgrades libpam0g/libpam-modules-bin/libpam-runtime;
# libncursesw6/libtinfo6 also upgrade ncurses-base/ncurses-bin. Refresh when
# newer Ubuntu security updates ship (see npm-cve-status tracker).
# CVEs patched by each pinned package:
# gpgv:           CVE-2025-68973, CVE-2026-57062
# libc-bin:       CVE-2025-15281, CVE-2026-0861, CVE-2026-0915, CVE-2026-19499,
#                 CVE-2026-19542, CVE-2026-4046, CVE-2026-4437, CVE-2026-4438,
#                 CVE-2026-5435, CVE-2026-5450, CVE-2026-5928, CVE-2026-6238,
#                 CVE-2026-6368, CVE-2026-6791, CVE-2026-77117, CVE-2026-80489
# libc6:          CVE-2025-15281, CVE-2026-0861, CVE-2026-0915, CVE-2026-19499,
#                 CVE-2026-19542, CVE-2026-4046, CVE-2026-4437, CVE-2026-4438,
#                 CVE-2026-5435, CVE-2026-5450, CVE-2026-5928, CVE-2026-6238,
#                 CVE-2026-6368, CVE-2026-6791, CVE-2026-77117, CVE-2026-80489
# libtasn1-6:     CVE-2025-13151
# dpkg:           CVE-2026-2219
# libcap2:        CVE-2026-4878
# libgcrypt20:    CVE-2024-2236, CVE-2026-41989
# libgnutls30t64: CVE-2025-14831, CVE-2025-9820, CVE-2026-33845, CVE-2026-33846,
#                 CVE-2026-3832, CVE-2026-3833, CVE-2026-42009, CVE-2026-42010,
#                 CVE-2026-42011, CVE-2026-42012, CVE-2026-42013, CVE-2026-42014,
#                 CVE-2026-42015, CVE-2026-5260, CVE-2026-5419
# libsystemd0:    CVE-2026-15059, CVE-2026-16742, CVE-2026-29111, CVE-2026-40225,
#                 CVE-2026-40226
# libudev1:       CVE-2026-15059, CVE-2026-16742, CVE-2026-29111, CVE-2026-40225,
#                 CVE-2026-40226
# liblzma5:       CVE-2026-34743
# sed:            CVE-2026-5958
# gzip:           CVE-2026-41991, CVE-2026-41992
# libncursesw6:   CVE-2025-6141, CVE-2025-69720
# libtinfo6:      CVE-2025-6141, CVE-2025-69720
# libpam-modules: CVE-2026-54411
# perl-base:      CVE-2025-15649, CVE-2026-12087, CVE-2026-13221, CVE-2026-15534,
#                 CVE-2026-19487, CVE-2026-42496, CVE-2026-42497, CVE-2026-48959,
#                 CVE-2026-48962, CVE-2026-57432, CVE-2026-57433, CVE-2026-7017,
#                 CVE-2026-8376, CVE-2026-9538
# tar:            CVE-2025-45582, CVE-2026-5704
# util-linux:     CVE-2026-13595, CVE-2026-27456, CVE-2026-53612, CVE-2026-53613,
#                 CVE-2026-53614, CVE-2026-53615
# mount:          CVE-2026-13595, CVE-2026-27456, CVE-2026-53612, CVE-2026-53613,
#                 CVE-2026-53614, CVE-2026-53615
# bsdutils:       CVE-2026-13595, CVE-2026-27456, CVE-2026-53612, CVE-2026-53613,
#                 CVE-2026-53614, CVE-2026-53615
# libblkid1:      CVE-2026-13595, CVE-2026-27456, CVE-2026-53612, CVE-2026-53613,
#                 CVE-2026-53614, CVE-2026-53615
# libmount1:      CVE-2026-13595, CVE-2026-27456, CVE-2026-53612, CVE-2026-53613,
#                 CVE-2026-53614, CVE-2026-53615
# libsmartcols1:  CVE-2026-13595, CVE-2026-27456, CVE-2026-53612, CVE-2026-53613,
#                 CVE-2026-53614, CVE-2026-53615
# libuuid1:       CVE-2026-13595, CVE-2026-27456, CVE-2026-53612, CVE-2026-53613,
#                 CVE-2026-53614, CVE-2026-53615
# coreutils:      CVE-2025-5278
# diffutils:      CVE-2026-53910
# libattr1:       CVE-2026-54371
# libbz2-1.0:     CVE-2026-42250
# libp11-kit0:    CVE-2026-13757, CVE-2026-18938
# zlib1g:         CVE-2026-27171
RUN sed -i 's|archive.ubuntu.com|azure.archive.ubuntu.com|g' /etc/apt/sources.list.d/ubuntu.sources \
    && apt-get update \
    && apt-get install -y \
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
    && apt-get autoremove -y \
    && apt-get clean
RUN chmod +x /usr/bin/azure-npm
ENTRYPOINT ["/usr/bin/azure-npm", "start"]
