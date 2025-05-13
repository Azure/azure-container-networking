ARG OS_VERSION

# skopeo inspect docker://mcr.microsoft.com/oss/go/microsoft/golang:1.24.1-cbl-mariner2.0 --format "{{.Name}}@{{.Digest}}"
FROM --platform=linux/amd64 mcr.microsoft.com/oss/go/microsoft/golang@sha256:82f110263e6110403fbbef97153f7532780b01afd44d5906753ac31a6b1b9e90 AS builder
ARG VERSION
ARG NPM_AI_PATH
ARG NPM_AI_ID
WORKDIR /usr/local/src
COPY . .
RUN GOOS=windows CGO_ENABLED=0 go build -v -o /usr/local/bin/azure-npm.exe -ldflags "-X main.version="$VERSION" -X "$NPM_AI_PATH"="$NPM_AI_ID"" -gcflags="-dwarflocationlists=true" npm/cmd/*.go

# intermediate for win-ltsc2022
FROM mcr.microsoft.com/windows/servercore@sha256:45952938708fbde6ec0b5b94de68bcdec3f8c838be018536b1e9e5bd95e6b943 as ltsc2022

FROM ${OS_VERSION}
COPY --from=builder /usr/local/src/npm/examples/windows/kubeconfigtemplate.yaml kubeconfigtemplate.yaml
COPY --from=builder /usr/local/src/npm/examples/windows/setkubeconfigpath.ps1 setkubeconfigpath.ps1
COPY --from=builder /usr/local/bin/azure-npm.exe azure-npm.exe
CMD ["azure-npm.exe", "start" "--kubeconfig=.\\kubeconfig"]
