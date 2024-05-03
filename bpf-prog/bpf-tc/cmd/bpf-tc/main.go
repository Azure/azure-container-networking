package main

import (
	"net"

	"github.com/Azure/azure-container-networking/bpf-prog/bpf-tc/pkg/egress"
	"github.com/Azure/azure-container-networking/bpf-prog/bpf-tc/pkg/ingress"

	"github.com/cilium/ebpf/rlimit"
	"go.uber.org/zap"
)

var logger *zap.Logger

func main() {
	// Set up logger
	logger, _ = zap.NewProduction()
	defer logger.Sync()

	// Remove resource limits for kernels <5.11.
	if err := rlimit.RemoveMemlock(); err != nil {
		logger.Error("Removing memlock", zap.Error(err))
	}
	ifname := "eth0"
	iface, err := net.InterfaceByName(ifname)
	if err != nil {
		logger.Error("Getting interface", zap.String("interface", ifname), zap.Error(err))
	}
	logger.Info("Interface has index", zap.String("interface", ifname), zap.Int("index", iface.Index))

	// Load the compiled eBPF ELF and load it into the kernel.
	// Set up ingress and egress filters to attach to eth0 clsact qdisc
	// the qdisc already exists from cilium installation
	var objsEgress egress.EgressObjects
	defer objsEgress.Close()
	if err := egress.LoadEgressObjects(&objsEgress, nil); err != nil {
		logger.Error("Failed to load eBPF egress objects", zap.Error(err))
	}
	if err := egress.SetupEgressFilter(iface.Index, &objsEgress, logger); err != nil {
		logger.Error("Setting up egress filter", zap.Error(err))
	} else {
		logger.Info("Successfully set egress filter on", zap.String("interface", ifname))
	}

	var objsIngress ingress.IngressObjects
	if err := ingress.LoadIngressObjects(&objsIngress, nil); err != nil {
		logger.Error("Loading eBPF ingress objects", zap.Error(err))
	}
	defer objsIngress.Close()
	if err := ingress.SetupIngressFilter(iface.Index, &objsIngress, logger); err != nil {
		logger.Error("Setting up ingress filter", zap.Error(err))
	} else {
		logger.Info("Successfully set ingress filter on", zap.String("interface", ifname))
	}
}
