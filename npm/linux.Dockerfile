FROM mcr.microsoft.com/oss/go/microsoft/golang:1.25.7 AS builder
ARG VERSION
ARG NPM_AI_PATH
ARG NPM_AI_ID
WORKDIR /usr/local/src
COPY . .
RUN MS_GO_NOSYSTEMCRYPTO=1 CGO_ENABLED=0 go build -v -o /usr/local/bin/azure-npm -ldflags "-X main.version="$VERSION" -X "$NPM_AI_PATH"="$NPM_AI_ID"" -gcflags="-dwarflocationlists=true" npm/cmd/*.go

FROM mcr.microsoft.com/mirror/docker/library/ubuntu:24.04 AS linux
COPY --from=builder /usr/local/bin/azure-npm /usr/bin/azure-npm
# Manually patch Ubuntu CVEs:
# gpgv:      CVE-2025-68973 (HIGH)
# libc-bin:  CVE-2025-15281, CVE-2026-0861, CVE-2026-0915 (MEDIUM)
# libc6:     CVE-2025-15281, CVE-2026-0861, CVE-2026-0915 (MEDIUM)
# libtasn1-6: CVE-2025-13151 (MEDIUM)
RUN apt-get update && apt-get install -y \
    iptables ipset ca-certificates \
    gpgv=2.4.4-2ubuntu17.4 \
    libc-bin=2.39-0ubuntu8.7 \
    libc6=2.39-0ubuntu8.7 \
    libtasn1-6=4.19.0-3ubuntu0.24.04.2 \
    && apt-get autoremove -y && apt-get clean
RUN chmod +x /usr/bin/azure-npm
ENTRYPOINT ["/usr/bin/azure-npm", "start"]
