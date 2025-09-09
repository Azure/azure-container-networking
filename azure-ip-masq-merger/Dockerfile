ARG ARCH
ARG OS

# skopeo inspect docker://mcr.microsoft.com/oss/go/microsoft/golang:1.24-azurelinux3.0 --format "{{.Name}}@{{.Digest}}"
FROM --platform=linux/${ARCH} mcr.microsoft.com/oss/go/microsoft/golang@sha256:3999f970bb52b7413ef9be2803173d4fd7f1f3c59362a98a0c78d155e3a0e59f AS go

FROM go AS azure-ip-masq-merger
ARG OS
ARG VERSION
WORKDIR /azure-ip-masq-merger
COPY ./azure-ip-masq-merger .
RUN GOOS=$OS CGO_ENABLED=0 go build -a -o /go/bin/ip-masq-merger -trimpath -ldflags "-s -w -X main.version="$VERSION"" -gcflags="-dwarflocationlists=true" .

FROM scratch AS linux
COPY --from=azure-ip-masq-merger /go/bin/ip-masq-merger azure-ip-masq-merger
ENTRYPOINT [ "/azure-ip-masq-merger" ]
