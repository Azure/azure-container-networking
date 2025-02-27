//go:build linux
// +build linux

package dhcp

import (
	"context"
	"encoding/binary"
	"io"
	"net"
	"time"

	"github.com/pkg/errors"
	"go.uber.org/zap"
	"golang.org/x/sys/unix"
)

type Socket struct {
	fd         int
	remoteAddr unix.SockaddrInet4
}

// Linux specific
// returns a writer which should always be closed, even if we return an error
func NewWriteSocket(ifname string, remoteAddr unix.SockaddrInet4) (io.WriteCloser, error) {
	fd, err := MakeBroadcastSocket(ifname)
	ret := &Socket{
		fd:         fd,
		remoteAddr: remoteAddr,
	}
	if err != nil {
		return ret, errors.Wrap(err, "could not make dhcp write socket")
	}

	return ret, nil
}

func (s *Socket) Write(packetBytes []byte) (int, error) {
	err := unix.Sendto(s.fd, packetBytes, 0, &s.remoteAddr)
	if err != nil {
		return 0, errors.Wrap(err, "failed unix send to")
	}
	return len(packetBytes), nil
}

// returns a reader which should always be closed, even if we return an error
func NewReadSocket(ifname string, timeout time.Duration) (io.ReadCloser, error) {
	fd, err := makeListeningSocket(ifname, timeout)
	ret := &Socket{
		fd: fd,
	}
	if err != nil {
		return ret, errors.Wrap(err, "could not make dhcp read socket")
	}

	return ret, nil
}

func (s *Socket) Read(p []byte) (n int, err error) {
	n, _, innerErr := unix.Recvfrom(s.fd, p, 0)
	if innerErr != nil {
		return 0, errors.Wrap(err, "failed unix recv from")
	}
	return n, nil
}

func (s *Socket) Close() error {
	// do not attempt to close fd with -1 as they are not valid
	if s.fd == -1 {
		return nil
	}
	// Ensure the file descriptor is closed when done
	if err := unix.Close(s.fd); err != nil {
		return errors.Wrap(err, "error closing dhcp unix socket")
	}
	return nil
}

func makeListeningSocket(ifname string, timeout time.Duration) (int, error) {
	// reference: https://manned.org/packet.7
	// starts listening to the specified protocol, or none if zero
	// the SockaddrLinkLayer also ensures packets for the htons(unix.ETH_P_IP) prot are received
	fd, err := unix.Socket(unix.AF_PACKET, unix.SOCK_DGRAM, int(htons(unix.ETH_P_IP)))
	if err != nil {
		return fd, errors.Wrap(err, "dhcp socket creation failure")
	}
	iface, err := net.InterfaceByName(ifname)
	if err != nil {
		return fd, errors.Wrap(err, "dhcp failed to get interface")
	}
	llAddr := unix.SockaddrLinklayer{
		Ifindex:  iface.Index,
		Protocol: htons(unix.ETH_P_IP),
	}
	err = unix.Bind(fd, &llAddr)

	// set max time waiting without any data received
	timeval := unix.NsecToTimeval(timeout.Nanoseconds())
	if innerErr := unix.SetsockoptTimeval(fd, unix.SOL_SOCKET, unix.SO_RCVTIMEO, &timeval); innerErr != nil {
		return fd, errors.Wrap(innerErr, "could not set timeout on socket")
	}

	return fd, errors.Wrap(err, "dhcp failed to bind")
}

// MakeBroadcastSocket creates a socket that can be passed to unix.Sendto
// that will send packets out to the broadcast address.
func MakeBroadcastSocket(ifname string) (int, error) {
	fd, err := makeRawSocket(ifname)
	if err != nil {
		return fd, err
	}
	// enables broadcast (disabled by default)
	err = unix.SetsockoptInt(fd, unix.SOL_SOCKET, unix.SO_BROADCAST, 1)
	if err != nil {
		return fd, errors.Wrap(err, "dhcp failed to set sockopt")
	}
	return fd, nil
}

