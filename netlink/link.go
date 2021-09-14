// Copyright 2017 Microsoft. All rights reserved.
// MIT License

//go:build linux
// +build linux

package netlink

import (
	"fmt"
	"net"

	"github.com/Azure/azure-container-networking/log"
	"golang.org/x/sys/unix"
)

// Link types.
const (
	LINK_TYPE_BRIDGE = "bridge"
	LINK_TYPE_VETH   = "veth"
	LINK_TYPE_IPVLAN = "ipvlan"
	LINK_TYPE_DUMMY  = "dummy"
)

// IPVLAN link attributes.
type IPVlanMode uint16

const (
	IPVLAN_MODE_L2 IPVlanMode = iota
	IPVLAN_MODE_L3
	IPVLAN_MODE_L3S
	IPVLAN_MODE_MAX
)

const (
	ADD = iota
	REMOVE
)

// Link represents a network interface.
type Link interface {
	Info() *LinkInfo
}

// LinkInfo respresents the common properties of all network interfaces.
type LinkInfo struct {
	Type        string
	Name        string
	Flags       net.Flags
	MTU         uint
	TxQLen      uint
	ParentIndex int
}

func (linkInfo *LinkInfo) Info() *LinkInfo {
	return linkInfo
}

// BridgeLink represents an ethernet bridge.
type BridgeLink struct {
	LinkInfo
}

// VEthLink represents a virtual ethernet network interface.
type VEthLink struct {
	LinkInfo
	PeerName string
}

// IPVlanLink represents an IPVlan network interface.
type IPVlanLink struct {
	LinkInfo
	Mode IPVlanMode
}

// DummyLink represents a dummy network interface.
type DummyLink struct {
	LinkInfo
}

// AddLink adds a new network interface of a specified type.
func (Netlink) AddLink(link Link) error {
	info := link.Info()

	if info.Name == "" || info.Type == "" {
		return fmt.Errorf("Invalid link name or type")
	}

	s, err := getSocket()
	if err != nil {
		return err
	}

	req := newRequest(unix.RTM_NEWLINK, unix.NLM_F_CREATE|unix.NLM_F_EXCL|unix.NLM_F_ACK)

	// Set interface information.
	ifInfo := newIfInfoMsg()

	// Set interface flags.
	if info.Flags&net.FlagUp != 0 {
		ifInfo.Change = unix.IFF_UP
		ifInfo.Flags = unix.IFF_UP
	}
	req.addPayload(ifInfo)

	// Set interface name.
	attrIfName := newAttributeStringZ(unix.IFLA_IFNAME, info.Name)
	req.addPayload(attrIfName)

	// Set MTU.
	if info.MTU > 0 {
		req.addPayload(newAttributeUint32(unix.IFLA_MTU, uint32(info.MTU)))
	}

	// Set transmission queue length.
	if info.TxQLen > 0 {
		req.addPayload(newAttributeUint32(unix.IFLA_TXQLEN, uint32(info.TxQLen)))
	}

	// Set parent interface index.
	if info.ParentIndex != 0 {
		req.addPayload(newAttributeUint32(unix.IFLA_LINK, uint32(info.ParentIndex)))
	}

	// Set link info.
	attrLinkInfo := newAttribute(unix.IFLA_LINKINFO, nil)
	attrLinkInfo.addNested(newAttributeString(IFLA_INFO_KIND, info.Type))

	// Set link type-specific attributes.
	if veth, ok := link.(*VEthLink); ok {
		// Set VEth attributes.
		attrData := newAttribute(IFLA_INFO_DATA, nil)

		attrPeer := newAttribute(VETH_INFO_PEER, nil)
		attrPeer.addNested(newIfInfoMsg())
		attrPeer.addNested(newAttributeStringZ(unix.IFLA_IFNAME, veth.PeerName))
		attrData.addNested(attrPeer)

		attrLinkInfo.addNested(attrData)

	} else if ipvlan, ok := link.(*IPVlanLink); ok {
		// Set IPVlan attributes.
		attrData := newAttribute(IFLA_INFO_DATA, nil)
		attrData.addNested(newAttributeUint16(IFLA_IPVLAN_MODE, uint16(ipvlan.Mode)))

		attrLinkInfo.addNested(attrData)
	}

	req.addPayload(attrLinkInfo)

	return s.sendAndWaitForAck(req)
}

