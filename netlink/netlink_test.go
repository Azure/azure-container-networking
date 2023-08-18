// Copyright 2017 Microsoft. All rights reserved.
// MIT License

//go:build linux
// +build linux

package netlink

import (
	"net"
	"testing"

	"github.com/stretchr/testify/require"
	"golang.org/x/sys/unix"
)

const (
	ifName    = "nltest"
	ifName2   = "nltest2"
	dummyName = "dummy1"
)

// AddDummyInterface creates a dummy test interface used during actual tests.
func addDummyInterface(name string) (*net.Interface, error) {
	nl := NewNetlink()
	err := nl.AddLink(&DummyLink{
		LinkInfo: LinkInfo{
			Type: LINK_TYPE_DUMMY,
			Name: name,
		},
	})
	if err != nil {
		return nil, err
	}

	dummy, err := net.InterfaceByName(name)
	if err != nil {
		return nil, err
	}

	return dummy, err
}

// TestEcho tests basic netlink messaging via echo.
func TestEcho(t *testing.T) {
	err := Echo("this is a test")
	if err != nil {
		t.Errorf("Echo failed: %+v", err)
	}
}

// TestAddDeleteBridge tests adding and deleting an ethernet bridge.
func TestAddDeleteBridge(t *testing.T) {
	link := BridgeLink{
		LinkInfo: LinkInfo{
			Type: LINK_TYPE_BRIDGE,
			Name: ifName,
		},
	}
	nl := NewNetlink()

	err := nl.AddLink(&link)
	if err != nil {
		t.Errorf("AddLink failed: %+v", err)
	}

	err = nl.DeleteLink(ifName)
	if err != nil {
		t.Errorf("DeleteLink failed: %+v", err)
	}

	_, err = net.InterfaceByName(ifName)
	if err == nil {
		t.Errorf("Interface not deleted")
	}
}

// TestAddDeleteVEth tests adding and deleting a virtual ethernet pair.
func TestAddDeleteVEth(t *testing.T) {
	link := VEthLink{
		LinkInfo: LinkInfo{
			Type: LINK_TYPE_VETH,
			Name: ifName,
		},
		PeerName: ifName2,
	}
	nl := NewNetlink()

	err := nl.AddLink(&link)
	if err != nil {
		t.Errorf("AddLink failed: %+v", err)
	}

	err = nl.DeleteLink(ifName)
	if err != nil {
		t.Errorf("DeleteLink failed: %+v", err)
	}

	_, err = net.InterfaceByName(ifName)
	if err == nil {
		t.Errorf("Interface not deleted")
	}
}

// TestSetMTU tests if MTU can be sent on link
func TestSetMTU(t *testing.T) {
	link := VEthLink{
		LinkInfo: LinkInfo{
			Type: LINK_TYPE_VETH,
			Name: ifName,
		},
		PeerName: ifName2,
	}
	nl := NewNetlink()

	err := nl.AddLink(&link)
	if err != nil {
		t.Errorf("AddLink failed: %+v", err)
	}

	//nolint:errcheck // not testing deletelink here
	defer nl.DeleteLink(ifName)

	if err = nl.SetLinkMTU(ifName, 1028); err != nil {
		t.Errorf("SetMTU failed: %+v", err)
	}

	iface, err := net.InterfaceByName(ifName)
	if err != nil {
		t.Errorf("InterfaceByName err:%v", err)
	}

	require.Equal(t, 1028, iface.MTU, "Expected mtu:1024 but got %d", iface.MTU)
}

// TestAddDeleteIPVlan tests adding and deleting an IPVLAN interface.
func TestAddDeleteIPVlan(t *testing.T) {
	dummy, err := addDummyInterface(dummyName)
	if err != nil {
		t.Errorf("addDummyInterface failed: %v", err)
	}

	link := IPVlanLink{
		LinkInfo: LinkInfo{
			Type:        LINK_TYPE_IPVLAN,
			Name:        ifName,
			ParentIndex: dummy.Index,
		},
		Mode: IPVLAN_MODE_L2,
	}
	nl := NewNetlink()

	err = nl.AddLink(&link)
	if err != nil {
		t.Errorf("AddLink failed: %+v", err)
	}

	err = nl.DeleteLink(ifName)
	if err != nil {
		t.Errorf("DeleteLink failed: %+v", err)
	}

	_, err = net.InterfaceByName(ifName)
	if err == nil {
		t.Errorf("Interface not deleted")
	}

	err = nl.DeleteLink(dummyName)
	if err != nil {
		t.Errorf("DeleteLink failed: %v", err)
	}
}

// TestSetLinkState tests setting the operational state of a network interface.
func TestSetLinkState(t *testing.T) {
	_, err := addDummyInterface(ifName)
	if err != nil {
		t.Errorf("addDummyInterface failed: %v", err)
	}
	nl := NewNetlink()

	err = nl.SetLinkState(ifName, true)
	if err != nil {
		t.Errorf("SetLinkState up failed: %+v", err)
	}

	dummy, err := net.InterfaceByName(ifName)
	if err != nil || (dummy.Flags&net.FlagUp) == 0 {
		t.Errorf("Interface not up")
	}

	err = nl.SetLinkState(ifName, false)
	if err != nil {
		t.Errorf("SetLinkState down failed: %+v", err)
	}

	dummy, err = net.InterfaceByName(ifName)
	if err != nil || (dummy.Flags&net.FlagUp) != 0 {
		t.Errorf("Interface not down")
	}

	err = nl.DeleteLink(ifName)
	if err != nil {
		t.Errorf("DeleteLink failed: %+v", err)
	}
}

