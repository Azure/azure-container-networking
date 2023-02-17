# OS_SKU can be one of [ltsc2022, 1809]. ltsc2022 is default. 1809 is also known as ws2019, but golang doesn't tag
# its 1809 image with ltsc2019, so we have to use 1809 here. 
ARG OS_SKU=ltsc2022
# Build cns
FROM mcr.microsoft.com/oss/go/microsoft/golang:1.19-windowsservercore-${OS_SKU} AS builder
# Build args
ARG VERSION=unknown
ARG CNS_AI_PATH=github.com/Azure/azure-container-networking/cns/logger.aiMetadata
ARG CNS_AI_ID=ce672799-8f08-4235-8c12-08563dc2acef

WORKDIR /usr/src/cns
RUN mkdir /usr/bin/

# Copy the source
COPY . .

# Build cns
RUN $Env:CGO_ENABLED=0; go build -mod vendor -v -o /usr/bin/azure-cns.exe -ldflags """-X main.version=${env:VERSION} -X ${env:CNS_AI_PATH}=${env:CNS_AI_ID}""" -gcflags="-dwarflocationlists=true" ./cns/service

# Copy into final image
FROM mcr.microsoft.com/windows/servercore:${OS_SKU}
COPY --from=builder /usr/src/cns/cns/kubeconfigtemplate.yaml kubeconfigtemplate.yaml
COPY --from=builder /usr/src/cns/npm/examples/windows/setkubeconfigpath.ps1 setkubeconfigpath.ps1
COPY --from=builder /usr/bin/azure-cns.exe azure-cns.exe

ENTRYPOINT ["azure-cns.exe"]
EXPOSE 10090
