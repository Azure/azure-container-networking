ARG ARCH
# skopeo inspect docker://mcr.microsoft.com/oss/go/microsoft/golang:1.24-cbl-mariner2.0 --format "{{.Name}}@{{.Digest}}"
FROM --platform=linux/${ARCH} mcr.microsoft.com/oss/go/microsoft/golang@sha256:15c9b9b8449f55446243ce20c5d3808cc18625d0b358d70aaad402fb73c0766f AS builder
ARG VERSION
ARG CNS_AI_PATH
ARG CNS_AI_ID
WORKDIR /usr/local/src
COPY . .
RUN CGO_ENABLED=0 go build -a -o /usr/local/bin/azure-cns -ldflags "-X main.version="$VERSION" -X "$CNS_AI_PATH"="$CNS_AI_ID"" -gcflags="-dwarflocationlists=true" cns/service/*.go
RUN CGO_ENABLED=0 go build -a -o /usr/local/bin/azure-vnet-telemetry -ldflags "-X main.version="$VERSION"" -gcflags="-dwarflocationlists=true" cni/telemetry/service/*.go

FROM mcr.microsoft.com/cbl-mariner/base/core:2.0
RUN tdnf upgrade -y && tdnf install -y ca-certificates iptables
COPY --from=builder /etc/passwd /etc/passwd
COPY --from=builder /etc/group /etc/group
COPY --from=builder /usr/local/bin/azure-cns \
	/usr/local/bin/azure-cns
COPY --from=builder /usr/local/bin/azure-vnet-telemetry \
	/usr/local/bin/azure-vnet-telemetry
ENTRYPOINT [ "/usr/local/bin/azure-cns" ]
EXPOSE 10090
