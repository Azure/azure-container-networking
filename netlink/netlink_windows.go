// Copyright 2017 Microsoft. All rights reserved.
// MIT License

package netlink

import "net"

// Link represents a network interface.
type Link interface {
	Info() *LinkInfo
}

type Route struct{}

// LinkInfo respresents the common properties of all network interfaces.
type LinkInfo struct {
	Type string
	Name string
}

func (linkInfo *LinkInfo) Info() *LinkInfo {
	return linkInfo
}

func (Netlink) AddLink(link Link) error {
	return nil
}

func (Netlink) DeleteLink(name string) error {
	return nil
}

func (Netlink) SetLinkName(name string, newName string) error {
	return nil
}

func (Netlink) SetLinkState(name string, up bool) error {
	return nil
}

func (Netlink) SetLinkMaster(name string, master string) error {
	return nil
}

func (Netlink) SetLinkNetNs(name string, fd uintptr) error {
	return nil
}

func (Netlink) SetLinkAddress(ifName string, hwAddress net.HardwareAddr) error {
	return nil
}

func (Netlink) SetLinkPromisc(ifName string, on bool) error {
	return nil
}

func (Netlink) SetLinkHairpin(bridgeName string, on bool) error {
	return nil
}

func (Netlink) AddOrRemoveStaticArp(mode int, name string, ipaddr net.IP, mac net.HardwareAddr, isProxy bool) error {
	return nil
}

func (Netlink) AddIPAddress(ifName string, ipAddress net.IP, ipNet *net.IPNet) error {
	return nil
}

func (Netlink) DeleteIPAddress(ifName string, ipAddress net.IP, ipNet *net.IPNet) error {
	return nil
}

func (Netlink) GetIPRoute(filter *Route) ([]*Route, error) {
	return nil, nil
}

func (Netlink) AddIPRoute(route *Route) error {
	return nil
}

func (Netlink) DeleteIPRoute(route *Route) error {
	return nil
}
