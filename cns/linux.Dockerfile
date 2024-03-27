FROM mcr.microsoft.com/oss/go/microsoft/golang:1.21 AS builder
ARG VERSION
ARG CNS_AI_PATH
ARG CNS_AI_ID
WORKDIR /usr/local/src
COPY . .
RUN CGO_ENABLED=0 go build -a -o /usr/local/bin/azure-cns -ldflags "-X main.version="$VERSION" -X "$CNS_AI_PATH"="$CNS_AI_ID"" -gcflags="-dwarflocationlists=true" cns/service/*.go
RUN CGO_ENABLED=0 go build -a -o /usr/local/bin/azure-vnet-telemetry -ldflags "-X main.version="$VERSION"" -gcflags="-dwarflocationlists=true" cni/telemetry/service/*.go

FROM registry.k8s.io/build-image/distroless-iptables:v0.5.2
COPY --from=builder /etc/passwd /etc/passwd
COPY --from=builder /etc/group /etc/group
COPY --from=builder /usr/local/bin/azure-cns \
	/usr/local/bin/azure-cns
ENTRYPOINT [ "/usr/local/bin/azure-cns" ]
EXPOSE 10090
