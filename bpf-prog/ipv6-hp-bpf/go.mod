module github.com/Azure/azure-container-networking/bpf-prog/ipv6-hp-bpf

go 1.23

toolchain go1.23.2

require (
	github.com/cilium/ebpf v0.15.0
	github.com/vishvananda/netlink v1.1.0
	go.uber.org/zap v1.27.0
)

require (
	github.com/vishvananda/netns v0.0.0-20191106174202-0a2b9b5464df // indirect
	go.uber.org/multierr v1.10.0 // indirect
	golang.org/x/exp v0.0.0-20230224173230-c95f2b4c22f2 // indirect
	golang.org/x/sys v0.15.0 // indirect
)
