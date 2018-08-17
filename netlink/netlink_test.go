// Copyright 2017 Microsoft. All rights reserved.
// MIT License

package netlink

import (
	"net"
	"testing"
)

const (
	ifName    = "nltest"
	ifName2   = "nltest2"
	dummyName = "dummy1"
)

// AddDummyInterface creates a dummy test interface used during actual tests.
func addDummyInterface(name string) (*net.Interface, error) {
	err := AddLink(&DummyLink{
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

	err := AddLink(&link)
	if err != nil {
		t.Errorf("AddLink failed: %+v", err)
	}

	err = DeleteLink(ifName)
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

	err := AddLink(&link)
	if err != nil {
		t.Errorf("AddLink failed: %+v", err)
	}

	err = DeleteLink(ifName)
	if err != nil {
		t.Errorf("DeleteLink failed: %+v", err)
	}

	_, err = net.InterfaceByName(ifName)
	if err == nil {
		t.Errorf("Interface not deleted")
	}
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

	err = AddLink(&link)
	if err != nil {
		t.Errorf("AddLink failed: %+v", err)
	}

	err = DeleteLink(ifName)
	if err != nil {
		t.Errorf("DeleteLink failed: %+v", err)
	}

	_, err = net.InterfaceByName(ifName)
	if err == nil {
		t.Errorf("Interface not deleted")
	}

	err = DeleteLink(dummyName)
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

	err = SetLinkState(ifName, true)
	if err != nil {
		t.Errorf("SetLinkState up failed: %+v", err)
	}

	dummy, err := net.InterfaceByName(ifName)
	if err != nil || (dummy.Flags&net.FlagUp) == 0 {
		t.Errorf("Interface not up")
	}

	err = SetLinkState(ifName, false)
	if err != nil {
		t.Errorf("SetLinkState down failed: %+v", err)
	}

	dummy, err = net.InterfaceByName(ifName)
	if err != nil || (dummy.Flags&net.FlagUp) != 0 {
		t.Errorf("Interface not down")
	}

	err = DeleteLink(ifName)
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

	err = SetLinkPromisc(ifName, true)
	if err != nil {
		t.Errorf("SetLinkPromisc on failed: %+v", err)
	}

	err = SetLinkPromisc(ifName, false)
	if err != nil {
		t.Errorf("SetLinkPromisc off failed: %+v", err)
	}

	err = DeleteLink(ifName)
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

	err := AddLink(&link)
	if err != nil {
		t.Errorf("AddLink failed: %+v", err)
	}

	_, err = addDummyInterface(ifName2)
	if err != nil {
		t.Errorf("addDummyInterface failed: %v", err)
	}

	err = SetLinkMaster(ifName2, ifName)
	if err != nil {
		t.Errorf("SetLinkMaster failed: %+v", err)
	}

	err = SetLinkHairpin(ifName2, true)
	if err != nil {
		t.Errorf("SetLinkHairpin on failed: %+v", err)
	}

	err = SetLinkHairpin(ifName2, false)
	if err != nil {
		t.Errorf("SetLinkHairpin off failed: %+v", err)
	}

	err = DeleteLink(ifName2)
	if err != nil {
		t.Errorf("DeleteLink failed: %+v", err)
	}

	err = DeleteLink(ifName)
	if err != nil {
		t.Errorf("DeleteLink failed: %+v", err)
	}
}
