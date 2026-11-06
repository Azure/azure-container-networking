ARG OS_VERSION
FROM --platform=linux/amd64 mcr.microsoft.com/oss/go/microsoft/golang:1.26.10-1 AS builder
ARG VERSION
ARG NPM_AI_PATH
ARG NPM_AI_ID
WORKDIR /usr/local/src
COPY . .
RUN MS_GO_NOSYSTEMCRYPTO=1 GOOS=windows CGO_ENABLED=0 go build -v -o /usr/local/bin/azure-npm.exe -ldflags "-s -w -X main.version="$VERSION" -X "$NPM_AI_PATH"="$NPM_AI_ID"" -gcflags="-dwarflocationlists=true" npm/cmd/*.go

# intermediate for win-ltsc2022
FROM mcr.microsoft.com/windows/servercore:ltsc2022@sha256:b52799a67fa596d4bddcd82ae1dab0077a84767fbc33c7342b72b53ecc71ad09 as windows
COPY --from=builder /usr/local/src/npm/examples/windows/kubeconfigtemplate.yaml kubeconfigtemplate.yaml
COPY --from=builder /usr/local/src/npm/examples/windows/setkubeconfigpath.ps1 setkubeconfigpath.ps1
COPY --from=builder /usr/local/src/npm/examples/windows/setkubeconfigpath-capz.ps1 setkubeconfigpath-capz.ps1
COPY --from=builder /usr/local/bin/azure-npm.exe npm.exe
CMD ["npm.exe", "start" "--kubeconfig=.\\kubeconfig"]
