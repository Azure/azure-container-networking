FROM golang as build
WORKDIR /go/src/github.com/Azure/azure-container-networking/
ADD . . 
ARG VERSION
RUN cd ./cni/installer && CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -o /go/installer -ldflags "-X main.version=${VERSION} -s -w"
RUN mv ./output /output
RUN chmod +x /go/installer
RUN find /output -name "*.zip" -type f -delete
RUN find /output -name "*.tgz" -type f -delete

FROM scratch
COPY --from=build /go/installer .
COPY --from=build /output /output
CMD ["./installer"]