// conversion between host and network byte order
func htons(v uint16) uint16 {
	var tmp [2]byte
	binary.BigEndian.PutUint16(tmp[:], v)
	return binary.LittleEndian.Uint16(tmp[:])
}

func BindToInterface(fd int, ifname string) error {
	return errors.Wrap(unix.BindToDevice(fd, ifname), "failed to bind to device")
}

// makeRawSocket creates a socket that can be passed to unix.Sendto.
func makeRawSocket(ifname string) (int, error) {
	// AF_INET sends via IPv4, SOCK_RAW means create an ip datagram socket (skips udp transport layer, see below)
	fd, err := unix.Socket(unix.AF_INET, unix.SOCK_RAW, unix.IPPROTO_RAW)
	if err != nil {
		return fd, errors.Wrap(err, "dhcp raw socket creation failure")
	}
	// Later on when we write to this socket, our packet already contains the header (we create it with MakeRawUDPPacket).
	err = unix.SetsockoptInt(fd, unix.IPPROTO_IP, unix.IP_HDRINCL, 1)
	if err != nil {
		return fd, errors.Wrap(err, "dhcp failed to set hdrincl raw sockopt")
	}
	err = BindToInterface(fd, ifname)
	if err != nil {
		return fd, errors.Wrap(err, "dhcp failed to bind to interface")
	}
	return fd, nil
}

// Issues a DHCP Discover packet from the nic specified by mac and name ifname
// Returns nil if a reply to the transaction was received, or error if time out
// Does not return the DHCP Offer that was received from the DHCP server
func (c *DHCP) DiscoverRequest(ctx context.Context, mac net.HardwareAddr, ifname string) error {
	txid, err := GenerateTransactionID()
	if err != nil {
		return errors.Wrap(err, "failed to generate random transaction id")
	}

	// Used in later steps
	raddr := &net.UDPAddr{IP: net.IPv4bcast, Port: dhcpServerPort}
	laddr := &net.UDPAddr{IP: net.IPv4zero, Port: dhcpClientPort}
	var destination [net.IPv4len]byte
	copy(destination[:], raddr.IP.To4())

	// Build a DHCP discover packet
	dhcpPacket, err := buildDHCPDiscover(mac, txid)
	if err != nil {
		return errors.Wrap(err, "failed to build dhcp discover packet")
	}
	// Make UDP packet from dhcp packet in previous steps
	packetToSendBytes, err := MakeRawUDPPacket(dhcpPacket, *raddr, *laddr)
	if err != nil {
		return errors.Wrap(err, "error making raw udp packet")
	}

	// Make writer
	remoteAddr := unix.SockaddrInet4{Port: laddr.Port, Addr: destination}
	writer, err := NewWriteSocket(ifname, remoteAddr)
	defer func() {
		// Ensure the file descriptor is closed when done
		if err = writer.Close(); err != nil {
			c.logger.Error("Error closing dhcp writer socket:", zap.Error(err))
		}
	}()
	if err != nil {
		return errors.Wrap(err, "failed to make broadcast socket")
	}

	// Make reader
	deadline, ok := ctx.Deadline()
	if !ok {
		return errors.New("no deadline for passed in context")
	}
	timeout := time.Until(deadline)
	// note: if the write/send takes a long time DiscoverRequest might take a bit longer than the deadline
	reader, err := NewReadSocket(ifname, timeout)
	defer func() {
		// Ensure the file descriptor is closed when done
		if err = reader.Close(); err != nil {
			c.logger.Error("Error closing dhcp reader socket:", zap.Error(err))
		}
	}()
	if err != nil {
		return errors.Wrap(err, "failed to make listening socket")
	}

	// Once writer and reader created, start sending and receiving
	_, err = writer.Write(packetToSendBytes)
	if err != nil {
		return errors.Wrap(err, "failed to send dhcp discover packet")
	}

	c.logger.Info("DHCP Discover packet was sent successfully", zap.Any("transactionID", txid))

	// Wait for DHCP response (Offer)
	res := c.receiveDHCPResponse(ctx, reader, txid)
	return res
}
