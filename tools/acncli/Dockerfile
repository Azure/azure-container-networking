FROM mcr.microsoft.com/oss/go/microsoft/golang:1.20 as build
WORKDIR /go/src/github.com/Azure/azure-container-networking/
ARG VERSION
ADD . . 
RUN make all-binaries
RUN make acncli
RUN rm -rf ./output/**/npm
RUN mv ./output /output
RUN find /output -name "*.zip" -type f -delete
RUN find /output -name "*.tgz" -type f -delete

FROM scratch
COPY --from=build /output/**/acncli/ .
COPY --from=build /output /output
ENV AZURE_CNI_OS=linux
ENV AZURE_CNI_TENANCY=singletenancy
ENV AZURE_CNI_IPAM=azure-cns
ENV AZURE_CNI_MODE=transparent
ENTRYPOINT ["./acn", "cni", "manager", "--follow", "--mode", "transparent"]
