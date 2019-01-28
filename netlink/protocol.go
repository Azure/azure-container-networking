// Copyright 2017 Microsoft. All rights reserved.
// MIT License

// +build linux

package netlink

import (
	"encoding/binary"
	"net"
	"unsafe"

	"golang.org/x/sys/unix"
)

const (
	NDA_UNSPEC = iota
	NDA_DST
	NDA_LLADDR
	NDA_CACHEINFO
	NDA_PROBES
	NDA_VLAN
	NDA_PORT
	NDA_VNI
	NDA_IFINDEX
	NDA_MAX = NDA_IFINDEX
)

// Neighbor Cache Entry States.
const (
	NUD_NONE       = 0x00
	NUD_INCOMPLETE = 0x01
	NUD_REACHABLE  = 0x02
	NUD_STALE      = 0x04
	NUD_DELAY      = 0x08
	NUD_PROBE      = 0x10
	NUD_FAILED     = 0x20
	NUD_NOARP      = 0x40
	NUD_PERMANENT  = 0x80
)

// Neighbor Flags
const (
	NTF_USE    = 0x01
	NTF_SELF   = 0x02
	NTF_MASTER = 0x04
	NTF_PROXY  = 0x08
	NTF_ROUTER = 0x80
)

// Netlink protocol constants that are not already defined in unix package.
const (
	IFLA_INFO_KIND   = 1
	IFLA_INFO_DATA   = 2
	IFLA_NET_NS_FD   = 28
	IFLA_IPVLAN_MODE = 1
	IFLA_BRPORT_MODE = 4
	VETH_INFO_PEER   = 1
	DEFAULT_CHANGE   = 0xFFFFFFFF
)

// Serializable types are used to construct netlink messages.
type serializable interface {
	serialize() []byte
	length() int
}

//
// Netlink message
//

// Generic netlink message
type message struct {
	unix.NlMsghdr
	data    []byte
	payload []serializable
}

// Generic netlink message attribute
type attribute struct {
	unix.NlAttr
	value    []byte
	children []serializable
}

// Neighbor entry message strutcure
type neighMsg struct {
	Family uint8
	Index  uint32
	State  uint16
	Flags  uint8
	Type   uint8
}

// rta attribute structure
type rtAttr struct {
	unix.RtAttr
	Data     []byte
	children []serializable
}

// Byte encoder
var encoder binary.ByteOrder

// Initializes the byte encoder.
func initEncoder() {
	var x uint32 = 0x01020304
	if *(*byte)(unsafe.Pointer(&x)) == 0x01 {
		encoder = binary.BigEndian
	} else {
		encoder = binary.LittleEndian
	}
}

// Creates a new netlink message.
func newMessage(msgType int, flags int) *message {
	return &message{
		NlMsghdr: unix.NlMsghdr{
			Len:   uint32(unix.NLMSG_HDRLEN),
			Type:  uint16(msgType),
			Flags: uint16(flags),
			Seq:   0,
			Pid:   uint32(unix.Getpid()),
		},
	}
}

// Creates a new netlink request message.
func newRequest(msgType int, flags int) *message {
	return newMessage(msgType, flags|unix.NLM_F_REQUEST)
}

// Appends protocol specific payload to a netlink message.
func (msg *message) addPayload(payload serializable) {
	if payload != nil {
		msg.payload = append(msg.payload, payload)
	}
}

// Serializes a netlink message.
func (msg *message) serialize() []byte {
	// Serialize the protocol specific payload.
	msg.Len = uint32(unix.NLMSG_HDRLEN)
	payload := make([][]byte, len(msg.payload))
	for i, p := range msg.payload {
		payload[i] = p.serialize()
		msg.Len += uint32(len(payload[i]))
	}

	// Serialize the message header.
	b := make([]byte, msg.Len)
	encoder.PutUint32(b[0:4], msg.Len)
	encoder.PutUint16(b[4:6], msg.Type)
	encoder.PutUint16(b[6:8], msg.Flags)
	encoder.PutUint32(b[8:12], msg.Seq)
	encoder.PutUint32(b[12:16], msg.Pid)

	// Append the payload.
	next := 16
	for _, p := range payload {
		copy(b[next:], p)
		next += len(p)
	}

	return b
}

