FROM --platform=linux/amd64 mcr.microsoft.com/oss/go/microsoft/golang:1.23 AS builder
ARG VERSION
ARG CNS_AI_PATH
ARG CNS_AI_ID
WORKDIR /usr/local/src
COPY . .
RUN GOOS=windows CGO_ENABLED=0 go build -a -o /usr/local/bin/azure-cns.exe -ldflags "-X main.version="$VERSION" -X "$CNS_AI_PATH"="$CNS_AI_ID"" -gcflags="-dwarflocationlists=true" cns/service/*.go

# skopeo inspect docker://mcr.microsoft.com/oss/kubernetes/windows-host-process-containers-base-image:v1.0.0 --format "{{.Name}}@{{.Digest}}"
FROM mcr.microsoft.com/oss/kubernetes/windows-host-process-containers-base-image@sha256:b4c9637e032f667c52d1eccfa31ad8c63f1b035e8639f3f48a510536bf34032b as windows
COPY --from=builder /usr/local/src/cns/kubeconfigtemplate.yaml kubeconfigtemplate.yaml
COPY --from=builder /usr/local/src/npm/examples/windows/setkubeconfigpath.ps1 setkubeconfigpath.ps1
COPY --from=builder /usr/local/bin/azure-cns.exe azure-cns.exe
ENTRYPOINT ["azure-cns.exe"]
EXPOSE 10090
