package network

import (
	"net"
	"testing"
)

func TestTransparentTunnelOldDeletePreservesReusedIPMembership(t *testing.T) {
	address := net.IPNet{IP: net.ParseIP("10.0.0.10"), Mask: net.CIDRMask(32, 32)}
	old := &endpoint{Id: "old", HostIfName: "old-veth", IPAddresses: []net.IPNet{address}}
	replacement := &endpoint{Id: "new", HostIfName: "new-veth", IPAddresses: []net.IPNet{address}}
	nw := &network{extIf: &externalInterface{Name: InfraInterfaceName}, Endpoints: map[string]*endpoint{"old": old, "new": replacement}}
	rules := &transparentTunnelMockIPTablesClient{}
	ipset := &transparentTunnelMockIpsetClient{}
	client := NewTransparentTunnelEndpointClient(nw, &EndpointInfo{Mode: opModeTransparentTunnel}, "old-veth", "", nil, nil, nil, rules)
	client.ipsetClient = ipset
	if err := client.deleteTransparentTunnelRules(old); err != nil {
		t.Fatal(err)
	}
	if ipset.countOps(ipsetOpDel) != 0 {
		t.Fatal("old DEL removed new endpoint's local-pod membership")
	}
	if len(rules.deleteCalls) != 1 || rules.deleteCalls[0].match != "-i old-veth" {
		t.Fatalf("old interface-specific rule not cleaned: %+v", rules.deleteCalls)
	}
	delete(nw.Endpoints, "new")
	if err := client.deleteTransparentTunnelRules(old); err != nil {
		t.Fatal(err)
	}
	if ipset.countOps(ipsetOpDel) != 1 {
		t.Fatal("last endpoint did not remove local-pod membership")
	}
}
