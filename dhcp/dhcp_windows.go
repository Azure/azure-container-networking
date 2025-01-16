package dhcp

import (
	"context"
	"net"
	"time"

	"github.com/Azure/azure-container-networking/retry"
	"github.com/pkg/errors"
	"go.uber.org/zap"
	"golang.org/x/sys/windows"
)

const (
	retryCount          = 4
	retryDelay          = 500 * time.Millisecond
	ipAssignRetryDelay  = 2000 * time.Millisecond
	socketTimeoutMillis = 1000
)

var (
	errInvalidIPv4Address    = errors.New("invalid ipv4 address")
	errIncorrectAddressCount = errors.New("address count found not equal to expected")
)

type Socket struct {
	fd       windows.Handle
	destAddr windows.SockaddrInet4
}

func NewSocket(destAddr windows.SockaddrInet4) (*Socket, error) {
	// Create a raw socket using windows.WSASocket
	fd, err := windows.WSASocket(windows.AF_INET, windows.SOCK_RAW, windows.IPPROTO_UDP, nil, 0, windows.WSA_FLAG_OVERLAPPED)
	ret := &Socket{
		fd:       fd,
		destAddr: destAddr,
	}
	if err != nil {
		return ret, errors.Wrap(err, "error creating socket")
	}

	// Set IP_HDRINCL to indicate that we are including our own IP header
	err = windows.SetsockoptInt(fd, windows.IPPROTO_IP, windows.IP_HDRINCL, 1)
	if err != nil {
		return ret, errors.Wrap(err, "error setting IP_HDRINCL")
	}
	// Set the SO_BROADCAST option or else we get an error saying that we access a socket in a way forbidden by its access perms
	err = windows.SetsockoptInt(windows.Handle(fd), windows.SOL_SOCKET, windows.SO_BROADCAST, 1)
	if err != nil {
		return ret, errors.Wrap(err, "error setting SO_BROADCAST")
	}
	// Set timeout
	if err = windows.SetsockoptInt(windows.Handle(fd), windows.SOL_SOCKET, windows.SO_RCVTIMEO, socketTimeoutMillis); err != nil {
		return ret, errors.Wrap(err, "error setting receive timeout")
	}
	return ret, nil
}

func (s *Socket) Write(packetBytes []byte) (int, error) {
	err := windows.Sendto(s.fd, packetBytes, 0, &s.destAddr)
	if err != nil {
		return 0, errors.Wrap(err, "failed windows send to")
	}
	return len(packetBytes), nil
}

func (s *Socket) Read(p []byte) (n int, err error) {
	n, _, innerErr := windows.Recvfrom(s.fd, p, 0)
	if innerErr != nil {
		return 0, errors.Wrap(err, "failed windows recv from")
	}
	return n, nil
}

func (s *Socket) Close() error {
	// do not attempt to close invalid fd (happens on socket creation failure)
	if s.fd == windows.InvalidHandle {
		return nil
	}
	// Ensure the file descriptor is closed when done
	if err := windows.Closesocket(s.fd); err != nil {
		return errors.Wrap(err, "error closing dhcp windows socket")
	}
	return nil
}

func (c *DHCP) getIPv4InterfaceAddresses(ifName string) ([]net.IP, error) {
	nic, err := c.netioClient.GetNetworkInterfaceByName(ifName)
	if err != nil {
		return []net.IP{}, errors.Wrap(err, "failed to get interface by name to find ipv4 addresses")
	}
	addresses, err := c.netioClient.GetNetworkInterfaceAddrs(nic)
	if err != nil {
		return []net.IP{}, errors.Wrap(err, "failed to get interface addresses")
	}
	ret := []net.IP{}
	for _, address := range addresses {
		// check if the ip is ipv4 and parse it
		ip, _, cidrErr := net.ParseCIDR(address.String())
		if cidrErr != nil || ip.To4() == nil {
			continue
		}
		ret = append(ret, ip)
	}

	c.logger.Info("Interface addresses found", zap.Any("foundIPs", addresses), zap.Any("selectedIPs", ret))
	return ret, nil
}

