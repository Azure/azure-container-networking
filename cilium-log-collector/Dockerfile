ARG ARCH
ARG OS

# mcr.microsoft.com/oss/go/microsoft/golang:1.27-azurelinux3.0
FROM --platform=linux/${ARCH} mcr.microsoft.com/oss/go/microsoft/golang:1.27-azurelinux3.0@sha256:7698ebf51921051df706c4a050c3d69bdd5b6f48b8c6f6166429f22f18d1ae8a AS go

FROM go AS fluent-bit-plugin
ARG VERSION
WORKDIR /cilium-log-collector
COPY ./cilium-log-collector .
RUN go build -buildmode=c-shared -a -o out_azure_app_insights.so -trimpath -ldflags "-X main.version=$VERSION" -gcflags="-dwarflocationlists=true" .

FROM mcr.microsoft.com/oss/v2/fluent/fluent-bit:v5.1.2-3 as linux
COPY --from=fluent-bit-plugin /cilium-log-collector/out_azure_app_insights.so /out_azure_app_insights.so
