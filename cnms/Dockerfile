FROM sbidprod.azurecr.io/quinault:latest
ARG CNMS_BUILD_DIR

RUN apt -y update
RUN apt-get -y upgrade
RUN apt install -y ebtables
RUN apt install -y net-tools
COPY $CNMS_BUILD_DIR/azure-cnms /usr/bin/azure-cnms
RUN chmod +x /usr/bin/azure-cnms
CMD ["/usr/bin/azure-cnms"]
