package blockservice

//go:generate go run github.com/cilium/ebpf/cmd/bpf2go -target bpfel,bpfeb BlockIptables ../../bpf/src/block_iptables.bpf.c -- -I../../bpf/include
