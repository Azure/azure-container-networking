FROM ubuntu:latest
RUN apt -y update
RUN apt install -y ebtables
RUN apt install -y net-tools
COPY networkmonitor /usr/bin/networkmonitor
CMD ["/usr/bin/networkmonitor"]