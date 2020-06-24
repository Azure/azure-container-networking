# Use a minimal image as a parent image
FROM ubuntu:focal-20200606
ARG NPM_BUILD_DIR

# Install dependencies.
RUN apt-get update
RUN apt-get install -y iptables
RUN apt-get install -y ipset
RUN apt-get install -y ca-certificates

# https://lists.ubuntu.com/archives/ubuntu-security-announce/2020-June/005466.html
RUN apt-get install -y libgnutls30=3.6.13-2ubuntu1.1
RUN apt-get upgrade -y

# Install plugin.
COPY $NPM_BUILD_DIR/azure-npm /usr/bin

WORKDIR /usr/bin

# Run the npm command by default when the container starts.
ENTRYPOINT ["/usr/bin/azure-npm"]
