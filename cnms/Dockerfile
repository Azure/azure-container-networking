FROM mcr.microsoft.com/cbl-mariner/base/core:2.0
ARG CNMS_BUILD_DIR
RUN tdnf upgrade
RUN tdnf install -y ebtables
RUN tdnf install -y net-tools
COPY output/linux_amd64/cnms/azure-cnms /usr/bin/azure-cnms
RUN chmod +x /usr/bin/azure-cnms
CMD ["/usr/bin/azure-cnms"]
