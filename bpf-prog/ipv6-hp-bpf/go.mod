module github.com/Azure/azure-container-networking/bpf-prog/ipv6-hp-bpf

go 1.25.0

toolchain go1.26.7

require (
	github.com/cilium/ebpf v0.22.0
	github.com/vishvananda/netlink v1.3.1
	go.uber.org/zap v1.28.0
)

require (
	github.com/vishvananda/netns v0.0.5 // indirect
	go.uber.org/multierr v1.10.0 // indirect
	golang.org/x/sys v0.43.0 // indirect
)