// TestSetLinkPromisc tests setting the promiscuous mode of a network interface.
func TestSetLinkPromisc(t *testing.T) {
	_, err := addDummyInterface(ifName)
	if err != nil {
		t.Errorf("addDummyInterface failed: %v", err)
	}
	nl := NewNetlink()

	err = nl.SetLinkPromisc(ifName, true)
	if err != nil {
		t.Errorf("SetLinkPromisc on failed: %+v", err)
	}

	err = nl.SetLinkPromisc(ifName, false)
	if err != nil {
		t.Errorf("SetLinkPromisc off failed: %+v", err)
	}

	err = nl.DeleteLink(ifName)
	if err != nil {
		t.Errorf("DeleteLink failed: %+v", err)
	}
}

// TestSetHairpinMode tests setting the hairpin mode of a bridged interface.
func TestSetLinkHairpin(t *testing.T) {
	link := BridgeLink{
		LinkInfo: LinkInfo{
			Type: LINK_TYPE_BRIDGE,
			Name: ifName,
		},
	}
	nl := NewNetlink()

	err := nl.AddLink(&link)
	if err != nil {
		t.Errorf("AddLink failed: %+v", err)
	}

	_, err = addDummyInterface(ifName2)
	if err != nil {
		t.Errorf("addDummyInterface failed: %v", err)
	}

	err = nl.SetLinkMaster(ifName2, ifName)
	if err != nil {
		t.Errorf("SetLinkMaster failed: %+v", err)
	}

	err = nl.SetLinkHairpin(ifName2, true)
	if err != nil {
		t.Errorf("SetLinkHairpin on failed: %+v", err)
	}

	err = nl.SetLinkHairpin(ifName2, false)
	if err != nil {
		t.Errorf("SetLinkHairpin off failed: %+v", err)
	}

	err = nl.DeleteLink(ifName2)
	if err != nil {
		t.Errorf("DeleteLink failed: %+v", err)
	}

	err = nl.DeleteLink(ifName)
	if err != nil {
		t.Errorf("DeleteLink failed: %+v", err)
	}
}

func TestAddRemoveStaticArp(t *testing.T) {
	_, err := addDummyInterface(ifName)
	if err != nil {
		t.Errorf("addDummyInterface failed: %v", err)
	}

	ip := net.ParseIP("192.168.0.2")
	mac, _ := net.ParseMAC("aa:b3:4d:5e:e2:4a")
	nl := NewNetlink()

	linkInfo := LinkInfo{
		Name:       ifName,
		IPAddr:     ip,
		MacAddress: mac,
	}

	err = nl.SetOrRemoveLinkAddress(linkInfo, ADD, NUD_PERMANENT)
	if err != nil {
		t.Errorf("ret val %v", err)
	}

	linkInfo = LinkInfo{
		Name:       ifName,
		IPAddr:     ip,
		MacAddress: mac,
	}

	err = nl.SetOrRemoveLinkAddress(linkInfo, REMOVE, NUD_INCOMPLETE)
	if err != nil {
		t.Errorf("ret val %v", err)
	}

	err = nl.DeleteLink(ifName)
	if err != nil {
		t.Errorf("DeleteLink failed: %+v", err)
	}
}

func TestAddRemoveIPAddress(t *testing.T) {
	_, err := addDummyInterface(ifName)
	if err != nil {
		t.Errorf("addDummyInterface failed: %v", err)
	}

	ip := net.ParseIP("192.168.0.4")
	_, ipNet, _ := net.ParseCIDR("192.168.0.4/24")
	nl := NewNetlink()

	err = nl.setIPAddress(ifName, ip, ipNet, true)
	if err != nil {
		t.Errorf("ret val %v", err)
	}

	err = nl.setIPAddress(ifName, ip, ipNet, false)
	if err != nil {
		t.Errorf("ret val %v", err)
	}

	err = nl.DeleteLink(ifName)
	if err != nil {
		t.Errorf("DeleteLink failed: %+v", err)
	}
}

func TestAddDeleteRoute(t *testing.T) {
	_, err := addDummyInterface(ifName)
	if err != nil {
		t.Errorf("addDummyInterface failed: %v", err)
	}

	nl := NewNetlink()
	err = nl.SetLinkState(ifName, true)
	if err != nil {
		t.Errorf("ret val %v", err)
	}
	_, dstIPNet, _ := net.ParseCIDR("192.168.0.4/24")
	netif, _ := net.InterfaceByName(ifName)

	route := Route{
		Family:    unix.AF_INET,
		Dst:       dstIPNet,
		LinkIndex: netif.Index,
		Scope:     RT_SCOPE_LINK,
	}

	err = setIpRoute(&route, true)
	if err != nil {
		t.Errorf("ret val %v", err)
	}

	err = setIpRoute(&route, false)
	if err != nil {
		t.Errorf("ret val %v", err)
	}

	err = nl.DeleteLink(ifName)
	if err != nil {
		t.Errorf("DeleteLink failed: %+v", err)
	}
}
