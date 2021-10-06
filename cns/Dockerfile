# Get a trusted CA cert bundle
FROM alpine:latest as certs
RUN apk --update add ca-certificates

# Build cns
FROM golang:1.17 AS builder
# Build ars
ARG VERSION
ARG CNS_AI_PATH
ARG CNS_AI_ID

WORKDIR /usr/local/src/cns

# Copy the source
COPY . .

# Build cns
RUN CGO_ENABLED=0 go build -a -o /usr/local/bin/azure-cns -ldflags "-X main.version="$VERSION" -X "$CNS_AI_PATH"="$CNS_AI_ID"" -gcflags="-dwarflocationlists=true" cns/service/*.go

# Build aitelemetry
RUN CGO_ENABLED=0 go build -a -o /usr/local/bin/azure-vnet-telemetry -ldflags "-X main.version="$VERSION"" -gcflags="-dwarflocationlists=true" cni/telemetry/service/*.go

# Copy into final image
FROM scratch
COPY --from=builder /etc/passwd /etc/passwd
COPY --from=builder /etc/group /etc/group
COPY --from=builder /usr/local/bin/azure-cns \
	/usr/local/bin/azure-cns
COPY --from=builder /usr/local/bin/azure-vnet-telemetry \
    /usr/local/bin/azure-vnet-telemetry
COPY --from=certs /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/ca-certificates.crt

ENTRYPOINT [ "/usr/local/bin/azure-cns" ]
EXPOSE 10090