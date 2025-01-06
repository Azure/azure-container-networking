//go:build linux
// +build linux

package dhcp

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/binary"
	"io"
	"net"
	"net/http"
	"strings"
	"time"

	"github.com/pkg/errors"
	"go.uber.org/zap"
	"golang.org/x/net/ipv4"
	"golang.org/x/sys/unix"
)

const (
	dhcpDiscover             = 1
	bootRequest              = 1
	ethPAll                  = 0x0003
	MaxUDPReceivedPacketSize = 8192
	dhcpServerPort           = 67
	dhcpClientPort           = 68
	dhcpOpCodeReply          = 2
	bootpMinLen              = 300
	bytesInAddress           = 4 // bytes in an ip address
	macBytes                 = 6 // bytes in a mac address
	udpProtocol              = 17

	opRequest     = 1
	htypeEthernet = 1
	hlenEthernet  = 6
	hops          = 0
	secs          = 0
	flags         = 0x8000 // Broadcast flag

	nmAgentSupportedApisURL = "http://168.63.129.16/machine/plugins/?comp=nmagent&type=GetSupportedApis"
	dhcpRehydrationAPIStr   = "SwiftV2DhcpRehydrationFromGoalState" // Set value provided by host
)

// TransactionID represents a 4-byte DHCP transaction ID as defined in RFC 951,
// Section 3.
//
// The TransactionID is used to match DHCP replies to their original request.
type TransactionID [4]byte

var (
	magicCookie        = []byte{0x63, 0x82, 0x53, 0x63} // DHCP magic cookie
	DefaultReadTimeout = 3 * time.Second
	DefaultTimeout     = 3 * time.Second

	errResponseNotOK = errors.New("not ok http status")
)

type DHCP struct {
	logger *zap.Logger
}

func New(logger *zap.Logger) *DHCP {
	return &DHCP{
		logger: logger,
	}
}

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