// DeleteLink deletes a network interface.
func (Netlink) DeleteLink(name string) error {
	if name == "" {
		log.Printf("[net] Invalid link name. Not returning error")
		return nil
	}

	iface, err := net.InterfaceByName(name)
	if err != nil {
		log.Printf("[net] Interface not found. Not returning error")
		return nil
	}

	s, err := getSocket()
	if err != nil {
		return err
	}

	req := newRequest(unix.RTM_DELLINK, unix.NLM_F_ACK)

	ifInfo := newIfInfoMsg()
	ifInfo.Index = int32(iface.Index)
	req.addPayload(ifInfo)

	return s.sendAndWaitForAck(req)
}

// SetLinkName sets the name of a network interface.
func (Netlink) SetLinkName(name string, newName string) error {
	s, err := getSocket()
	if err != nil {
		return err
	}

	iface, err := net.InterfaceByName(name)
	if err != nil {
		return err
	}

	req := newRequest(unix.RTM_SETLINK, unix.NLM_F_ACK)

	ifInfo := newIfInfoMsg()
	ifInfo.Type = unix.RTM_SETLINK
	ifInfo.Index = int32(iface.Index)
	ifInfo.Flags = unix.NLM_F_REQUEST
	ifInfo.Change = DEFAULT_CHANGE
	req.addPayload(ifInfo)

	attrName := newAttributeString(unix.IFLA_IFNAME, newName)
	req.addPayload(attrName)

	return s.sendAndWaitForAck(req)
}

// SetLinkState sets the operational state of a network interface.
func (Netlink) SetLinkState(name string, up bool) error {
	s, err := getSocket()
	if err != nil {
		return err
	}

	iface, err := net.InterfaceByName(name)
	if err != nil {
		return err
	}

	req := newRequest(unix.RTM_NEWLINK, unix.NLM_F_ACK)

	ifInfo := newIfInfoMsg()
	ifInfo.Type = unix.RTM_SETLINK
	ifInfo.Index = int32(iface.Index)

	if up {
		ifInfo.Flags = unix.IFF_UP
		ifInfo.Change = unix.IFF_UP
	} else {
		ifInfo.Flags = 0 & ^unix.IFF_UP
		ifInfo.Change = DEFAULT_CHANGE
	}

	req.addPayload(ifInfo)

	return s.sendAndWaitForAck(req)
}

// SetLinkMaster sets the master (upper) device of a network interface.
func (Netlink) SetLinkMaster(name string, master string) error {
	s, err := getSocket()
	if err != nil {
		return err
	}

	iface, err := net.InterfaceByName(name)
	if err != nil {
		return err
	}

	var masterIndex uint32
	if master != "" {
		masterIface, err := net.InterfaceByName(master)
		if err != nil {
			return err
		}
		masterIndex = uint32(masterIface.Index)
	}

	req := newRequest(unix.RTM_SETLINK, unix.NLM_F_ACK)

	ifInfo := newIfInfoMsg()
	ifInfo.Type = unix.RTM_SETLINK
	ifInfo.Index = int32(iface.Index)
	ifInfo.Flags = unix.NLM_F_REQUEST
	ifInfo.Change = DEFAULT_CHANGE
	req.addPayload(ifInfo)

	attrMaster := newAttributeUint32(unix.IFLA_MASTER, masterIndex)
	req.addPayload(attrMaster)

	return s.sendAndWaitForAck(req)
}

// SetLinkNetNs sets the network namespace of a network interface.
func (Netlink) SetLinkNetNs(name string, fd uintptr) error {
	s, err := getSocket()
	if err != nil {
		return err
	}

	iface, err := net.InterfaceByName(name)
	if err != nil {
		return err
	}

	req := newRequest(unix.RTM_SETLINK, unix.NLM_F_ACK)

	ifInfo := newIfInfoMsg()
	ifInfo.Type = unix.RTM_SETLINK
	ifInfo.Index = int32(iface.Index)
	ifInfo.Flags = unix.NLM_F_REQUEST
	ifInfo.Change = DEFAULT_CHANGE
	req.addPayload(ifInfo)

	attrNetNs := newAttributeUint32(IFLA_NET_NS_FD, uint32(fd))
	req.addPayload(attrNetNs)

	return s.sendAndWaitForAck(req)
}

