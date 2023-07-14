ARG OS_VERSION
FROM --platform=linux/amd64 mcr.microsoft.com/oss/go/microsoft/golang:1.20 AS builder
ARG VERSION
ARG CNI_AI_PATH
ARG CNI_AI_ID
WORKDIR /azure-container-networking
COPY . .
RUN GOOS=windows CGO_ENABLED=0 go build -a -o azure-vnet.exe -trimpath -ldflags "-X main.version="$VERSION"" -gcflags="-dwarflocationlists=true" cni/network/plugin/main.go
RUN GOOS=windows CGO_ENABLED=0 go build -a -o azure-vnet-telemetry.exe -trimpath -ldflags "-X main.version="$VERSION" -X "$CNI_AI_PATH"="$CNI_AI_ID"" -gcflags="-dwarflocationlists=true" cni/telemetry/service/telemetrymain.go
RUN GOOS=windows CGO_ENABLED=0 go build -a -o azure-vnet-ipam.exe -trimpath -ldflags "-X main.version="$VERSION"" -gcflags="-dwarflocationlists=true" cni/ipam/plugin/main.go

FROM mcr.microsoft.com/windows/servercore:${OS_VERSION}
SHELL ["powershell", "-command"]
COPY --from=builder /azure-container-networking/azure-vnet.exe azure-vnet.exe
COPY --from=builder /azure-container-networking/azure-vnet-telemetry.exe azure-vnet-telemetry.exe
COPY --from=builder /azure-container-networking/telemetry/azure-vnet-telemetry.config azure-vnet-telemetry.config
COPY --from=builder /azure-container-networking/azure-vnet-ipam.exe azure-vnet-ipam.exe

# This would be replaced with dropgz version of windows.
COPY --from=builder /azure-container-networking/hack/scripts/updatecni.ps1 updatecni.ps1
ENTRYPOINT ["powershell.exe", ".\\updatecni.ps1"]
