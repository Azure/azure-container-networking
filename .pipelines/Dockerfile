FROM ubuntu:20.04
RUN apt-get update && apt-get install -y software-properties-common sudo wget apt-utils apt-transport-https curl lsb-release gnupg jq
RUN wget -q https://packages.microsoft.com/config/ubuntu/20.04/packages-microsoft-prod.deb -O packages-microsoft-prod.deb
RUN sudo dpkg -i packages-microsoft-prod.deb
RUN curl -sL https://aka.ms/InstallAzureCLIDeb | sudo bash
RUN curl -fsSL https://get.docker.com -o get-docker.sh && sh get-docker.sh
RUN apt install -y git golang-1.14-go iptables ipset iproute2 ebtables python3-pip gcc zip dotnet-sdk-3.1 azure-cli
RUN apt install -y --no-install-recommends clang cmake zlib1g-dev libboost-dev libboost-thread-dev gdb build-essential libssl-dev
RUN sudo pip3 install coverage
ENV PATH="/usr/lib/go-1.14/bin/:${PATH}"
RUN go get github.com/docker/libnetwork/driverapi
RUN go get github.com/gorilla/mux
RUN go get github.com/jstemmer/go-junit-report
RUN go get github.com/axw/gocov/gocov
RUN go get github.com/AlekSi/gocov-xml
RUN go get -u gopkg.in/matm/v1/gocov-html