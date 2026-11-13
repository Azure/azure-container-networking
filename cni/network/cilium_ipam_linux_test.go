package network

import (
	"errors"
	"os"
	"testing"

	"github.com/Azure/azure-container-networking/cni"
	"github.com/Azure/azure-container-networking/ipam"
	"github.com/Azure/azure-container-networking/network"
	"github.com/Azure/azure-container-networking/platform"
	cnitypes "github.com/containernetworking/cni/pkg/types/100"
)

const testLegacyIPAM = "azure-vnet-ipam"

func TestCiliumIPAMRejectsUnsupportedDispatch(t *testing.T) {
	valid := cni.NetworkConfig{Mode: "transparent-tunnel", IPAM: cni.IPAM{Type: "azure-cilium-ipam"}}
	if err := validateCiliumIPAMConfig(&valid); err != nil {
		t.Fatal(err)
	}
	for _, change := range []func(*cni.NetworkConfig){
		func(c *cni.NetworkConfig) { c.Mode = "transparent" },
		func(c *cni.NetworkConfig) { c.ExecutionMode = "baremetal" },
		func(c *cni.NetworkConfig) { c.MultiTenancy = true },
		func(c *cni.NetworkConfig) { c.IPV6Mode = "ipv6" },
	} {
		cfg := valid
		change(&cfg)
		if err := validateCiliumIPAMConfig(&cfg); err == nil {
			t.Fatal("unsupported dispatch accepted")
		}
		cfg.IPAM.Type = testLegacyIPAM
		if err := validateCiliumIPAMConfig(&cfg); err != nil {
			t.Fatal("legacy configuration changed")
		}
	}
}

type countingDelegate struct {
	mockDelegatePlugin
	calls int
}

func (d *countingDelegate) DelegateAdd(string, *cni.NetworkConfig) (*cnitypes.Result, error) {
	d.calls++
	return nil, ipam.ErrNoAvailableAddressPools
}

func TestCiliumIPAMDoesNotRetryThroughLegacyStateReset(t *testing.T) {
	for _, path := range []string{platform.CNIStateFilePath, platform.CNIIpamStatePath} {
		if _, err := os.Lstat(path); !errors.Is(err, os.ErrNotExist) {
			t.Skip("host has CNI state; do not exercise legacy reset path")
		}
	}
	delegate := &countingDelegate{}
	invoker := AzureIPAMInvoker{plugin: delegate, nwInfo: &network.EndpointInfo{}}
	_, err := invoker.Add(IPAMAddConfig{nwCfg: &cni.NetworkConfig{IPAM: cni.IPAM{Type: "azure-cilium-ipam"}}})
	if err == nil || delegate.calls != 1 {
		t.Fatalf("calls=%d, error=%v; want one failed call", delegate.calls, err)
	}
}
