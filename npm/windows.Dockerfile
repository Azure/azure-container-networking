ARG OS_VERSION
FROM --platform=linux/amd64 mcr.microsoft.com/oss/go/microsoft/golang:1.25.5 AS builder
ARG VERSION
ARG NPM_AI_PATH
ARG NPM_AI_ID
WORKDIR /usr/local/src
COPY . .
RUN MS_GO_NOSYSTEMCRYPTO=1 GOOS=windows CGO_ENABLED=0 go build -v -o /usr/local/bin/azure-npm.exe -ldflags "-s -w -X main.version="$VERSION" -X "$NPM_AI_PATH"="$NPM_AI_ID"" -gcflags="-dwarflocationlists=true" npm/cmd/*.go

# intermediate for win-ltsc2022
FROM mcr.microsoft.com/windows/servercore@sha256:3a2a2fdfbae2f720f6fe26f2d7680146712ce330f605b02a61d624889735c72e as windows
COPY --from=builder /usr/local/src/npm/examples/windows/kubeconfigtemplate.yaml kubeconfigtemplate.yaml
COPY --from=builder /usr/local/src/npm/examples/windows/setkubeconfigpath.ps1 setkubeconfigpath.ps1
COPY --from=builder /usr/local/src/npm/examples/windows/setkubeconfigpath-capz.ps1 setkubeconfigpath-capz.ps1
COPY --from=builder /usr/local/bin/azure-npm.exe npm.exe
CMD ["npm.exe", "start" "--kubeconfig=.\\kubeconfig"]
