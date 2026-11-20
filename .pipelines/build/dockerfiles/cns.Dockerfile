ARG ARCH

# mcr.microsoft.com/oss/kubernetes/windows-host-process-containers-base-image:v1.0.0
FROM --platform=windows/${ARCH} mcr.microsoft.com/oss/kubernetes/windows-host-process-containers-base-image:v1.0.0@sha256:b4c9637e032f667c52d1eccfa31ad8c63f1b035e8639f3f48a510536bf34032b AS windows
ARG ARTIFACT_DIR .

COPY ${ARTIFACT_DIR}/bin/azure-cns.exe /azure-cns.exe
ENTRYPOINT ["azure-cns.exe"]
EXPOSE 10090

# mcr.microsoft.com/azurelinux/base/core:3.0
FROM --platform=linux/${ARCH} mcr.microsoft.com/azurelinux/base/core:3.0@sha256:3d16d8799f71a78f5bddb3b7cda4a57df1f40ecfa58bac89933010596aa86880 AS build-helper
RUN tdnf install -y iptables

# mcr.microsoft.com/azurelinux/distroless/base:3.0
FROM --platform=linux/${ARCH} mcr.microsoft.com/azurelinux/distroless/base:3.0@sha256:11ecd8f7cd1cad0e4fbb565ab98768f1a2ce35cb2730c867217baac011ae8645 AS linux
ARG ARTIFACT_DIR .

COPY --from=build-helper /usr/sbin/*tables* /usr/sbin/
COPY --from=build-helper /usr/lib /usr/lib
COPY ${ARTIFACT_DIR}/bin/azure-cns /usr/local/bin/azure-cns
ENTRYPOINT [ "/usr/local/bin/azure-cns" ]
EXPOSE 10090