// GenerateTransactionID generates a random 32-bits number suitable for use as TransactionID
func GenerateTransactionID() (TransactionID, error) {
	var xid TransactionID
	_, err := rand.Read(xid[:])
	if err != nil {
		return xid, errors.Errorf("could not get random number: %v", err)
	}
	return xid, nil
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

// Build DHCP Discover Packet
func buildDHCPDiscover(mac net.HardwareAddr, txid TransactionID) ([]byte, error) {
	if len(mac) != macBytes {
		return nil, errors.Errorf("invalid MAC address length")
	}

	var packet bytes.Buffer

	// BOOTP header
	packet.WriteByte(opRequest)                                  // op: BOOTREQUEST (1)
	packet.WriteByte(htypeEthernet)                              // htype: Ethernet (1)
	packet.WriteByte(hlenEthernet)                               // hlen: MAC address length (6)
	packet.WriteByte(hops)                                       // hops: 0
	packet.Write(txid[:])                                        // xid: Transaction ID (4 bytes)
	err := binary.Write(&packet, binary.BigEndian, uint16(secs)) // secs: Seconds elapsed
	if err != nil {
		return nil, errors.Wrap(err, "failed to write seconds elapsed")
	}
	err = binary.Write(&packet, binary.BigEndian, uint16(flags)) // flags: Broadcast flag
	if err != nil {
		return nil, errors.Wrap(err, "failed to write broadcast flag")
	}

	// Client IP address (0.0.0.0)
	packet.Write(make([]byte, bytesInAddress))
	// Your IP address (0.0.0.0)
	packet.Write(make([]byte, bytesInAddress))
	// Server IP address (0.0.0.0)
	packet.Write(make([]byte, bytesInAddress))
	// Gateway IP address (0.0.0.0)
	packet.Write(make([]byte, bytesInAddress))

	// chaddr: Client hardware address (MAC address)
	paddingBytes := 10
	packet.Write(mac)                        // MAC address (6 bytes)
	packet.Write(make([]byte, paddingBytes)) // Padding to 16 bytes

	// sname: Server host name (64 bytes)
	serverHostNameBytes := 64
	packet.Write(make([]byte, serverHostNameBytes))
	// file: Boot file name (128 bytes)
	bootFileNameBytes := 128
	packet.Write(make([]byte, bootFileNameBytes))

	// Magic cookie (DHCP)
	err = binary.Write(&packet, binary.BigEndian, magicCookie)
	if err != nil {
		return nil, errors.Wrap(err, "failed to write magic cookie")
	}

	// DHCP options (minimal required options for DISCOVER)
	packet.Write([]byte{
		53, 1, 1, // Option 53: DHCP Message Type (1 = DHCP Discover)
		55, 3, 1, 3, 6, // Option 55: Parameter Request List (1 = Subnet Mask, 3 = Router, 6 = DNS)
		255, // End option
	})

	// padding length to 300 bytes
	var value uint8 // default is zero
	if packet.Len() < bootpMinLen {
		packet.Write(bytes.Repeat([]byte{value}, bootpMinLen-packet.Len()))
	}

	return packet.Bytes(), nil
}

// MakeRawUDPPacket converts a payload (a serialized packet) into a
// raw UDP packet for the specified serverAddr from the specified clientAddr.
func MakeRawUDPPacket(payload []byte, serverAddr, clientAddr net.UDPAddr) ([]byte, error) {
	udpBytes := 8
	udp := make([]byte, udpBytes)
	binary.BigEndian.PutUint16(udp[:2], uint16(clientAddr.Port))
	binary.BigEndian.PutUint16(udp[2:4], uint16(serverAddr.Port))
	totalLen := uint16(udpBytes + len(payload))
	binary.BigEndian.PutUint16(udp[4:6], totalLen)
	binary.BigEndian.PutUint16(udp[6:8], 0) // try to offload the checksum

	headerVersion := 4
	headerLen := 20
	headerTTL := 64

	h := ipv4.Header{
		Version:  headerVersion, // nolint
		Len:      headerLen,     // nolint
		TotalLen: headerLen + len(udp) + len(payload),
		TTL:      headerTTL,
		Protocol: udpProtocol, // UDP
		Dst:      serverAddr.IP,
		Src:      clientAddr.IP,
	}
	ret, err := h.Marshal()
	if err != nil {
		return nil, errors.Wrap(err, "failed to marshal when making udp packet")
	}
	ret = append(ret, udp...)
	ret = append(ret, payload...)
	return ret, nil
}

// Receive DHCP response packet using reader
func (c *DHCP) receiveDHCPResponse(ctx context.Context, reader io.ReadCloser, xid TransactionID) error {
	recvErrors := make(chan error, 1)
	// Recvfrom is a blocking call, so if something goes wrong with its timeout it won't return.

	// Additionally, the timeout on the socket (on the Read(...)) call is how long until the socket times out and gives an error,
	// but it won't error if we do get some sort of data within the time out period.

	// If we get some data (even if it is not the packet we are looking for, like wrong txid, wrong response opcode etc.)
	// then we continue in the for loop. We then call recvfrom again which will reset the timeout period
	// Without the secondary timeout at the bottom of the function, we could stay stuck in the for loop as long as we receive packets.
	go func(errs chan<- error) {
		// loop will only exit if there is an error, context canceled, or we find our reply packet
		for {
			if ctx.Err() != nil {
				errs <- ctx.Err()
				return
			}

			buf := make([]byte, MaxUDPReceivedPacketSize)
			// Blocks until data received or timeout period is reached
			n, innerErr := reader.Read(buf)
			if innerErr != nil {
				errs <- innerErr
				return
			}
			// check header
			var iph ipv4.Header
			if err := iph.Parse(buf[:n]); err != nil {
				// skip non-IP data
				continue
			}
			if iph.Protocol != udpProtocol {
				// skip non-UDP packets
				continue
			}
			udph := buf[iph.Len:n]
			// source is from dhcp server if receiving
			srcPort := int(binary.BigEndian.Uint16(udph[0:2]))
			if srcPort != dhcpServerPort {
				continue
			}
			// client is to dhcp client if receiving
			dstPort := int(binary.BigEndian.Uint16(udph[2:4]))
			if dstPort != dhcpClientPort {
				continue
			}
			// check payload
			pLen := int(binary.BigEndian.Uint16(udph[4:6]))
			payload := buf[iph.Len+8 : iph.Len+pLen]

			// retrieve opcode from payload
			opcode := payload[0] // opcode is first byte
			// retrieve txid from payload
			txidOffset := 4 // after 4 bytes, the txid starts
			// the txid is 4 bytes, so we take four bytes after the offset
			txid := payload[txidOffset : txidOffset+4]

			c.logger.Info("Received packet", zap.Int("opCode", int(opcode)), zap.Any("transactionID", TransactionID(txid)))
			if opcode != dhcpOpCodeReply {
				continue // opcode is not a reply, so continue
			}

			if TransactionID(txid) == xid {
				break
			}
		}
		// only occurs if we find our reply packet successfully
		// a nil error means a reply was found for this txid
		recvErrors <- nil
	}(recvErrors)

	// sends a message on repeat after timeout, but only the first one matters
	ticker := time.NewTicker(DefaultReadTimeout)
	defer ticker.Stop()

	select {
	case err := <-recvErrors:
		if err != nil {
			return errors.Wrap(err, "error during receiving")
		}
	case <-ticker.C:
		return errors.New("timed out waiting for replies")
	}
	return nil
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

// DHCPRehydrationFeatureOnHost queries the host to check if a particular API is supported,
// and if so, returns true; if we are unable to determine if the feature is enabled, an error
// is returned
func (c *DHCP) DHCPRehydrationFeatureOnHost(ctx context.Context) (bool, error) {
	var (
		resp       *http.Response
		httpClient = &http.Client{Timeout: DefaultTimeout}
	)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, nmAgentSupportedApisURL, nil)
	if err != nil {
		return false, errors.Wrap(err, "failed to create http request")
	}
	req.Header.Add("Metadata", "true")
	req.Header.Add("x-ms-version", "2012-11-30")
	resp, err = httpClient.Do(req)
	if err != nil {
		return false, errors.Wrap(err, "error issuing http request")
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return false, errResponseNotOK
	}
	bytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return false, errors.Wrap(err, "failed to read response body")
	}
	str := string(bytes)
	if !strings.Contains(str, dhcpRehydrationAPIStr) {
		return false, nil
	}

	return true, nil
}