// Get attributes.
func (msg *message) getAttributes(body serializable) []*attribute {
	var attrs []*attribute

	s := msg.payload[1:]
	for _, v := range s {
		attr, ok := v.(*attribute)
		if !ok {
			continue
		}

		attrs = append(attrs, attr)
	}

	return attrs
}

//
// Netlink message attribute
//
// Creates a new attribute.
func newAttribute(attrType int, value []byte) *attribute {
	return &attribute{
		NlAttr: unix.NlAttr{
			Type: uint16(attrType),
		},
		value:    value,
		children: []serializable{},
	}
}

// Creates a new attribute with a string value.
func newAttributeString(attrType int, value string) *attribute {
	return newAttribute(attrType, []byte(value))
}

// Creates a new attribute with a null-terminated string value.
func newAttributeStringZ(attrType int, value string) *attribute {
	return newAttribute(attrType, []byte(value+"\000"))
}

// Creates a new attribute with a uint32 value.
func newAttributeUint32(attrType int, value uint32) *attribute {
	buf := make([]byte, 4)
	encoder.PutUint32(buf, value)
	return newAttribute(attrType, buf)
}

// Creates a new attribute with a uint16 value.
func newAttributeUint16(attrType int, value uint16) *attribute {
	buf := make([]byte, 2)
	encoder.PutUint16(buf, value)
	return newAttribute(attrType, buf)
}

// Creates a new attribute with a net.IP value.
func newAttributeIpAddress(attrType int, value net.IP) *attribute {
	addr := value.To4()
	if addr != nil {
		return newAttribute(attrType, addr)
	} else {
		return newAttribute(attrType, value.To16())
	}
}

// Adds a nested attribute to an attribute.
func (attr *attribute) addNested(nested serializable) {
	attr.children = append(attr.children, nested)
}

// Serializes an attribute.
func (attr *attribute) serialize() []byte {
	length := attr.length()
	buf := make([]byte, length)

	// Encode length.
	if l := uint16(length); l != 0 {
		encoder.PutUint16(buf[0:2], l)
	}

	// Encode type.
	encoder.PutUint16(buf[2:4], attr.Type)

	if attr.value != nil {
		// Encode value.
		copy(buf[4:], attr.value)
	} else {
		// Serialize any nested attributes.
		offset := 4
		for _, child := range attr.children {
			childBuf := child.serialize()
			copy(buf[offset:], childBuf)
			offset += len(childBuf)
		}
	}

	return buf
}

// Returns the aligned length of an attribute.
func (attr *attribute) length() int {
	len := unix.SizeofNlAttr + len(attr.value)

	for _, child := range attr.children {
		len += child.length()
	}

	return (len + unix.NLA_ALIGNTO - 1) & ^(unix.NLA_ALIGNTO - 1)
}

//
// Network interface service module
//

// Interface info message
type ifInfoMsg struct {
	unix.IfInfomsg
}

// Creates a new interface info message.
func newIfInfoMsg() *ifInfoMsg {
	return &ifInfoMsg{
		IfInfomsg: unix.IfInfomsg{
			Family: uint8(unix.AF_UNSPEC),
		},
	}
}

// Serializes an interface info message.
func (ifInfo *ifInfoMsg) serialize() []byte {
	b := make([]byte, ifInfo.length())
	b[0] = ifInfo.Family
	b[1] = 0 // Padding.
	encoder.PutUint16(b[2:4], ifInfo.Type)
	encoder.PutUint32(b[4:8], uint32(ifInfo.Index))
	encoder.PutUint32(b[8:12], ifInfo.Flags)
	encoder.PutUint32(b[12:16], ifInfo.Change)
	return b
}

// Returns the length of an interface info message.
func (ifInfo *ifInfoMsg) length() int {
	return unix.SizeofIfInfomsg
}

