package snat

import (
	"os"
	"testing"

	"github.com/Azure/azure-container-networking/netio"
	"github.com/Azure/azure-container-networking/netlink"
)

var anyInterface = "dummy"

type mockIPTablesClient struct{}

func (c mockIPTablesClient) InsertIptableRule(_, _, _, _, _ string) error {
	return nil
}

func (c mockIPTablesClient) AppendIptableRule(_, _, _, _, _ string) error {
	return nil
}

func (c mockIPTablesClient) DeleteIptableRule(_, _, _, _, _ string) error {
	return nil
}

func (c mockIPTablesClient) CreateChain(_, _, _ string) error {
	return nil
}

func TestMain(m *testing.M) {
	exitCode := m.Run()

	// Create a dummy test network interface.

	os.Exit(exitCode)
}

func GetTestClient(nl netlink.NetlinkInterface, iptc ipTablesClient, nio netio.NetIOInterface) *Client {
	return &Client{
		SnatBridgeIP:          "169.254.0.1/16",
		localIP:               "169.254.0.4/16",
		containerSnatVethName: anyInterface,
		netlink:               nl,
		ipTablesClient:        iptc,
		netioClient:           nio,
	}
}

func TestAllowInboundFromHostToNC(t *testing.T) {
	nl := netlink.NewMockNetlink(false, "")
	iptc := &mockIPTablesClient{}
	nio := netio.NewMockNetIO(false, 0)
	client := GetTestClient(nl, iptc, nio)

	if err := nl.AddLink(&netlink.DummyLink{
		LinkInfo: netlink.LinkInfo{
			Type: netlink.LINK_TYPE_DUMMY,
			Name: anyInterface,
		},
	}); err != nil {
		t.Errorf("Error adding dummy interface %v", err)
	}

	if err := nl.AddLink(&netlink.DummyLink{
		LinkInfo: netlink.LinkInfo{
			Type: netlink.LINK_TYPE_DUMMY,
			Name: SnatBridgeName,
		},
	}); err != nil {
		t.Errorf("Error adding dummy interface %v", err)
	}

	if err := client.AllowInboundFromHostToNC(); err != nil {
		t.Errorf("Error adding inbound rule: %v", err)
	}

	if err := client.AllowInboundFromHostToNC(); err != nil {
		t.Errorf("Error adding existing inbound rule: %v", err)
	}

	if err := client.DeleteInboundFromHostToNC(); err != nil {
		t.Errorf("Error removing inbound rule: %v", err)
	}

	if err := nl.DeleteLink(anyInterface); err != nil {
		t.Errorf("Error removing any interface link: %v", err)
	}
	if err := nl.DeleteLink(SnatBridgeName); err != nil {
		t.Errorf("Error removing snat bridge: %v", err)
	}

	client.netioClient = netio.NewMockNetIO(true, 1)
	if err := client.AllowInboundFromHostToNC(); err == nil {
		t.Errorf("Expected error when interface not found in allow host to nc but got nil")
	}
}

func TestAllowInboundFromNCToHost(t *testing.T) {
	nl := netlink.NewMockNetlink(false, "")
	iptc := &mockIPTablesClient{}
	nio := netio.NewMockNetIO(false, 0)
	client := GetTestClient(nl, iptc, nio)

	if err := nl.AddLink(&netlink.DummyLink{
		LinkInfo: netlink.LinkInfo{
			Type: netlink.LINK_TYPE_DUMMY,
			Name: anyInterface,
		},
	}); err != nil {
		t.Errorf("Error adding dummy interface %v", err)
	}

	if err := nl.AddLink(&netlink.DummyLink{
		LinkInfo: netlink.LinkInfo{
			Type: netlink.LINK_TYPE_DUMMY,
			Name: SnatBridgeName,
		},
	}); err != nil {
		t.Errorf("Error adding dummy interface %v", err)
	}

	if err := client.AllowInboundFromNCToHost(); err != nil {
		t.Errorf("Error adding inbound rule: %v", err)
	}

	if err := client.AllowInboundFromNCToHost(); err != nil {
		t.Errorf("Error adding existing inbound rule: %v", err)
	}

	if err := client.DeleteInboundFromNCToHost(); err != nil {
		t.Errorf("Error removing inbound rule: %v", err)
	}

	if err := nl.DeleteLink(anyInterface); err != nil {
		t.Errorf("Error removing any interface link: %v", err)
	}
	if err := nl.DeleteLink(SnatBridgeName); err != nil {
		t.Errorf("Error removing snat bridge: %v", err)
	}

	client.netioClient = netio.NewMockNetIO(true, 1)
	if err := client.AllowInboundFromNCToHost(); err == nil {
		t.Errorf("Expected error when interface not found in allow nc to host but got nil")
	}
}
