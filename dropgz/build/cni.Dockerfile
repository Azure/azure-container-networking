FROM mcr.microsoft.com/oss/go/microsoft/golang:1.18 AS builder
ARG VERSION
WORKDIR /azure-container-networking
COPY . .
RUN CGO_ENABLED=0 go build -a -o bin/azure-vnet -trimpath -ldflags "-X main.version="$VERSION"" -gcflags="-dwarflocationlists=true" cni/network/plugin/main.go
RUN mv bin/* dropgz/pkg/embed/gz &&\ 
    mv cni/*.conflist dropgz/pkg/embed/gz &&\
    cd dropgz/pkg/embed/gz/ && sha256sum * > sum.txt
RUN gzip --best --recursive dropgz/pkg/embed/gz && for f in dropgz/pkg/embed/gz/*.gz; do mv -- "$f" "${f%%.gz}"; done
RUN cd dropgz && CGO_ENABLED=0 go build -a -o ../bin/dropgz -trimpath -ldflags "-X github.com/Azure/azure-container-networking/dropgz/internal/buildinfo.Version="$VERSION"" -gcflags="-dwarflocationlists=true" main.go

FROM scratch
COPY --from=builder /etc/passwd /etc/passwd
COPY --from=builder /etc/group /etc/group
COPY --from=builder azure-container-networking/bin/dropgz /usr/local/bin/dropgz
ENTRYPOINT [ "/usr/local/bin/dropgz" ]
