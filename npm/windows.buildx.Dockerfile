FROM --platform=linux/amd64 mcr.microsoft.com/oss/go/microsoft/golang:1.19 as builder
# Build args
ARG VERSION
ARG NPM_AI_PATH
ARG NPM_AI_ID

ENV GOOS=windows
ENV GOARCH=amd64

WORKDIR /usr/src/npm
# Copy the source
COPY . .

RUN go build -mod=mod -v -o /usr/bin/npm.exe -ldflags "-X main.version=$VERSION -X $NPM_AI_PATH=$NPM_AI_ID" -gcflags="-dwarflocationlists=true" ./npm/cmd/

# Copy into final image
FROM mcr.microsoft.com/windows/servercore:ltsc2022
COPY --from=builder /usr/src/npm/npm/examples/windows/kubeconfigtemplate.yaml kubeconfigtemplate.yaml
COPY --from=builder /usr/src/npm/npm/examples/windows/setkubeconfigpath.ps1 setkubeconfigpath.ps1
COPY --from=builder /usr/bin/npm.exe npm.exe

CMD ["npm.exe", "start" "--kubeconfig=.\\kubeconfig"]