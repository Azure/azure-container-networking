ARG ARCH

FROM mcr.microsoft.com/oss/v2/fluent/fluent-bit:v4.2.2 as linux
ARG ARTIFACT_DIR
COPY ${ARTIFACT_DIR}/bin/out_azure_app_insights.so /out_azure_app_insights.so
