# Build cns
FROM mcr.microsoft.com/oss/go/microsoft/golang:1.19-windowsservercore-ltsc2022 AS builder
# Build args
ARG VERSION
ARG CNS_AI_PATH
ARG CNS_AI_ID

WORKDIR /usr/src/cns
RUN mkdir /usr/bin/

# Copy the source
COPY . .

# Build cns
RUN $Env:CGO_ENABLED=0; go build -mod vendor -v -o /usr/bin/azure-cns.exe -ldflags """-X main.version=${env:VERSION} -X ${env:CNS_AI_PATH}=${env:CNS_AI_ID}""" -gcflags="-dwarflocationlists=true" ./cns/service

# Copy into final image
FROM mcr.microsoft.com/windows/servercore:ltsc2022
COPY --from=builder /usr/src/cns/cns/kubeconfigtemplate.yaml kubeconfigtemplate.yaml
COPY --from=builder /usr/src/cns/npm/examples/windows/setkubeconfigpath.ps1 setkubeconfigpath.ps1
COPY --from=builder /usr/bin/azure-cns.exe azure-cns.exe

ENTRYPOINT ["azure-cns.exe"]
EXPOSE 10090
