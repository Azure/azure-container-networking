FROM mcr.microsoft.com/oss/go/microsoft/golang:1.25.5@sha256:e04b12710a9cb7df385da634f5b4f4f970d6bbdb0ae062fe11aee48749d564f3 AS builder
ARG VERSION
ARG NPM_AI_PATH
ARG NPM_AI_ID
WORKDIR /usr/local/src
COPY . .
RUN MS_GO_NOSYSTEMCRYPTO=1 CGO_ENABLED=0 go build -v -o /usr/local/bin/azure-npm -ldflags "-s -w -X main.version="$VERSION" -X "$NPM_AI_PATH"="$NPM_AI_ID"" -gcflags="-dwarflocationlists=true" npm/cmd/*.go

FROM mcr.microsoft.com/mirror/docker/library/ubuntu:24.04@sha256:c35e29c9450151419d9448b0fd75374fec4fff364a27f176fb458d472dfc9e54 as linux
COPY --from=builder /usr/local/bin/azure-npm /usr/bin/azure-npm
RUN apt-get update && apt-get install -y iptables ipset ca-certificates && apt-get autoremove -y && apt-get clean
RUN chmod +x /usr/bin/azure-npm
ENTRYPOINT ["/usr/bin/azure-npm", "start"]
