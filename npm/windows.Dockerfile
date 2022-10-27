FROM mcr.microsoft.com/oss/go/microsoft/golang:1.19-windowsservercore-ltsc2022 AS builder
# Build args
ARG VERSION
ARG NPM_AI_PATH
ARG NPM_AI_ID

WORKDIR /usr/src/npm
RUN mkdir /usr/bin/
# Copy the source
COPY . .

RUN $Env:CGO_ENABLED=0; go build -mod vendor -v -o /usr/bin/npm.exe -ldflags """-X main.version=${env:VERSION} -X ${env:NPM_AI_PATH}=${env:NPM_AI_ID}""" -gcflags="-dwarflocationlists=true" ./npm/cmd/

# Copy into final image
FROM mcr.microsoft.com/windows/servercore:ltsc2022
COPY --from=builder /usr/src/npm/npm/examples/windows/kubeconfigtemplate.yaml kubeconfigtemplate.yaml
COPY --from=builder /usr/src/npm/npm/examples/windows/setkubeconfigpath.ps1 setkubeconfigpath.ps1
COPY --from=builder /usr/bin/npm.exe npm.exe

CMD ["npm.exe", "start" "--kubeconfig=.\\kubeconfig"]
