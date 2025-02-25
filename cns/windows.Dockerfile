ARG OS_VERSION
FROM --platform=linux/amd64 mcr.microsoft.com/oss/go/microsoft/golang:1.23 AS builder
ARG VERSION
ARG CNS_AI_PATH
ARG CNS_AI_ID
WORKDIR /usr/local/src
COPY . .
RUN GOOS=windows CGO_ENABLED=0 go build -a -o /usr/local/bin/azure-cns.exe -ldflags "-X main.version="$VERSION" -X "$CNS_AI_PATH"="$CNS_AI_ID"" -gcflags="-dwarflocationlists=true" cns/service/*.go

# intermediate for win-ltsc2019
FROM mcr.microsoft.com/windows/servercore@sha256:2bacc4bdc5d1bd805587cb90a2fb2d58d1c775b02df1a19fd221cbfc639ff587 as ltsc2019

# intermediate for win-ltsc2022
FROM mcr.microsoft.com/windows/servercore@sha256:3cfddbce425e07cc658829098c29af86ff1a3ccc9b8d4735f5263a76ea4b7561 as ltsc2022

FROM ${OS_VERSION}
COPY --from=builder /usr/local/src/cns/kubeconfigtemplate.yaml kubeconfigtemplate.yaml
COPY --from=builder /usr/local/src/npm/examples/windows/setkubeconfigpath.ps1 setkubeconfigpath.ps1
COPY --from=builder /usr/local/bin/azure-cns.exe azure-cns.exe
ENTRYPOINT ["azure-cns.exe"]
EXPOSE 10090
