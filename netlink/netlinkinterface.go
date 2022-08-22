// Copyright 2021 Microsoft. All rights reserved.
// MIT License

package netlink

import (
	"net"
)

type NetlinkInterface interface {
	AddLink(link Link) error
	DeleteLink(name string) error
	SetLinkName(name string, newName string) error
	SetLinkState(name string, up bool) error
	SetLinkMTU(name string, mtu int) error
	SetLinkMaster(name string, master string) error
	SetLinkNetNs(name string, fd uintptr) error
	SetLinkAddress(ifName string, hwAddress net.HardwareAddr) error
	SetLinkPromisc(ifName string, on bool) error
	SetLinkHairpin(bridgeName string, on bool) error
	SetOrRemoveLinkAddress(linkInfo LinkInfo, mode, linkState int) error
	AddIPAddress(ifName string, ipAddress net.IP, ipNet *net.IPNet) error
	DeleteIPAddress(ifName string, ipAddress net.IP, ipNet *net.IPNet) error
	GetIPRoute(filter *Route) ([]*Route, error)
	AddIPRoute(route *Route) error
	DeleteIPRoute(route *Route) error
}
