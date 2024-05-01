package ingress

//go:generate go run github.com/cilium/ebpf/cmd/bpf2go -target bpfel,bpfeb -go-package ingress Ingress ./bpf/ingress.c -- -I./bpf/include
