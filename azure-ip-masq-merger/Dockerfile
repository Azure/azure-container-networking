ARG ARCH
ARG OS

# mcr.microsoft.com/oss/go/microsoft/golang:1.26-azurelinux3.0
FROM --platform=linux/${ARCH} mcr.microsoft.com/oss/go/microsoft/golang:1.26-azurelinux3.0@sha256:ed6bde29ab1d8e2ee55cacb3bff7f8f073bb74d4529833c4fee77d647cdeaeda AS go

# mcr.microsoft.com/azurelinux/distroless/base:3.0
FROM --platform=linux/${ARCH} mcr.microsoft.com/azurelinux/distroless/base:3.0@sha256:370fb2c55e7f0474fed019623cce6a8db65a4d70dedfca9219fc69d6c7627f89 AS mariner-distroless

FROM go AS azure-ip-masq-merger
ARG OS
ARG VERSION
ENV GOEXPERIMENT=ms_nocgo_opensslcrypto
WORKDIR /azure-ip-masq-merger
COPY ./azure-ip-masq-merger .
RUN GOOS=$OS CGO_ENABLED=0 go build -a -o /go/bin/ip-masq-merger -trimpath -ldflags "-s -w -X main.version="$VERSION"" -gcflags="-dwarflocationlists=true" .

FROM mariner-distroless AS linux
COPY --from=azure-ip-masq-merger /go/bin/ip-masq-merger /azure-ip-masq-merger
ENTRYPOINT [ "/azure-ip-masq-merger" ]
