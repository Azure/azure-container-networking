// Copyright 2017 Microsoft. All rights reserved.
// MIT License

//go:build linux
// +build linux

package netlink

import (
	"net"

	"golang.org/x/sys/unix"
)

const (
	RT_SCOPE_UNIVERSE = 0
	RT_SCOPE_SITE     = 200
	RT_SCOPE_LINK     = 253
	RT_SCOPE_HOST     = 254
	RT_SCOPE_NOWHERE  = 255
)

const (
	RTPROT_KERNEL = 2
)

// setIPAddress sends an IP address set request.
func (Netlink) setIPAddress(ifName string, ipAddress net.IP, ipNet *net.IPNet, add bool) error {
	var msgType, flags int

	s, err := getSocket()
	if err != nil {
		return err
	}

	iface, err := net.InterfaceByName(ifName)
	if err != nil {
		return err
	}

	if add {
		msgType = unix.RTM_NEWADDR
		flags = unix.NLM_F_CREATE | unix.NLM_F_EXCL | unix.NLM_F_ACK
	} else {
		msgType = unix.RTM_DELADDR
		flags = unix.NLM_F_ACK
	}

	req := newRequest(msgType, flags)

	family := GetIPAddressFamily(ipAddress)

	ifAddr := newIfAddrMsg(family)
	ifAddr.Index = uint32(iface.Index)
	prefixLen, _ := ipNet.Mask.Size()
	ifAddr.Prefixlen = uint8(prefixLen)
	req.addPayload(ifAddr)

	var ipAddrValue []byte
	if family == unix.AF_INET {
		ipAddrValue = ipAddress.To4()
	} else {
		ipAddrValue = ipAddress.To16()
	}

	req.addPayload(newAttribute(unix.IFA_LOCAL, ipAddrValue))
	req.addPayload(newAttribute(unix.IFA_ADDRESS, ipAddrValue))

	return s.sendAndWaitForAck(req)
}

// AddIPAddress adds an IP address to a network interface.
func (n Netlink) AddIPAddress(ifName string, ipAddress net.IP, ipNet *net.IPNet) error {
	return n.setIPAddress(ifName, ipAddress, ipNet, true)
}

// DeleteIPAddress deletes an IP address from a network interface.
func (n Netlink) DeleteIPAddress(ifName string, ipAddress net.IP, ipNet *net.IPNet) error {
	return n.setIPAddress(ifName, ipAddress, ipNet, false)
}

// Route represents a netlink route.
type Route struct {
	Family     int
	Dst        *net.IPNet
	Src        net.IP
	Gw         net.IP
	Tos        int
	Table      int
	Protocol   int
	Scope      int
	Type       int
	Flags      int
	Priority   int
	LinkIndex  int
	ILinkIndex int
}

// deserializeRoute decodes a netlink message into a Route struct.
func deserializeRoute(msg *message) (*Route, error) {
	// Parse route message.
	rtmsg := deserializeRtMsg(msg.data)
	attrs := msg.getAttributes(rtmsg)

	// Initialize a new route object.
	route := Route{
		Family:   int(rtmsg.Family),
		Tos:      int(rtmsg.Tos),
		Table:    int(rtmsg.Table),
		Protocol: int(rtmsg.Protocol),
		Scope:    int(rtmsg.Scope),
		Type:     int(rtmsg.Type),
		Flags:    int(rtmsg.Flags),
	}

	// Populate route attributes.
	for _, attr := range attrs {
		switch attr.Type {
		case unix.RTA_DST:
			route.Dst = &net.IPNet{
				IP:   attr.value,
				Mask: net.CIDRMask(int(rtmsg.Dst_len), 8*len(attr.value)),
			}
		case unix.RTA_PREFSRC:
			route.Src = net.IP(attr.value)
		case unix.RTA_GATEWAY:
			route.Gw = net.IP(attr.value)
		case unix.RTA_TABLE:
			route.Table = int(encoder.Uint32(attr.value[0:4]))
		case unix.RTA_PRIORITY:
			route.Priority = int(encoder.Uint32(attr.value[0:4]))
		case unix.RTA_OIF:
			route.LinkIndex = int(encoder.Uint32(attr.value[0:4]))
		case unix.RTA_IIF:
			route.ILinkIndex = int(encoder.Uint32(attr.value[0:4]))
		}
	}

	return &route, nil
}