// SetLinkAddress sets the link layer hardware address of a network interface.
func (Netlink) SetLinkAddress(ifName string, hwAddress net.HardwareAddr) error {
	s, err := getSocket()
	if err != nil {
		return err
	}

	iface, err := net.InterfaceByName(ifName)
	if err != nil {
		return err
	}

	req := newRequest(unix.RTM_SETLINK, unix.NLM_F_ACK)

	ifInfo := newIfInfoMsg()
	ifInfo.Type = unix.RTM_SETLINK
	ifInfo.Index = int32(iface.Index)
	ifInfo.Flags = unix.NLM_F_REQUEST
	ifInfo.Change = DEFAULT_CHANGE
	req.addPayload(ifInfo)

	req.addPayload(newAttribute(unix.IFLA_ADDRESS, hwAddress))

	return s.sendAndWaitForAck(req)
}

// SetLinkPromisc sets the promiscuous mode of a network interface.
// TODO do we need this function, not used anywhere currently
func (Netlink) SetLinkPromisc(ifName string, on bool) error {
	s, err := getSocket()
	if err != nil {
		return err
	}

	iface, err := net.InterfaceByName(ifName)
	if err != nil {
		return err
	}

	req := newRequest(unix.RTM_SETLINK, unix.NLM_F_ACK)

	ifInfo := newIfInfoMsg()
	ifInfo.Type = unix.RTM_SETLINK
	ifInfo.Index = int32(iface.Index)

	if on {
		ifInfo.Flags = unix.IFF_PROMISC
		ifInfo.Change = unix.IFF_PROMISC
	} else {
		ifInfo.Flags = 0 & ^unix.IFF_PROMISC
		ifInfo.Change = unix.IFF_PROMISC
	}

	req.addPayload(ifInfo)

	return s.sendAndWaitForAck(req)
}

// SetLinkHairpin sets the hairpin (reflective relay) mode of a bridged interface.
func (Netlink) SetLinkHairpin(bridgeName string, on bool) error {
	s, err := getSocket()
	if err != nil {
		return err
	}

	iface, err := net.InterfaceByName(bridgeName)
	if err != nil {
		return err
	}

	req := newRequest(unix.RTM_SETLINK, unix.NLM_F_ACK)

	ifInfo := newIfInfoMsg()
	ifInfo.Family = unix.AF_BRIDGE
	ifInfo.Type = unix.RTM_SETLINK
	ifInfo.Index = int32(iface.Index)
	ifInfo.Flags = unix.NLM_F_REQUEST
	ifInfo.Change = DEFAULT_CHANGE
	req.addPayload(ifInfo)

	hairpin := []byte{0}
	if on {
		hairpin[0] = byte(1)
	}

	attrProtInfo := newAttribute(unix.IFLA_PROTINFO|unix.NLA_F_NESTED, nil)
	attrProtInfo.addNested(newAttribute(IFLA_BRPORT_MODE, hairpin))
	req.addPayload(attrProtInfo)

	return s.sendAndWaitForAck(req)
}

// AddOrRemoveStaticArp sets/removes static arp entry based on mode
func (Netlink) AddOrRemoveStaticArp(mode int, name string, ipaddr net.IP, mac net.HardwareAddr, isProxy bool) error {
	s, err := getSocket()
	if err != nil {
		return err
	}

	var req *message
	state := 0
	if mode == ADD {
		req = newRequest(unix.RTM_NEWNEIGH, unix.NLM_F_CREATE|unix.NLM_F_REPLACE|unix.NLM_F_ACK)
		state = NUD_PERMANENT
	} else {
		req = newRequest(unix.RTM_DELNEIGH, unix.NLM_F_ACK)
		state = NUD_INCOMPLETE
	}

	iface, err := net.InterfaceByName(name)
	if err != nil {
		return err
	}

	msg := neighMsg{
		Family: uint8(GetIPAddressFamily(ipaddr)),
		Index:  uint32(iface.Index),
		State:  uint16(state),
	}

	// NTF_PROXY is for setting neighbor proxy
	if isProxy {
		msg.Flags = msg.Flags | NTF_PROXY
	}

	req.addPayload(&msg)

	ipData := ipaddr.To4()
	if ipData == nil {
		ipData = ipaddr.To16()
	}

	dstData := newRtAttr(NDA_DST, ipData)
	req.addPayload(dstData)

	hwData := newRtAttr(NDA_LLADDR, []byte(mac))
	req.addPayload(hwData)

	return s.sendAndWaitForAck(req)
}
