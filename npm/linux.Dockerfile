FROM mcr.microsoft.com/oss/go/microsoft/golang:1.23 AS builder
ARG VERSION
ARG NPM_AI_PATH
ARG NPM_AI_ID
RUN apt-get update && apt-get install -y iptables ipset ca-certificates conntrack grep && apt-get autoremove -y && apt-get clean
WORKDIR /usr/local/src
COPY . .
RUN CGO_ENABLED=0 go build -v -o /usr/local/bin/azure-npm -ldflags "-X main.version="$VERSION" -X "$NPM_AI_PATH"="$NPM_AI_ID"" -gcflags="-dwarflocationlists=true" npm/cmd/*.go
RUN chmod +x /usr/local/bin/azure-npm

FROM mcr.microsoft.com/cbl-mariner/distroless/minimal@sha256:63a0a70ceaa1320bc6eb98b81106667d43e46b674731ea8d28e4de1b87e0747f AS linux
COPY --from=builder /usr/local/bin/azure-npm /usr/bin/azure-npm
COPY --from=builder /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/ca-certificates.crt
COPY --from=builder /lib/ /lib
COPY --from=builder /usr/lib/ /usr/lib
COPY --from=builder /usr/sbin/ /usr/sbin/

# Copy iptables, iptables-nft, and iptables-nft-save binaries
COPY --from=builder /usr/sbin/iptables /usr/sbin/iptables
COPY --from=builder /usr/sbin/iptables-nft /usr/sbin/iptables-nft
COPY --from=builder /usr/sbin/iptables-restore /usr/sbin/iptables-restore
COPY --from=builder /usr/sbin/iptables-save /usr/sbin/iptables-save
COPY --from=builder /usr/sbin/iptables-nft-restore /usr/sbin/iptables-nft-restore
COPY --from=builder /usr/sbin/iptables-nft-save /usr/sbin/iptables-nft-save
COPY --from=builder /usr/sbin/conntrack /usr/sbin/conntrack
COPY --from=builder /bin/grep /bin/grep

# Copy required libraries based on ldd output
COPY --from=builder /lib/x86_64-linux-gnu/libxtables.so.12 /lib/x86_64-linux-gnu/libxtables.so.12
COPY --from=builder /lib/x86_64-linux-gnu/libmnl.so.0 /lib/x86_64-linux-gnu/libmnl.so.0
COPY --from=builder /lib/x86_64-linux-gnu/libnftnl.so.11 /lib/x86_64-linux-gnu/libnftnl.so.11
COPY --from=builder /lib/x86_64-linux-gnu/libnetfilter_conntrack.so.3 /lib/x86_64-linux-gnu/libnetfilter_conntrack.so.3
COPY --from=builder /lib/x86_64-linux-gnu/libnfnetlink.so.0 /lib/x86_64-linux-gnu/libnfnetlink.so.0
COPY --from=builder /lib/x86_64-linux-gnu/libc.so.6 /lib/x86_64-linux-gnu/libc.so.6
COPY --from=builder /lib64/ld-linux-x86-64.so.2 /lib64/ld-linux-x86-64.so.2

ENTRYPOINT ["/usr/bin/azure-npm", "start"]
