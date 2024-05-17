package egress

//go:generate go run github.com/cilium/ebpf/cmd/bpf2go -target bpfel,bpfeb -go-package egress Egress ./bpf/egress.c -- -I./bpf/include
