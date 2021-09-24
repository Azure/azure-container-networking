# Build npm
FROM golang:1.17 AS builder
# Build args
ARG VERSION
ARG NPM_AI_PATH
ARG NPM_AI_ID

WORKDIR /usr/local/src/npm

# Copy the source
COPY . .

# Build npm
RUN CGO_ENABLED=0 go build -v -o /usr/local/bin/azure-npm -ldflags "-X main.version="$VERSION" -X "$NPM_AI_PATH"="$NPM_AI_ID"" -gcflags="-dwarflocationlists=true" npm/cmd/*.go

# Use a minimal image as a final image base
FROM alpine:3.14

# Copy into final image
COPY --from=builder /usr/local/bin/azure-npm \
	/usr/bin/azure-npm

# Install dependencies.
RUN apk update
RUN apk add --no-cache iptables
RUN apk add --no-cache ipset
RUN apk add --no-cache ca-certificates
RUN apk upgrade

RUN chmod +x /usr/bin/azure-npm

# Run the npm command by default when the container starts.
ENTRYPOINT ["/usr/bin/azure-npm", "start"]
