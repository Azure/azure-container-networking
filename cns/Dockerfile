FROM mcr.microsoft.com/cbl-mariner/base/core:1.0 as certs
RUN tdnf upgrade -y && tdnf install -y ca-certificates

FROM mcr.microsoft.com/oss/go/microsoft/golang:1.18 AS builder
ARG VERSION
ARG CNS_AI_PATH
ARG CNS_AI_ID
WORKDIR /usr/local/src/cns
COPY . .
RUN CGO_ENABLED=0 go build -a -o /usr/local/bin/azure-cns -ldflags "-X main.version="$VERSION" -X "$CNS_AI_PATH"="$CNS_AI_ID"" -gcflags="-dwarflocationlists=true" cns/service/*.go
RUN CGO_ENABLED=0 go build -a -o /usr/local/bin/azure-vnet-telemetry -ldflags "-X main.version="$VERSION"" -gcflags="-dwarflocationlists=true" cni/telemetry/service/*.go

FROM mcr.microsoft.com/cbl-mariner/base/core:2.0
RUN tdnf install -y iptables
COPY --from=builder /etc/passwd /etc/passwd
COPY --from=builder /etc/group /etc/group
COPY --from=builder /usr/local/bin/azure-cns \
	/usr/local/bin/azure-cns
COPY --from=builder /usr/local/bin/azure-vnet-telemetry \
	/usr/local/bin/azure-vnet-telemetry
COPY --from=certs /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/ca-certificates.crt
ENTRYPOINT [ "/usr/local/bin/azure-cns" ]
EXPOSE 10090
