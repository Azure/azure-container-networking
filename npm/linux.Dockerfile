FROM mcr.microsoft.com/oss/go/microsoft/golang:1.25.10 AS builder
ARG VERSION
ARG NPM_AI_PATH
ARG NPM_AI_ID
WORKDIR /usr/local/src
COPY . .
RUN MS_GO_NOSYSTEMCRYPTO=1 CGO_ENABLED=0 go build -v -o /usr/local/bin/azure-npm -ldflags "-X main.version="$VERSION" -X "$NPM_AI_PATH"="$NPM_AI_ID"" -gcflags="-dwarflocationlists=true" npm/cmd/*.go

FROM mcr.microsoft.com/mirror/docker/library/ubuntu:24.04 AS linux
COPY --from=builder /usr/local/bin/azure-npm /usr/bin/azure-npm
# Manually patch Ubuntu CVEs:
# gpgv:           CVE-2025-68973 (HIGH)
# libc-bin:       CVE-2025-15281, CVE-2026-0861, CVE-2026-0915 (MEDIUM)
# libc6:          CVE-2025-15281, CVE-2026-0861, CVE-2026-0915 (MEDIUM)
# libtasn1-6:     CVE-2025-13151 (MEDIUM)
# dpkg:           CVE-2026-2219 (MEDIUM)
# libcap2:        CVE-2026-4878 (MEDIUM)
# libgnutls30t64: CVE-2026-33845, CVE-2026-33846, CVE-2026-3832, CVE-2026-3833,
#                 CVE-2026-42009, CVE-2026-42010, CVE-2026-42011, CVE-2026-42012,
#                 CVE-2026-42013, CVE-2026-42014, CVE-2026-42015, CVE-2026-5260,
#                 CVE-2026-5419 (MEDIUM)
# libsystemd0:    CVE-2026-29111 (MEDIUM)
# libudev1:       CVE-2026-29111 (MEDIUM)
# sed:            CVE-2026-5958 (MEDIUM)
RUN apt-get update && apt-get install -y \
    iptables ipset ca-certificates \
    gpgv=2.4.4-2ubuntu17.4 \
    libc-bin=2.39-0ubuntu8.7 \
    libc6=2.39-0ubuntu8.7 \
    libtasn1-6=4.19.0-3ubuntu0.24.04.2 \
    dpkg=1.22.6ubuntu6.6 \
    libcap2=1:2.66-5ubuntu2.4 \
    libgnutls30t64=3.8.3-1.1ubuntu3.6 \
    libsystemd0=255.4-1ubuntu8.14 \
    libudev1=255.4-1ubuntu8.14 \
    sed=4.9-2ubuntu0.24.04.1 \
    && apt-get autoremove -y && apt-get clean
RUN chmod +x /usr/bin/azure-npm
ENTRYPOINT ["/usr/bin/azure-npm", "start"]
