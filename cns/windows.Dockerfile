# Build cns
FROM golang:1.17 AS builder
# Build args
ARG VERSION
ARG CNS_AI_PATH
ARG CNS_AI_ID

WORKDIR /usr/local/src/cns

# Copy the source
COPY . .

# Build cns
RUN $Env:CGO_ENABLED=0; go build -v -o /usr/local/bin/azure-cns.exe -ldflags """-X main.version=${env:VERSION} -X ${env:CNS_AI_PATH}=${env:CNS_AI_ID}""" -gcflags="-dwarflocationlists=true" ./cns/service

# Copy into final image
FROM mcr.microsoft.com/windows/nanoserver:1809
COPY --from=builder /usr/local/bin/azure-cns.exe \
    /usr/local/bin/azure-cns.exe

ENTRYPOINT ["/usr/local/bin/azure-cns.exe"]
EXPOSE 10090
