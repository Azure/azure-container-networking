FROM mcr.microsoft.com/oss/go/microsoft/golang:1.19 as build
WORKDIR /go/src/github.com/Azure/azure-container-networking/
ARG VERSION
ADD . . 
ENV GOOS=windows
RUN make all-binaries
RUN make acncli
RUN rm -rf ./output/**/npm
RUN mv ./output /output
RUN find /output -name "*.zip" -type f -delete
RUN find /output -name "*.tgz" -type f -delete

FROM scratch
COPY --from=build /output/**/acncli/ .
COPY --from=build /output /output
ENV AZURE_CNI_OS=windows
ENV AZURE_CNI_TENANCY=multitenancy
ENV AZURE_CNI_IPAM=azure-cns
ENV AZURE_CNI_MODE=bridge
ENTRYPOINT ["./acn", "cni", "manager", "--follow", "--mode", "bridge"]