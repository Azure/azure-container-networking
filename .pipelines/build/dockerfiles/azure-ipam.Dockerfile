ARG ARTIFACT_DIR

FROM scratch AS linux
COPY ${ARTIFACT_DIR}/bin/dropgz /dropgz
ENTRYPOINT [ "/dropgz" ]


# skopeo inspect docker://mcr.microsoft.com/oss/kubernetes/windows-host-process-containers-base-image:v1.0.0 --format "{{.Name}}@{{.Digest}}"
FROM mcr.microsoft.com/oss/kubernetes/windows-host-process-containers-base-image@sha256:b4c9637e032f667c52d1eccfa31ad8c63f1b035e8639f3f48a510536bf34032b as windows
COPY ${ARTIFACT_DIR}/bin/dropgz.exe /dropgz.exe
ENTRYPOINT [ "/dropgz.exe" ]
