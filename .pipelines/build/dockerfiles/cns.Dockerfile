ARG ARCH

# mcr.microsoft.com/oss/kubernetes/windows-host-process-containers-base-image:v1.0.0
FROM --platform=windows/${ARCH} mcr.microsoft.com/oss/kubernetes/windows-host-process-containers-base-image:v1.0.0@sha256:b4c9637e032f667c52d1eccfa31ad8c63f1b035e8639f3f48a510536bf34032b AS windows
ARG ARTIFACT_DIR .

COPY ${ARTIFACT_DIR}/bin/azure-cns.exe /azure-cns.exe
ENTRYPOINT ["azure-cns.exe"]
EXPOSE 10090

# mcr.microsoft.com/azurelinux/base/core:3.0
FROM --platform=linux/${ARCH} mcr.microsoft.com/azurelinux/base/core:3.0@sha256:bd88359666e3f20296e7fe639a480dfd3c3c8eba299b21f26e4560353e21a656 AS build-helper
RUN tdnf install -y iptables

# mcr.microsoft.com/azurelinux/distroless/base:3.0
FROM --platform=linux/${ARCH} mcr.microsoft.com/azurelinux/distroless/base:3.0@sha256:e952865122598043cfb54f0257f016e28e02c21904e172b47dcb2148e095693a AS linux
ARG ARTIFACT_DIR .

COPY --from=build-helper /usr/sbin/*tables* /usr/sbin/
COPY --from=build-helper /usr/lib /usr/lib
COPY ${ARTIFACT_DIR}/bin/azure-cns /usr/local/bin/azure-cns
ENTRYPOINT [ "/usr/local/bin/azure-cns" ]
EXPOSE 10090
