package main

import (
	"context"
	"testing"

	"github.com/containernetworking/cni/pkg/skel"
)

func TestResultPreservesHostAddressAndSubnetMask(t *testing.T) {
	result, err := allocationResult("10.0.0.10", "10.0.0.0/24", "eth0", "1.0.0")
	if err != nil {
		t.Fatal(err)
	}
	if len(result.IPs) != 1 || result.IPs[0].Address.String() != "10.0.0.10/24" ||
		result.IPs[0].Gateway.String() != "10.0.0.1" {
		t.Fatalf("incorrect delegated result: %+v", result)
	}
	if len(result.Interfaces) != 1 || result.Interfaces[0].Name != "eth0" || result.Interfaces[0].Sandbox != "" {
		t.Fatalf("selected host interface missing: %+v", result.Interfaces)
	}
}

func TestInvalidCNIInputFailsBeforeClientOrGuestDiscovery(t *testing.T) {
	if err := cniCommand(context.Background(), "ADD", &skel.CmdArgs{StdinData: []byte(`{"name":"azure","ipam":{"type":"azure-vnet-ipam"}}`)}); err == nil {
		t.Fatal("wrong plugin type accepted")
	}
}
