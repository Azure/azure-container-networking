ARG ARCH
ARG OS

# mcr.microsoft.com/oss/go/microsoft/golang:1.27-azurelinux3.0
FROM --platform=linux/${ARCH} mcr.microsoft.com/oss/go/microsoft/golang:1.27-azurelinux3.0@sha256:f611aba17edb544de685c05b0418e341fb7ef0227ea6f6ec11b10dc6fbd982f1 AS go

# mcr.microsoft.com/azurelinux/distroless/base:3.0
FROM --platform=linux/${ARCH} mcr.microsoft.com/azurelinux/distroless/base:3.0@sha256:e952865122598043cfb54f0257f016e28e02c21904e172b47dcb2148e095693a AS mariner-distroless

FROM go AS azure-ip-masq-merger
ARG OS
ARG VERSION
WORKDIR /azure-ip-masq-merger
COPY ./azure-ip-masq-merger .
RUN GOOS=$OS CGO_ENABLED=0 go build -a -o /go/bin/ip-masq-merger -trimpath -ldflags "-s -w -X main.version="$VERSION"" -gcflags="-dwarflocationlists=true" .

FROM mariner-distroless AS linux
COPY --from=azure-ip-masq-merger /go/bin/ip-masq-merger /azure-ip-masq-merger
ENTRYPOINT [ "/azure-ip-masq-merger" ]
