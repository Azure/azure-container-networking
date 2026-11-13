package network

import (
	"testing"

	"github.com/Azure/azure-container-networking/cni"
	"github.com/Azure/azure-container-networking/cns"
	"github.com/Azure/azure-container-networking/network"
	cnitypes "github.com/containernetworking/cni/pkg/types/100"
)

func TestCiliumIPAMUsesDelegatedHostInterface(t *testing.T) {
	cfg := &cni.NetworkConfig{IPAM: cni.IPAM{Type: cni.AzureCiliumIPAM}}
	results := getSingleResult("10.0.0.10/24")
	results[0].Interfaces = []*cnitypes.Interface{{Name: eth0IfName}}
	delegate := &mockDelegatePlugin{add: add{resultsIPv4: results}}
	invoker := AzureIPAMInvoker{plugin: delegate, nwInfo: &network.EndpointInfo{}}
	got, err := invoker.Add(IPAMAddConfig{nwCfg: cfg})
	if err != nil {
		t.Fatal(err)
	}
	ifInfo := got.interfaceInfo[invoker.getInterfaceInfoKey(cns.InfraNIC)]
	plugin := &NetPlugin{netClient: &InterfaceGetterMock{}}
	master := plugin.findMasterInterface(&createEpInfoOpt{
		ifInfo: &ifInfo, ipamAddConfig: &IPAMAddConfig{nwCfg: cfg},
	})
	if master != eth0IfName {
		t.Fatalf("master = %q; want delegated interface %q without subnet discovery", master, eth0IfName)
	}
}

type recordingCiliumDelegate struct {
	mockDelegatePlugin
	released string
}

func (d *recordingCiliumDelegate) DelegateDel(_ string, cfg *cni.NetworkConfig) error {
	d.released = cfg.IPAM.Address
	return nil
}

func TestCiliumIPAMReleasesMalformedInterfaceResult(t *testing.T) {
	for _, interfaces := range [][]*cnitypes.Interface{nil, {nil}, {{Name: eth0IfName, Sandbox: "/pod"}}} {
		cfg := &cni.NetworkConfig{IPAM: cni.IPAM{Type: cni.AzureCiliumIPAM}}
		results := getSingleResult("10.0.0.10/24")
		results[0].Interfaces = interfaces
		delegate := &recordingCiliumDelegate{mockDelegatePlugin: mockDelegatePlugin{add: add{resultsIPv4: results}}}
		invoker := AzureIPAMInvoker{plugin: delegate, nwInfo: &network.EndpointInfo{}}
		if _, err := invoker.Add(IPAMAddConfig{nwCfg: cfg}); err == nil || delegate.released != "10.0.0.10" {
			t.Fatalf("invalid result was not rejected and released: %v, released=%q", err, delegate.released)
		}
	}
}
