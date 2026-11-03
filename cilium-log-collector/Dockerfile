ARG ARCH
ARG OS

# mcr.microsoft.com/oss/go/microsoft/golang:1.26-azurelinux3.0
FROM --platform=linux/${ARCH} mcr.microsoft.com/oss/go/microsoft/golang:1.26-azurelinux3.0@sha256:68beb4b8e5374a82f66eb907a3eb5ace30c29bdd8c70be019b5f386064920259 AS go

FROM go AS fluent-bit-plugin
ARG VERSION
ENV GOEXPERIMENT=systemcrypto
WORKDIR /cilium-log-collector
COPY ./cilium-log-collector .
RUN go build -buildmode=c-shared -a -o out_azure_app_insights.so -trimpath -ldflags "-X main.version=$VERSION" -gcflags="-dwarflocationlists=true" .

FROM mcr.microsoft.com/oss/v2/fluent/fluent-bit:v5.1.2-6 as linux
COPY --from=fluent-bit-plugin /cilium-log-collector/out_azure_app_insights.so /out_azure_app_insights.so
