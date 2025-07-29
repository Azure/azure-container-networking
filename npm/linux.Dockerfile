FROM mcr.microsoft.com/oss/go/microsoft/golang:1.23-azurelinux3.0 AS builder
ARG VERSION
ARG NPM_AI_PATH
ARG NPM_AI_ID
WORKDIR /usr/local/src
COPY . .
RUN CGO_ENABLED=0 go build -v -o /usr/local/bin/azure-npm -ldflags "-X main.version="$VERSION" -X "$NPM_AI_PATH"="$NPM_AI_ID"" -gcflags="-dwarflocationlists=true" npm/cmd/*.go

FROM mcr.microsoft.com/mirror/docker/library/ubuntu:24.04 as linux
COPY --from=builder /usr/local/bin/azure-npm /usr/bin/azure-npm
RUN apt-get update && apt-get install -y libsystemd0=255.4-1ubuntu8.8 libudev1=255.4-1ubuntu8.8 libpam-modules=1.5.3-5ubuntu5.4 libpam-modules-bin=1.5.3-5ubuntu5.4 libpam-runtime=1.5.3-5ubuntu5.4 libpam0g=1.5.3-5ubuntu5.4 iptables ipset ca-certificates && apt-get autoremove -y && apt-get clean
RUN chmod +x /usr/bin/azure-npm
ENTRYPOINT ["/usr/bin/azure-npm", "start"]
