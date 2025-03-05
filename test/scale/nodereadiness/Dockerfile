FROM mcr.microsoft.com/oss/go/microsoft/golang:1.23 as builder
WORKDIR /app
COPY . .
RUN go mod tidy && go build -o nncscale latency.go

FROM gcr.io/distroless/base
COPY --from=builder /app/nncscale /nncscale
EXPOSE 2112
ENTRYPOINT ["/nncscale"]