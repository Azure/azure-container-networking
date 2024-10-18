package dhcp

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/binary"
	"io"
	"net"
	"time"

	"github.com/pkg/errors"
	"go.uber.org/zap"
	"golang.org/x/net/ipv4"
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
)

type ExecClient interface {
	ExecuteCommand(ctx context.Context, command string, args ...string) (string, error)
}

type NetIOClient interface {
	GetNetworkInterfaceByName(name string) (*net.Interface, error)
	GetNetworkInterfaceAddrs(iface *net.Interface) ([]net.Addr, error)
}

type DHCP struct {
	logger      *zap.Logger
	netioClient NetIOClient
}

func New(logger *zap.Logger, netio NetIOClient) *DHCP {
	return &DHCP{
		logger:      logger,
		netioClient: netio,
	}
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
			c.logger.Info("Received DHCP reply packet", zap.Int("opCode", int(opcode)), zap.Any("transactionID", TransactionID(txid)))
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
