FROM ubuntu:16.04
RUN apt-get update && apt-get install -y software-properties-common sudo wget apt-utils apt-transport-https curl lsb-release gnupg jq
RUN wget -q https://packages.microsoft.com/config/ubuntu/20.04/packages-microsoft-prod.deb -O packages-microsoft-prod.deb
RUN dpkg -i packages-microsoft-prod.deb
RUN curl -sL https://aka.ms/InstallAzureCLIDeb | sudo bash
RUN curl -fsSL https://get.docker.com -o get-docker.sh && sh get-docker.sh
RUN apt install -y git python3-pip gcc zip dotnet-sdk-3.1 azure-cli
RUN apt install -y --no-install-recommends clang cmake zlib1g-dev libboost-dev libboost-thread-dev gdb build-essential libssl-dev
RUN pip3 install coverage
RUN echo "deb http://archive.ubuntu.com/ubuntu/ bionic multiverse" | sudo tee -a /etc/apt/sources.list
RUN echo "deb http://archive.ubuntu.com/ubuntu/ bionic universe" | sudo tee -a /etc/apt/sources.list
RUN echo "deb http://archive.ubuntu.com/ubuntu/ bionic main" | sudo tee -a /etc/apt/sources.list
RUN apt-get update && apt-get install -y iptables ipset iproute2 ebtables 
RUN wget -qO- https://golang.org/dl/go1.16.linux-amd64.tar.gz | tar zxf - -C /usr/lib/
ENV PATH="/usr/lib/go/bin/:${PATH}"
ENV GOROOT="/usr/lib/go"
ENV GOPATH="/root/go"
ENV PATH="/root/go/bin/:${PATH}"
RUN go get github.com/docker/libnetwork/driverapi
RUN go get github.com/gorilla/mux
RUN go get github.com/jstemmer/go-junit-report
RUN go get github.com/axw/gocov/gocov
RUN go get github.com/AlekSi/gocov-xml
RUN go get -u gopkg.in/matm/v1/gocov-html
RUN go get -u github.com/onsi/ginkgo/ginkgo
