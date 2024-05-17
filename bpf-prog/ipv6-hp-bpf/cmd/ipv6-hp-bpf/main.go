package main

import (
	"bytes"
	"net"
	"os/exec"

	"github.com/Azure/azure-container-networking/bpf-prog/ipv6-hp-bpf/pkg/egress"
	"github.com/Azure/azure-container-networking/bpf-prog/ipv6-hp-bpf/pkg/ingress"
	"github.com/vishvananda/netlink"

	"github.com/cilium/ebpf/rlimit"
	"go.uber.org/zap"
)

var logger *zap.Logger

func main() {
	// Set up logger
	config := zap.NewProductionConfig()
	config.OutputPaths = []string{"stdout", "/var/log/azure-ipv6-hp-bpf.log"}
	logger, _ = config.Build()

	// Remove resource limits for kernels <5.11.
	if err := rlimit.RemoveMemlock(); err != nil {
		logger.Error("Removing memlock", zap.Error(err))
		return
	}

	// Check 'nft -n list tables ip6' to see if table exists
	cmd := exec.Command("nft", "-n", "list", "tables", "ip6")
	output, err := cmd.CombinedOutput()
	if err != nil {
		logger.Error("error running 'nft -n list tables ip6'", zap.Error(err), zap.String("output", string(output)))
		return
	}

	// if azureSLBProbe table exists, delete it
	if bytes.Contains(output, []byte("azureSLBProbe")) {
		cmd := exec.Command("nft", "delete", "table", "ip6", "azureSLBProbe")
		err = cmd.Run()
		if err != nil {
			logger.Error("failed to run 'nft delete table ip6 azureSLBProbe'", zap.Error(err))
			return
		}
	}

	ifname := "eth0"
	iface, err := net.InterfaceByName(ifname)
	if err != nil {
		logger.Error("Getting interface", zap.String("interface", ifname), zap.Error(err))
	}
	logger.Info("Interface has index", zap.String("interface", ifname), zap.Int("index", iface.Index))

	// Create a qdisc filter for traffic on the interface.
	fq := &netlink.GenericQdisc{
		QdiscAttrs: netlink.QdiscAttrs{
			LinkIndex: iface.Index,
			Handle:    netlink.MakeHandle(0xffff, 0),
			Parent:    netlink.HANDLE_CLSACT,
		},
		QdiscType: "clsact",
	}
	if err := netlink.QdiscReplace(fq); err != nil {
		logger.Error("failed setting egress qdisc", zap.Error(err))
		return
	}

	// Load the compiled eBPF ELF and load it into the kernel.
	// Set up ingress and egress filters to attach to eth0 clsact qdisc
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
