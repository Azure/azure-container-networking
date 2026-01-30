ARG ARCH
ARG OS

# skopeo inspect docker://mcr.microsoft.com/oss/go/microsoft/golang:1.24-azurelinux3.0 --format "{{.Name}}@{{.Digest}}"
FROM --platform=linux/${ARCH} mcr.microsoft.com/oss/go/microsoft/golang@sha256:3999f970bb52b7413ef9be2803173d4fd7f1f3c59362a98a0c78d155e3a0e59f AS go

FROM go AS fluent-bit-plugin
ARG VERSION
WORKDIR /cilium-log-collector
COPY ./cilium-log-collector .
RUN go build -buildmode=c-shared -a -o out_azure_app_insights.so -trimpath -ldflags "-X main.version=$VERSION" -gcflags="-dwarflocationlists=true" .

FROM mcr.microsoft.com/oss/v2/fluent/fluent-bit:v4.2.2 as linux
COPY --from=fluent-bit-plugin /cilium-log-collector/out_azure_app_insights.so /out_azure_app_insights.so
