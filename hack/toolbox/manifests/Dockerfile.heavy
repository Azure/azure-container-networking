FROM ubuntu:20.04
RUN apt-get update
RUN apt-get install -y \
    python3 \
    python \
    nmap \
    iptables \
    ebtables \
    sudo \
    iproute2 \
    curl \
    wget \
    axel \
    jq \
    ssh \
    traceroute \
    iputils-ping \
    net-tools \
    dnsutils \
    bridge-utils \
    netcat \
    ncat \
    ssh \
    tcpdump \
    vim \
    emacs \
    nano
RUN wget -qO- https://golang.org/dl/go1.14.6.linux-amd64.tar.gz | tar zxf - -C /usr/lib/
ENV PATH="/usr/lib/go/bin/:${PATH}"
ENV GOROOT="/usr/lib/go"
ENV GOPATH="/root/go"
ENV PATH="/root/go/bin/:${PATH}"
ADD ./server.go /app/
WORKDIR /app 
RUN go build -o main .
CMD ["./main"]