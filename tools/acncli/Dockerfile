FROM golang:1.17 as build
WORKDIR /go/src/github.com/Azure/azure-container-networking/
ARG VERSION
ARG PLATFORM
ADD . . 
RUN make all-binaries
RUN make acncli
RUN rm -rf ./output/windows*
RUN rm -rf ./output/${PLATFORM:-linux_amd64}/npm/*
RUN mv ./output /output
RUN find /output -name "*.zip" -type f -delete
RUN find /output -name "*.tgz" -type f -delete

FROM scratch
ARG PLATFORM
COPY --from=build /output/${PLATFORM:-linux_amd64}/acncli/ .
COPY --from=build /output /output
ENV AZURE_CNI_OS=linux
ENV AZURE_CNI_TENANCY=singletenancy
ENV AZURE_CNI_IPAM=azure-cns
ENV AZURE_CNI_MODE=transparent
ENTRYPOINT ["./acn", "cni", "manager", "--follow", "--mode", "transparent"]
