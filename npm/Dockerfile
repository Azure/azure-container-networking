# Build npm
FROM mcr.microsoft.com/oss/go/microsoft/golang:1.19 AS builder
# Build args
ARG VERSION
ARG NPM_AI_PATH
ARG NPM_AI_ID

WORKDIR /usr/local/src/npm

# Copy the source
COPY . .

# Build npm
RUN CGO_ENABLED=1 go build -v -o /usr/local/bin/azure-npm -ldflags "-X main.version="$VERSION" -X "$NPM_AI_PATH"="$NPM_AI_ID"" -gcflags="-dwarflocationlists=true" npm/cmd/*.go

# Use a minimal image as a final image base
FROM mcr.microsoft.com/oss/mirror/docker.io/library/ubuntu:20.04

# Copy into final image
COPY --from=builder /usr/local/bin/azure-npm \
  /usr/bin/azure-npm

COPY --from=builder /usr/local/src/npm/npm/scripts \
  /usr/local/npm

# Install dependencies.
RUN apt-get update
RUN apt-get install -y iptables
RUN apt-get install -y ipset
RUN apt-get install -y ca-certificates
RUN apt-get upgrade -y

RUN chmod +x /usr/bin/azure-npm

WORKDIR /usr/local/npm
RUN ./generate_certs.sh

# Run the npm command by default when the container starts.
ENTRYPOINT ["/usr/bin/azure-npm", "start"]