func (c *DHCP) verifyIPv4InterfaceAddressCount(ctx context.Context, ifName string, count, numRetries int, sleep time.Duration) error {
	retrier := retry.Retrier{
		Cooldown: retry.Max(numRetries, retry.Fixed(sleep)),
	}
	addressCountErr := retrier.Do(ctx, func() error {
		addresses, err := c.getIPv4InterfaceAddresses(ifName)
		if err != nil || len(addresses) != count {
			return retry.WrapTemporaryError(errIncorrectAddressCount) // nolint
		}
		return nil
	})
	return errors.Wrap(addressCountErr, "failed to verify interface ipv4 address count")
}

// issues a dhcp discover request on an interface by finding the secondary's ip and sending on its ip
func (c *DHCP) DiscoverRequest(ctx context.Context, macAddress net.HardwareAddr, ifName string) error {
	// Find the ipv4 address of the secondary interface (we're betting that this gets autoconfigured)
	err := c.verifyIPv4InterfaceAddressCount(ctx, ifName, 1, retryCount, ipAssignRetryDelay)
	if err != nil {
		return errors.Wrap(err, "failed to get auto ip config assigned in apipa range in time")
	}
	ipv4Addresses, err := c.getIPv4InterfaceAddresses(ifName)
	if err != nil || len(ipv4Addresses) == 0 {
		return errors.Wrap(err, "failed to get ipv4 addresses on interface")
	}
	uniqueAddress := ipv4Addresses[0].To4()
	if uniqueAddress == nil {
		return errInvalidIPv4Address
	}
	uniqueAddressStr := uniqueAddress.String()
	c.logger.Info("Retrieved automatic ip configuration: ", zap.Any("ip", uniqueAddress), zap.String("ipStr", uniqueAddressStr))

	// now begin the dhcp request
	txid, err := GenerateTransactionID()
	if err != nil {
		return errors.Wrap(err, "failed to generate transaction id")
	}

	// Prepare an IP and UDP header
	raddr := &net.UDPAddr{IP: net.IPv4bcast, Port: dhcpServerPort}
	laddr := &net.UDPAddr{IP: uniqueAddress, Port: dhcpClientPort}

	dhcpDiscover, err := buildDHCPDiscover(macAddress, txid)
	if err != nil {
		return errors.Wrap(err, "failed to build dhcp discover")
	}

	// Fill out the headers, add payload, and construct the full packet
	bytesToSend, err := MakeRawUDPPacket(dhcpDiscover, *raddr, *laddr)
	if err != nil {
		return errors.Wrap(err, "failed to make raw udp packet")
	}

	destAddr := windows.SockaddrInet4{
		Addr: [4]byte{255, 255, 255, 255}, // Destination IP
		Port: dhcpServerPort,              // Destination Port
	}
	// create new socket for writing and reading
	sock, err := NewSocket(destAddr)
	defer func() {
		// always clean up the socket, even if we fail while setting options
		closeErr := sock.Close()
		if closeErr != nil {
			c.logger.Error("Error closing dhcp socket:", zap.Error(closeErr))
		}
	}()
	if err != nil {
		return errors.Wrap(err, "failed to create socket")
	}

	retrier := retry.Retrier{
		Cooldown: retry.Max(retryCount, retry.Fixed(retryDelay)),
	}
	// retry sending the packet until it succeeds
	err = retrier.Do(ctx, func() error {
		_, sockErr := sock.Write(bytesToSend)
		return retry.WrapTemporaryError(sockErr) // nolint
	})
	if err != nil {
		return errors.Wrap(err, "failed to write to dhcp socket")
	}

	c.logger.Info("DHCP Discover packet was sent successfully", zap.Any("transactionID", txid))

	// Wait for DHCP response (Offer)
	err = c.receiveDHCPResponse(ctx, sock, txid)
	if err != nil {
		return errors.Wrap(err, "failed to read from dhcp socket")
	}

	return nil
}
