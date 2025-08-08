ARG ARCH
ARG OS

# skopeo inspect docker://mcr.microsoft.com/oss/go/microsoft/golang:1.23.2-azurelinux3.0 --format "{{.Name}}@{{.Digest}}"
FROM --platform=linux/${ARCH} mcr.microsoft.com/oss/go/microsoft/golang@sha256:f1f0cbd464ae4cd9d41176d47f1f9fe16a6965425871f817587314e3a04576ec AS go

FROM go AS azure-ip-masq-merger
ARG OS
ARG VERSION
WORKDIR /azure-ip-masq-merger
COPY ./azure-ip-masq-merger .
RUN GOOS=$OS CGO_ENABLED=0 go build -a -o /go/bin/ip-masq-merger -trimpath -ldflags "-s -w -X main.version="$VERSION"" -gcflags="-dwarflocationlists=true" .

FROM scratch AS linux
COPY --from=azure-ip-masq-merger /go/bin/ip-masq-merger azure-ip-masq-merger
ENTRYPOINT [ "/azure-ip-masq-merger" ]
