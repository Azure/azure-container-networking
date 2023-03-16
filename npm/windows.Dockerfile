ARG OS_VERSION
FROM --platform=linux/amd64 mcr.microsoft.com/oss/go/microsoft/golang:1.19 AS builder
ARG VERSION
ARG NPM_AI_PATH
ARG NPM_AI_ID
WORKDIR /usr/local/src
COPY . .
RUN GOOS=windows CGO_ENABLED=1 go build -v -o /usr/local/bin/azure-npm.exe -ldflags "-X main.version="$VERSION" -X "$NPM_AI_PATH"="$NPM_AI_ID"" -gcflags="-dwarflocationlists=true" npm/cmd/*.go

FROM mcr.microsoft.com/powershell:lts-7.2-nanoserver-${OS_VERSION}
COPY --from=builder /usr/local/src/npm/examples/windows/kubeconfigtemplate.yaml kubeconfigtemplate.yaml
COPY --from=builder /usr/local/src/npm/examples/windows/setkubeconfigpath.ps1 setkubeconfigpath.ps1
COPY --from=builder /usr/local/src/npm/examples/windows/setkubeconfigpath-capz.ps1 setkubeconfigpath-capz.ps1
COPY --from=builder /usr/local/bin/azure-npm.exe npm.exe
CMD ["npm.exe", "start" "--kubeconfig=.\\kubeconfig"]
