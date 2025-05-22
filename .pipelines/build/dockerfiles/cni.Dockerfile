ARG ARCH


# skopeo inspect docker://mcr.microsoft.com/oss/kubernetes/windows-host-process-containers-base-image:v1.0.0 --format "{{.Name}}@{{.Digest}}"
FROM --platform=windows/${ARCH} mcr.microsoft.com/oss/kubernetes/windows-host-process-containers-base-image@sha256:b4c9637e032f667c52d1eccfa31ad8c63f1b035e8639f3f48a510536bf34032b as windows
ARG ARTIFACT_DIR .

COPY ${ARTIFACT_DIR}/bin/dropgz.exe /dropgz.exe
ENTRYPOINT [ "/dropgz.exe" ]


# skopeo inspect docker://mcr.microsoft.com/cbl-mariner/base/core:2.0 --format "{{.Name}}@{{.Digest}}"
FROM --platform=linux/${ARCH} mcr.microsoft.com/cbl-mariner/base/core@sha256:a490e0b0869dc570ae29782c2bc17643aaaad1be102aca83ce0b96e0d0d2d328 AS archive-helper
ARG ARTIFACT_DIR .

COPY ${ARTIFACT_DIR}/root_artifact.tar .
RUN tar xvf root_artifact.tar /artifacts/

FROM scratch AS linux
COPY --from=archive-helper /artifacts/bin/dropgz /dropgz
ENTRYPOINT [ "/dropgz" ]
