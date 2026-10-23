ARG ARCH

FROM mcr.microsoft.com/oss/v2/fluent/fluent-bit:v5.1.2-6 as linux
ARG ARTIFACT_DIR
COPY ${ARTIFACT_DIR}/bin/out_azure_app_insights.so /out_azure_app_insights.so
