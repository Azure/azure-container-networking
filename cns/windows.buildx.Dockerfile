# Build cns
FROM --platform=linux/amd64 mcr.microsoft.com/oss/go/microsoft/golang:1.19 as builder
# Build args
ARG VERSION
ARG CNS_AI_PATH
ARG CNS_AI_ID

ENV GOOS=windows
ENV GOARCH=amd64
# Build cns
#RUN make azure-cns-binary
WORKDIR /usr/src/cns
COPY . .

RUN go build -mod mod -v -o /usr/bin/azure-cns.exe -ldflags "-X main.version=$VERSION -X $CNS_AI_PATH=$CNS_AI_ID" -gcflags="-dwarflocationlists=true" ./cns/service/

# Copy into final image
FROM mcr.microsoft.com/windows/servercore:ltsc2022
COPY --from=builder /usr/src/cns/cns/kubeconfigtemplate.yaml kubeconfigtemplate.yaml
COPY --from=builder /usr/src/cns/npm/examples/windows/setkubeconfigpath.ps1 setkubeconfigpath.ps1
COPY --from=builder /usr/bin/azure-cns.exe azure-cns.exe

ENTRYPOINT ["azure-cns.exe"]
EXPOSE 10090
