FROM mcr.microsoft.com/oss/go/microsoft/golang:1.23 AS builder
ARG VERSION
ARG NPM_AI_PATH
ARG NPM_AI_ID
WORKDIR /usr/local/src
COPY . .
RUN CGO_ENABLED=0 go build -v -o /usr/local/bin/azure-npm -ldflags "-X main.version="$VERSION" -X "$NPM_AI_PATH"="$NPM_AI_ID"" -gcflags="-dwarflocationlists=true" npm/cmd/*.go

FROM mcr.microsoft.com/mirror/docker/library/ubuntu:20.04 as linux
COPY --from=builder /usr/local/bin/azure-npm /usr/bin/azure-npm
RUN apt-get update && apt-get install -y libc-bin=2.31-0ubuntu9.17 libc6=2.31-0ubuntu9.17 libtasn1-6=4.16.0-2ubuntu0.1 iptables ipset ca-certificates && apt-get autoremove -y && apt-get clean
RUN chmod +x /usr/bin/azure-npm
ENTRYPOINT ["/usr/bin/azure-npm", "start"]
