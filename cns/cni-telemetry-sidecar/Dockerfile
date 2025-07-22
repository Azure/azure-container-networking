# Azure CNI Telemetry Sidecar Dockerfile
# Based on proven CNI Dockerfile pattern from azure-container-networking
ARG ARCH
ARG OS_VERSION
ARG OS

# Use the same proven base images as CNI
# mcr.microsoft.com/oss/go/microsoft/golang:1.23-azurelinux3.0
FROM --platform=linux/${ARCH} mcr.microsoft.com/oss/go/microsoft/golang@sha256:8f60e85f4b2f567c888d0b3a4cd12dc74bee534d94c528655546452912d90c74 AS go

# mcr.microsoft.com/azurelinux/base/core:3.0
FROM --platform=linux/${ARCH} mcr.microsoft.com/azurelinux/base/core@sha256:9948138108a3d69f1dae62104599ac03132225c3b7a5ac57b85a214629c8567d AS mariner-core

FROM go AS azure-cni-telemetry-sidecar
ARG OS
ARG VERSION
ARG CNI_AI_PATH
ARG CNI_AI_ID

WORKDIR /azure-container-networking
COPY . .

# Debug: Check if the source file exists and show Go version
RUN ls -la cns/cni-telemetry-sidecar/ && go version

# Build the Azure CNI telemetry sidecar binary for Linux
RUN CGO_ENABLED=0 GOOS=linux go build \
    -a \
    -o /go/bin/azure-cni-telemetry-sidecar \
    -trimpath \
    -ldflags "-X main.version=${VERSION:-unknown} -X ${CNI_AI_PATH:-main.aiMetadata}=${CNI_AI_ID:-unknown}" \
    -gcflags="-dwarflocationlists=true" \
    ./cns/cni-telemetry-sidecar/

# Build the Azure CNI telemetry sidecar binary for Windows
RUN CGO_ENABLED=0 GOOS=windows go build \
    -a \
    -o /go/bin/azure-cni-telemetry-sidecar.exe \
    -trimpath \
    -ldflags "-X main.version=${VERSION:-unknown} -X ${CNI_AI_PATH:-main.aiMetadata}=${CNI_AI_ID:-unknown}" \
    -gcflags="-dwarflocationlists=true" \
    ./cns/cni-telemetry-sidecar/

# Verify both binaries were built
RUN ls -la /go/bin/azure-cni-telemetry-sidecar*

FROM scratch AS bins
COPY --from=azure-cni-telemetry-sidecar /go/bin/* /

FROM scratch AS linux
COPY --from=azure-cni-telemetry-sidecar /go/bin/azure-cni-telemetry-sidecar /azure-cni-telemetry-sidecar
ENTRYPOINT [ "/azure-cni-telemetry-sidecar" ]

# Windows support following CNI pattern
FROM --platform=windows/${ARCH} mcr.microsoft.com/oss/kubernetes/windows-host-process-containers-base-image@sha256:b4c9637e032f667c52d1eccfa31ad8c63f1b035e8639f3f48a510536bf34032b AS hpc

FROM hpc AS windows
COPY --from=azure-cni-telemetry-sidecar /go/bin/azure-cni-telemetry-sidecar.exe /azure-cni-telemetry-sidecar.exe
ENTRYPOINT [ "/azure-cni-telemetry-sidecar.exe" ]