# mcr.microsoft.com/oss/go/microsoft/golang:1.22.3-1-cbl-mariner2.0
FROM mcr.microsoft.com/oss/go/microsoft/golang@sha256:8253def0216b87b2994b7ad689aeec7440f6eb67f981e438071d8d67e36ff69f as golang

# mcr.microsoft.com/cbl-mariner/base/core:2.0
FROM mcr.microsoft.com/cbl-mariner/base/core@sha256:77651116f2e83cf50fddd8a0316945499f8ce6521ff8e94e67539180d1e5975a as mariner-core

# mcr.microsoft.com/cbl-mariner/distroless/minimal:2.0
FROM mcr.microsoft.com/cbl-mariner/distroless/minimal@sha256:63a0a70ceaa1320bc6eb98b81106667d43e46b674731ea8d28e4de1b87e0747f as mariner-distroless

FROM golang AS builder
ARG VERSION
ARG CNS_AI_PATH
ARG CNS_AI_ID
WORKDIR /usr/local/src
COPY . .
RUN CGO_ENABLED=0 go build -a -o /usr/local/bin/azure-cns -ldflags "-X main.version="$VERSION" -X "$CNS_AI_PATH"="$CNS_AI_ID"" -gcflags="-dwarflocationlists=true" cns/service/*.go
RUN CGO_ENABLED=0 go build -a -o /usr/local/bin/azure-vnet-telemetry -ldflags "-X main.version="$VERSION"" -gcflags="-dwarflocationlists=true" cni/telemetry/service/*.go

FROM mariner-core as iptables
RUN tdnf install -y iptables

FROM mariner-distroless
COPY --from=iptables /usr/sbin/*tables* /usr/sbin/
COPY --from=iptables /usr/lib /usr/lib
COPY --from=builder /usr/local/bin/azure-cns \
	/usr/local/bin/azure-cns
ENTRYPOINT [ "/usr/local/bin/azure-cns" ]
EXPOSE 10090
