package main

import (
	"ebpf-tc-poc/pkg/egress"
	"ebpf-tc-poc/pkg/ingress"
	"log"
	"net"

	"github.com/cilium/ebpf/rlimit"
)

func main() {
	// Remove resource limits for kernels <5.11.
	if err := rlimit.RemoveMemlock(); err != nil {
		log.Printf("Removing memlock:", err)
	}

	ifname := "eth0"
	iface, err := net.InterfaceByName(ifname)
	if err != nil {
		log.Printf("Getting interface %s: %s", ifname, err)
	}
	log.Printf("Interface %s has index %d", ifname, iface.Index)

	// Load the compiled eBPF ELF and load it into the kernel.
	// Set up ingress and egress filters to attach to eth0 clsact qdisc
	// the qdisc already exists from cilium installation
	var objsEgress egress.EgressObjects
	defer objsEgress.Close()
	if err := egress.LoadEgressObjects(&objsEgress, nil); err != nil {
		log.Printf("Failed to load eBPF egress objects: %v", err)
	}
	if err := egress.SetupEgressFilter(iface.Index, &objsEgress); err != nil {
		log.Printf("Setting up egress filter:", err)
	} else {
		log.Printf("Successfully set egress filter on %s..", ifname)
	}

	var objsIngress ingress.IngressObjects
	if err := ingress.LoadIngressObjects(&objsIngress, nil); err != nil {
		log.Printf("Loading eBPF ingress objects:", err)
	}
	defer objsIngress.Close()
	if err := ingress.SetupIngressFilter(iface.Index, &objsIngress); err != nil {
		log.Printf("Setting up ingress filter:", err)
	} else {
		log.Printf("Successfully set ingress filter on %s..", ifname)
	}

}
