//go:build linux
// +build linux

package dhcp

import (
	"encoding/binary"
	"log"
	"net"

	"github.com/insomniacslk/dhcp/dhcpv4"
	"github.com/insomniacslk/dhcp/dhcpv4/client4"
	"github.com/pkg/errors"
	"golang.org/x/sys/unix"
)

type DHCP struct{}

func New() *DHCP {
	return &DHCP{}
}

func makeListeningSocketWithCustomPort(ifname string, port int) (int, error) {
	fd, err := unix.Socket(unix.AF_PACKET, unix.SOCK_DGRAM, int(htons(unix.ETH_P_IP)))
	if err != nil {
		return fd, err
	}
	iface, err := net.InterfaceByName(ifname)
	if err != nil {
		return fd, err
	}
	llAddr := unix.SockaddrLinklayer{
		Ifindex:  iface.Index,
		Protocol: htons(unix.ETH_P_IP),
	}
	err = unix.Bind(fd, &llAddr)
	return fd, err
}

// MakeBroadcastSocket creates a socket that can be passed to unix.Sendto
// that will send packets out to the broadcast address.
func MakeBroadcastSocket(ifname string) (int, error) {
	fd, err := makeRawSocket(ifname)
	if err != nil {
		return fd, err
	}
	err = unix.SetsockoptInt(fd, unix.SOL_SOCKET, unix.SO_BROADCAST, 1)
	if err != nil {
		return fd, err
	}
	return fd, nil
}

func htons(v uint16) uint16 {
	var tmp [2]byte
	binary.BigEndian.PutUint16(tmp[:], v)
	return binary.LittleEndian.Uint16(tmp[:])
}

// makeRawSocket creates a socket that can be passed to unix.Sendto.
func makeRawSocket(ifname string) (int, error) {
	fd, err := unix.Socket(unix.AF_INET, unix.SOCK_RAW, unix.IPPROTO_RAW)
	if err != nil {
		return fd, err
	}
	err = unix.SetsockoptInt(fd, unix.SOL_SOCKET, unix.SO_REUSEADDR, 1)
	if err != nil {
		return fd, err
	}
	err = unix.SetsockoptInt(fd, unix.IPPROTO_IP, unix.IP_HDRINCL, 1)
	if err != nil {
		return fd, err
	}
	err = dhcpv4.BindToInterface(fd, ifname)
	if err != nil {
		return fd, err
	}
	return fd, nil
}

func (c *DHCP) DiscoverRequest(hwAddr net.HardwareAddr, ifName string) (*dhcpv4.DHCPv4, error) {
	discover, err := dhcpv4.NewDiscovery(hwAddr)

	if err != nil {
		return nil, errors.Wrap(err, "failed to create dhcp discover request")
	}

	// send the DHCP DISCOVER request
	client := client4.NewClient()

	laddrPort := dhcpv4.ClientPort

	// get send and receive file descriptors
	sfd, err := MakeBroadcastSocket(ifName)
	if err != nil {
		return nil, errors.Wrap(err, "failed to make broadcast socket")
	}

	rfd, err := makeListeningSocketWithCustomPort(ifName, laddrPort)
	if err != nil {
		return nil, errors.Wrap(err, "failed to make listening socket")
	}

	defer func() {
		if err := unix.Close(sfd); err != nil {
			log.Printf("unix.Close(sendFd) failed: %v", err)
		}
		if sfd != rfd {
			if err := unix.Close(rfd); err != nil {
				log.Printf("unix.Close(recvFd) failed: %v", err)
			}
		}
	}()

	// send discover request and expect an offer response
	response, err := client.SendReceive(sfd, rfd, discover, dhcpv4.MessageTypeOffer)
	if err != nil {
		return nil, errors.Wrap(err, "failed during send receive")
	}

	return response, nil
}