//
// IP address service module
//

// Interface address message
type ifAddrMsg struct {
	unix.IfAddrmsg
}

// Creates a new interface address message.
func newIfAddrMsg(family int) *ifAddrMsg {
	return &ifAddrMsg{
		IfAddrmsg: unix.IfAddrmsg{
			Family: uint8(family),
		},
	}
}

// Serializes an interface address message.
func (ifAddr *ifAddrMsg) serialize() []byte {
	b := make([]byte, ifAddr.length())
	b[0] = ifAddr.Family
	b[1] = ifAddr.Prefixlen
	b[2] = ifAddr.Flags
	b[3] = ifAddr.Scope
	encoder.PutUint32(b[4:8], ifAddr.Index)
	return b
}

// Returns the length of an interface address message.
func (ifAddr *ifAddrMsg) length() int {
	return unix.SizeofIfAddrmsg
}

//
// Network route service module
//

// Route message
type rtMsg struct {
	unix.RtMsg
}

// Creates a new route message.
func newRtMsg(family int) *rtMsg {
	return &rtMsg{
		RtMsg: unix.RtMsg{
			Family:   uint8(family),
			Protocol: unix.RTPROT_STATIC,
			Scope:    unix.RT_SCOPE_UNIVERSE,
			Type:     unix.RTN_UNICAST,
		},
	}
}

// Deserializes a route message.
func deserializeRtMsg(b []byte) *rtMsg {
	return (*rtMsg)(unsafe.Pointer(&b[0:unix.SizeofRtMsg][0]))
}

// Serializes a route message.
func (rt *rtMsg) serialize() []byte {
	b := make([]byte, rt.length())
	b[0] = rt.Family
	b[1] = rt.Dst_len
	b[2] = rt.Src_len
	b[3] = rt.Tos
	b[4] = rt.Table
	b[5] = rt.Protocol
	b[6] = rt.Scope
	b[7] = rt.Type
	encoder.PutUint32(b[8:12], rt.Flags)
	return b
}

// Returns the length of a route message.
func (rt *rtMsg) length() int {
	return unix.SizeofRtMsg
}

// serialize neighbor message
func (msg *neighMsg) serialize() []byte {
	return (*(*[unsafe.Sizeof(*msg)]byte)(unsafe.Pointer(msg)))[:]
}

func (msg *neighMsg) length() int {
	return int(unsafe.Sizeof(*msg))
}

// creates new rta attr message
func newRtAttr(attrType int, data []byte) *rtAttr {
	return &rtAttr{
		RtAttr: unix.RtAttr{
			Type: uint16(attrType),
		},
		children: []serializable{},
		Data:     data,
	}
}

// align rta attributes
func rtaAlignOf(attrlen int) int {
	return (attrlen + unix.RTA_ALIGNTO - 1) & ^(unix.RTA_ALIGNTO - 1)
}

// serialize rta message
func (rta *rtAttr) serialize() []byte {
	length := rta.length()
	buf := make([]byte, rtaAlignOf(length))

	next := 4
	if rta.Data != nil {
		copy(buf[next:], rta.Data)
		next += rtaAlignOf(len(rta.Data))
	}
	if len(rta.children) > 0 {
		for _, child := range rta.children {
			childBuf := child.serialize()
			copy(buf[next:], childBuf)
			next += rtaAlignOf(len(childBuf))
		}
	}

	if l := uint16(length); l != 0 {
		encoder.PutUint16(buf[0:2], l)
	}
	encoder.PutUint16(buf[2:4], rta.Type)
	return buf
}

func (rta *rtAttr) length() int {
	if len(rta.children) == 0 {
		return (unix.SizeofRtAttr + len(rta.Data))
	}

	l := 0
	for _, child := range rta.children {
		l += rtaAlignOf(child.length())
	}
	l += unix.SizeofRtAttr
	return rtaAlignOf(l + len(rta.Data))
}

func (rta *rtAttr) addChild(attr serializable) {
	rta.children = append(rta.children, attr)
}
