FROM mcr.microsoft.com/oss/go/microsoft/golang:1.20 AS builder
ARG VERSION
ARG NPM_AI_PATH
ARG NPM_AI_ID
WORKDIR /usr/local/src
COPY . .
RUN CGO_ENABLED=0 go build -v -o /usr/local/bin/azure-npm -ldflags "-X main.version="$VERSION" -X "$NPM_AI_PATH"="$NPM_AI_ID"" -gcflags="-dwarflocationlists=true" npm/cmd/*.go

FROM mcr.microsoft.com/oss/mirror/docker.io/library/ubuntu:20.04
COPY --from=builder /usr/local/bin/azure-npm /usr/bin/azure-npm
COPY --from=builder /usr/local/src/npm/scripts /usr/local/npm
RUN apt-get update && apt-get install -y iptables ipset ca-certificates && apt-get autoremove -y && apt-get clean
RUN chmod +x /usr/bin/azure-npm
WORKDIR /usr/local/npm
RUN ./generate_certs.sh
ENTRYPOINT ["/usr/bin/azure-npm", "start"]
