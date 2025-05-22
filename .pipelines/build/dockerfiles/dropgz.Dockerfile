ARG ARTIFACT_DIR

FROM go AS dropgz
ARG DROPGZ_VERSION
ARG OS
ARG VERSION
ARG ARTIFACT_DIR

RUN go mod download github.com/azure/azure-container-networking/dropgz@$DROPGZ_VERSION
WORKDIR /go/pkg/mod/github.com/azure/azure-container-networking/dropgz\@$DROPGZ_VERSION
ADD ${ARTIFACT_DIR}/ pkg/embed/fs/
RUN GOOS=$OS CGO_ENABLED=0 go build -a -o /go/bin/dropgz -trimpath -ldflags "-X github.com/Azure/azure-container-networking/dropgz/internal/buildinfo.Version="$VERSION"" -gcflags="-dwarflocationlists=true" main.go


FROM scratch AS linux
COPY ${ARTIFACT_DIR}/bin/dropgz /dropgz
RUN chmod +x /dropgz
ENTRYPOINT [ "/dropgz" ]


# skopeo inspect docker://mcr.microsoft.com/oss/kubernetes/windows-host-process-containers-base-image:v1.0.0 --format "{{.Name}}@{{.Digest}}"
FROM mcr.microsoft.com/oss/kubernetes/windows-host-process-containers-base-image@sha256:b4c9637e032f667c52d1eccfa31ad8c63f1b035e8639f3f48a510536bf34032b as windows
COPY ${ARTIFACT_DIR}/bin/dropgz.exe /dropgz.exe
RUN chmod +x /dropgz.exe
ENTRYPOINT [ "/dropgz.exe" ]
