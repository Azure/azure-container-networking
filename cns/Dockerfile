# Use a minimal image as a parent image
FROM ubuntu:19.10
ARG CNS_BUILD_ARCHIVE

# Install plugin
COPY $CNS_BUILD_ARCHIVE azure-cns.tgz

# Update and unzip cns archive
RUN apt-get update && \
    tar -xvzf azure-cns.tgz && \
    rm azure-cns.tgz

EXPOSE 10090

# Run the cns command by default when the container starts.
ENTRYPOINT ["/azure-cns", "-c", "tcp://localhost:10090"]