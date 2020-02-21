# Use a minimal image as a parent image
FROM ubuntu:19.10
ARG CNS_BUILD_ARCHIVE

# Install plugin
COPY $CNS_BUILD_ARCHIVE azure-cns.tgz

# Update and unzip cns archive
RUN apt-get update && \
    apt-get install -y ca-certificates && \
    tar -xvzf azure-cns.tgz && \
    rm azure-cns.tgz
RUN apt-get upgrade -y

EXPOSE 10090

# Run the cns command by default when the container starts.
ENTRYPOINT ["sh", "-c", "/azure-cns -c tcp://$CNSIpAddress:10090"]