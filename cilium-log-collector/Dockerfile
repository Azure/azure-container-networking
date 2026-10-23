ARG ARCH
ARG OS

# mcr.microsoft.com/oss/go/microsoft/golang:1.26-azurelinux3.0
FROM --platform=linux/${ARCH} mcr.microsoft.com/oss/go/microsoft/golang:1.26-azurelinux3.0@sha256:d104fef62900c2ca101f52568f43e0e6b6499b60be324fdd4d2ad5bae4bc3203 AS go

FROM go AS fluent-bit-plugin
ARG VERSION
ENV GOEXPERIMENT=systemcrypto
WORKDIR /cilium-log-collector
COPY ./cilium-log-collector .
RUN go build -buildmode=c-shared -a -o out_azure_app_insights.so -trimpath -ldflags "-X main.version=$VERSION" -gcflags="-dwarflocationlists=true" .

FROM mcr.microsoft.com/oss/v2/fluent/fluent-bit:v5.1.2-6 as linux
COPY --from=fluent-bit-plugin /cilium-log-collector/out_azure_app_insights.so /out_azure_app_insights.so