// GetIPRoute returns a list of IP routes matching the given filter.
func (Netlink) GetIPRoute(filter *Route) ([]*Route, error) {
	s, err := getSocket()
	if err != nil {
		return nil, err
	}

	req := newRequest(unix.RTM_GETROUTE, unix.NLM_F_DUMP)

	ifInfo := newIfInfoMsg()
	ifInfo.Family = uint8(filter.Family)
	req.addPayload(ifInfo)

	msgs, err := s.sendAndWaitForResponse(req)
	if err != nil {
		return nil, err
	}

	var routes []*Route

	// For each route in the list...
	for _, msg := range msgs {
		route, err := deserializeRoute(msg)
		if err != nil {
			return nil, err
		}

		// Ignore cloned routes.
		if route.Flags&unix.RTM_F_CLONED != 0 {
			continue
		}

		// Filter by table.
		if (filter.Table == 0 && route.Table != unix.RT_TABLE_MAIN) ||
			(filter.Table != 0 && filter.Table != route.Table) {
			continue
		}

		// Filter by protocol.
		if filter.Protocol != 0 && filter.Protocol != route.Protocol {
			continue
		}

		// Filter by destination prefix.
		if filter.Dst != nil {
			fMaskOnes, fMaskBits := filter.Dst.Mask.Size()

			if route.Dst == nil {
				if fMaskOnes != 0 {
					continue
				}
			} else {
				rMaskOnes, rMaskBits := route.Dst.Mask.Size()

				if !filter.Dst.IP.Equal(route.Dst.IP) ||
					fMaskOnes != rMaskOnes || fMaskBits != rMaskBits {
					continue
				}
			}
		}

		// Filter by link index.
		if filter.LinkIndex != 0 && filter.LinkIndex != route.LinkIndex {
			continue
		}

		routes = append(routes, route)
	}

	return routes, nil
}

// setIpRoute sends an IP route set request.
func setIpRoute(route *Route, add bool) error {
	var msgType, flags int

	s, err := getSocket()
	if err != nil {
		return err
	}

	if add {
		msgType = unix.RTM_NEWROUTE
		flags = unix.NLM_F_CREATE | unix.NLM_F_EXCL | unix.NLM_F_ACK
	} else {
		msgType = unix.RTM_DELROUTE
		flags = unix.NLM_F_ACK
	}

	req := newRequest(msgType, flags)

	msg := newRtMsg(route.Family)
	msg.Tos = uint8(route.Tos)
	msg.Table = uint8(route.Table)

	if route.Protocol != 0 {
		msg.Protocol = uint8(route.Protocol)
	}

	if route.Scope != 0 {
		msg.Scope = uint8(route.Scope)
	}

	if route.Type != 0 {
		msg.Type = uint8(route.Type)
	}

	msg.Flags = uint32(route.Flags)

	req.addPayload(msg)

	if route.Dst != nil {
		prefixLength, _ := route.Dst.Mask.Size()
		msg.Dst_len = uint8(prefixLength)
		req.addPayload(newAttributeIpAddress(unix.RTA_DST, route.Dst.IP))
	}

	if route.Src != nil {
		req.addPayload(newAttributeIpAddress(unix.RTA_PREFSRC, route.Src))
	}

	if route.Gw != nil {
		req.addPayload(newAttributeIpAddress(unix.RTA_GATEWAY, route.Gw))
	}

	if route.Priority != 0 {
		req.addPayload(newAttributeUint32(unix.RTA_PRIORITY, uint32(route.Priority)))
	}

	if route.LinkIndex != 0 {
		req.addPayload(newAttributeUint32(unix.RTA_OIF, uint32(route.LinkIndex)))
	}

	if route.ILinkIndex != 0 {
		req.addPayload(newAttributeUint32(unix.RTA_IIF, uint32(route.ILinkIndex)))
	}

	return s.sendAndWaitForAck(req)
}

// AddIPRoute adds an IP route to the route table.
func (Netlink) AddIPRoute(route *Route) error {
	return setIpRoute(route, true)
}

// DeleteIPRoute deletes an IP route from the route table.
func (Netlink) DeleteIPRoute(route *Route) error {
	return setIpRoute(route, false)
}

// GetIPAddressFamily returns the address family of an IP address.
func GetIPAddressFamily(ip net.IP) int {
	if len(ip) <= net.IPv4len {
		return unix.AF_INET
	}
	if ip.To4() != nil {
		return unix.AF_INET
	}
	return unix.AF_INET6
}
