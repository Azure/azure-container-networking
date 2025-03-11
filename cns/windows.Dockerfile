ARG OS_VERSION
# skopeo inspect docker://mcr.microsoft.com/oss/go/microsoft/golang:1.24-cbl-mariner2.0 --format "{{.Name}}@{{.Digest}}"
FROM --platform=linux/amd64 --platform=linux/${ARCH} mcr.microsoft.com/oss/go/microsoft/golang@sha256:15c9b9b8449f55446243ce20c5d3808cc18625d0b358d70aaad402fb73c0766f AS builder
ARG VERSION
ARG CNS_AI_PATH
ARG CNS_AI_ID
WORKDIR /usr/local/src
COPY . .
RUN GOOS=windows CGO_ENABLED=0 go build -a -o /usr/local/bin/azure-cns.exe -ldflags "-X main.version="$VERSION" -X "$CNS_AI_PATH"="$CNS_AI_ID"" -gcflags="-dwarflocationlists=true" cns/service/*.go

# intermediate for win-ltsc2019
FROM mcr.microsoft.com/windows/servercore@sha256:6fdf140282a2f809dae9b13fe441635867f0a27c33a438771673b8da8f3348a4 as ltsc2019

# intermediate for win-ltsc2022
FROM mcr.microsoft.com/windows/servercore@sha256:45952938708fbde6ec0b5b94de68bcdec3f8c838be018536b1e9e5bd95e6b943 as ltsc2022

FROM ${OS_VERSION}
COPY --from=builder /usr/local/src/cns/kubeconfigtemplate.yaml kubeconfigtemplate.yaml
COPY --from=builder /usr/local/src/npm/examples/windows/setkubeconfigpath.ps1 setkubeconfigpath.ps1
COPY --from=builder /usr/local/bin/azure-cns.exe azure-cns.exe
ENTRYPOINT ["azure-cns.exe"]
EXPOSE 10090
