# Use a minimal image as a parent image
FROM docker.io/ubuntu:19.10
ARG TELEMETRY_BUILD_DIR
ARG TELEMETRY_CONF_DIR

# Install plugin
COPY $TELEMETRY_BUILD_DIR/azure-vnet-telemetry /usr/bin
COPY $TELEMETRY_CONF_DIR/azure-vnet-telemetry.config /usr/bin

WORKDIR /usr/bin

# Run below command by default when the container starts.
ENTRYPOINT ["/usr/bin/azure-vnet-telemetry", "-d", "/usr/bin"]
