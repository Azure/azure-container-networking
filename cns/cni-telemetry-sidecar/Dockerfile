ARG ARCH
ARG OS_VERSION
ARG OS

FROM --platform=linux/${ARCH} mcr.microsoft.com/oss/go/microsoft/golang@sha256:8f60e85f4b2f567c888d0b3a4cd12dc74bee534d94c528655546452912d90c74 AS go

FROM go AS cni-telemetry-sidecar
ARG OS
ARG VERSION
ARG CNI_AI_PATH
ARG CNI_AI_ID

# Add debug output to verify build args are received
RUN echo "=== Dockerfile Build Debug ==="
RUN echo "VERSION = $VERSION"
RUN echo "CNI_AI_ID = $CNI_AI_ID"
RUN echo "CNI_AI_PATH = $CNI_AI_PATH"

WORKDIR /azure-container-networking
COPY . .

# Build the telemetry sidecar with embedded AppInsights key using CNI_AI_PATH
RUN GOOS=$OS CGO_ENABLED=0 go build -a -o /go/bin/azure-cni-telemetry-sidecar \
    -trimpath \
    -ldflags "-X main.version=$VERSION -X $CNI_AI_PATH=$CNI_AI_ID" \
    -gcflags="-dwarflocationlists=true" \
    ./cns/cni-telemetry-sidecar

# Verify the key is embedded in the telemetry.aiMetadata variable
RUN echo "=== Binary Debug ==="
RUN strings /go/bin/azure-cni-telemetry-sidecar | grep "5515a1eb" || echo "AI key NOT found in binary!"
RUN ls -la /go/bin/azure-cni-telemetry-sidecar

FROM scratch AS bins
COPY --from=cni-telemetry-sidecar /go/bin/azure-cni-telemetry-sidecar /

FROM mcr.microsoft.com/azurelinux/base/core:3.0 AS linux
COPY --from=cni-telemetry-sidecar /go/bin/azure-cni-telemetry-sidecar /usr/local/bin/

# Create directories for logs and config
RUN mkdir -p /var/log /etc/azure-cns /var/run

ENTRYPOINT ["/usr/local/bin/azure-cni-telemetry-sidecar"]
CMD ["--config=/etc/azure-cns/cns_config.json", "--log-level=info"]