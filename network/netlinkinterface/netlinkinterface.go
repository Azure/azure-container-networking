// Copyright 2021 Microsoft. All rights reserved.
// MIT License

package netlinkinterface

import (
	"net"

	"github.com/Azure/azure-container-networking/netlink"
)

type NetlinkInterface interface {
	AddLink(link netlink.Link) error
	DeleteLink(name string) error
	SetLinkName(name string, newName string) error
	SetLinkState(name string, up bool) error
	SetLinkMaster(name string, master string) error
	SetLinkNetNs(name string, fd uintptr) error
	SetLinkAddress(ifName string, hwAddress net.HardwareAddr) error
	SetLinkPromisc(ifName string, on bool) error
	SetLinkHairpin(bridgeName string, on bool) error
	AddOrRemoveStaticArp(mode int, name string, ipaddr net.IP, mac net.HardwareAddr, isProxy bool) error
	AddIPAddress(ifName string, ipAddress net.IP, ipNet *net.IPNet) error
	DeleteIPAddress(ifName string, ipAddress net.IP, ipNet *net.IPNet) error
	GetIPRoute(filter *netlink.Route) ([]*netlink.Route, error)
	AddIPRoute(route *netlink.Route) error
	DeleteIPRoute(route *netlink.Route) error
}
