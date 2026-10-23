FROM mcr.microsoft.com/oss/go/microsoft/golang:1.25.11-azurelinux3.0 AS builder
ARG VERSION
ARG NPM_AI_PATH
ARG NPM_AI_ID
WORKDIR /usr/local/src
COPY . .
RUN MS_GO_NOSYSTEMCRYPTO=1 CGO_ENABLED=0 go build -v -o /usr/local/bin/azure-npm -ldflags "-X main.version="$VERSION" -X "$NPM_AI_PATH"="$NPM_AI_ID"" -gcflags="-dwarflocationlists=true" npm/cmd/*.go

FROM mcr.microsoft.com/azurelinux/base/core:3.0@sha256:bd88359666e3f20296e7fe639a480dfd3c3c8eba299b21f26e4560353e21a656 AS linux
COPY --from=builder /usr/local/bin/azure-npm /usr/bin/azure-npm
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
RUN chmod +x /usr/bin/azure-npm
ENTRYPOINT ["/usr/bin/azure-npm", "start"]
