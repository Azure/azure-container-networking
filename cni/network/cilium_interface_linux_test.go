package network

import (
	"testing"

	"github.com/Azure/azure-container-networking/cni"
	"github.com/Azure/azure-container-networking/cns"
	"github.com/Azure/azure-container-networking/network"
)

func TestCiliumIPAMUsesConfiguredMaster(t *testing.T) {
	cfg := &cni.NetworkConfig{Master: eth0IfName, IPAM: cni.IPAM{Type: cni.AzureCiliumIPAM}}
	results := getSingleResult("10.0.0.10/24")
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
		t.Fatalf("master = %q; want configured interface %q without subnet discovery", master, eth0IfName)
	}
}
