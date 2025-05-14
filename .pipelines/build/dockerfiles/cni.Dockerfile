ARG ARCH
ARG ARTIFACT_DIR

FROM scratch AS linux
ADD ${ARTIFACT_DIR}/bins/dropgz dropgz
ENTRYPOINT [ "/dropgz" ]


# mcr.microsoft.com/oss/kubernetes/windows-host-process-containers-base-image:v1.0.0
FROM --platform=windows/${ARCH} mcr.microsoft.com/oss/kubernetes/windows-host-process-containers-base-image@sha256:b4c9637e032f667c52d1eccfa31ad8c63f1b035e8639f3f48a510536bf34032b as hpc

FROM hpc as windows
ADD ${ARTIFACT_DIR}/bins/dropgz dropgz.exe
ENTRYPOINT [ "/dropgz.exe" ]


