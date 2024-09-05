package dhcp

import (
	"bytes"
	"context"
	"encoding/binary"
	"fmt"
	"log"
	"net"
	"time"

	"github.com/pkg/errors"
	"github.com/u-root/uio/rand"
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

var magicCookie = []byte{0x63, 0x82, 0x53, 0x63} // DHCP magic cookie
var RandomTimeout = 5 * time.Second
var DefaultReadTimeout = 3 * time.Second

type DHCP struct{}

func New() *DHCP {
	return &DHCP{}
}

// GenerateTransactionID generates a random 32-bits number suitable for use as TransactionID
func GenerateTransactionID() (TransactionID, error) {
	var xid TransactionID
	ctx, cancel := context.WithTimeout(context.Background(), RandomTimeout)
	defer cancel()
	n, err := rand.ReadContext(ctx, xid[:])
	if err != nil {
		return xid, errors.Errorf("could not get random number: %v", err)
	}
	if n != 4 {
		return xid, errors.New("invalid random sequence for transaction ID: smaller than 32 bits")
	}
	return xid, err
}

func makeListeningSocket(ifname string) (int, error) {
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
	return fd, errors.Wrap(err, "dhcp failed to bind")
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
		return fd, errors.Wrap(err, "dhcp failed to set sockopt")
	}
	return fd, nil
}

func htons(v uint16) uint16 {
	var tmp [2]byte
	binary.BigEndian.PutUint16(tmp[:], v)
	return binary.LittleEndian.Uint16(tmp[:])
}

func BindToInterface(fd int, ifname string) error {
	return unix.BindToDevice(fd, ifname)
}

// makeRawSocket creates a socket that can be passed to unix.Sendto.
func makeRawSocket(ifname string) (int, error) {
	fd, err := unix.Socket(unix.AF_INET, unix.SOCK_RAW, unix.IPPROTO_RAW)
	if err != nil {
		return fd, errors.Wrap(err, "dhcp raw socket creation failure")
	}
	err = unix.SetsockoptInt(fd, unix.SOL_SOCKET, unix.SO_REUSEADDR, 1)
	if err != nil {
		return fd, errors.Wrap(err, "dhcp failed to set raw sockopt")
	}
	err = unix.SetsockoptInt(fd, unix.IPPROTO_IP, unix.IP_HDRINCL, 1)
	if err != nil {
		return fd, errors.Wrap(err, "dhcp failed to set second raw sockopt")
	}
	err = BindToInterface(fd, ifname)
	if err != nil {
		return fd, errors.Wrap(err, "dhcp failed to bind to interface")
	}
	return fd, nil
}

// Build DHCP Discover Packet
func buildDHCPDiscover(mac net.HardwareAddr, txid TransactionID) ([]byte, error) {
	if len(mac) != 6 {
		return nil, fmt.Errorf("invalid MAC address length")
	}

	var packet bytes.Buffer

	// BOOTP header
	packet.WriteByte(opRequest)                            // op: BOOTREQUEST (1)
	packet.WriteByte(htypeEthernet)                        // htype: Ethernet (1)
	packet.WriteByte(hlenEthernet)                         // hlen: MAC address length (6)
	packet.WriteByte(hops)                                 // hops: 0
	packet.Write(txid[:])                                  // xid: Transaction ID (4 bytes)
	binary.Write(&packet, binary.BigEndian, uint16(secs))  // secs: Seconds elapsed
	binary.Write(&packet, binary.BigEndian, uint16(flags)) // flags: Broadcast flag

	// Client IP address (0.0.0.0)
	packet.Write(make([]byte, 4))
	// Your IP address (0.0.0.0)
	packet.Write(make([]byte, 4))
	// Server IP address (0.0.0.0)
	packet.Write(make([]byte, 4))
	// Gateway IP address (0.0.0.0)
	packet.Write(make([]byte, 4))

	// chaddr: Client hardware address (MAC address)

	packet.Write(mac)              // MAC address
	packet.Write(make([]byte, 10)) // Padding to 16 bytes

	// sname: Server host name (64 bytes)
	packet.Write(make([]byte, 64))
	// file: Boot file name (128 bytes)
	packet.Write(make([]byte, 128))

	// Magic cookie (DHCP)
	binary.Write(&packet, binary.BigEndian, magicCookie)

	// DHCP options (minimal required options for DISCOVER)
	packet.Write([]byte{
		53, 1, 1, // Option 53: DHCP Message Type (1 = DHCP Discover)
		55, 3, 1, 3, 6, // Option 55: Parameter Request List (1 = Subnet Mask, 3 = Router, 6 = DNS)
		255, // End option
	})

	// padding length to 300 bytes
	var value uint8 = uint8(0)
	if packet.Len() < bootpMinLen {
		packet.Write(bytes.Repeat([]byte{value}, bootpMinLen-packet.Len()))
	}

	return packet.Bytes(), nil
}

// MakeRawUDPPacket converts a payload (a serialized packet) into a
// raw UDP packet for the specified serverAddr from the specified clientAddr.
func MakeRawUDPPacket(payload []byte, serverAddr, clientAddr net.UDPAddr) ([]byte, error) {
	udp := make([]byte, 8)
	binary.BigEndian.PutUint16(udp[:2], uint16(clientAddr.Port))
	binary.BigEndian.PutUint16(udp[2:4], uint16(serverAddr.Port))
	binary.BigEndian.PutUint16(udp[4:6], uint16(8+len(payload)))
	binary.BigEndian.PutUint16(udp[6:8], 0) // try to offload the checksum

	h := ipv4.Header{
		Version:  4,
		Len:      20,
		TotalLen: 20 + len(udp) + len(payload),
		TTL:      64,
		Protocol: 17, // UDP
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

// Send DHCP discover packet using unix.RawConn
func sendDHCPDiscover(fd int, packet []byte) error {
	raddr := &net.UDPAddr{IP: net.IPv4bcast, Port: dhcpServerPort}
	laddr := &net.UDPAddr{IP: net.IPv4zero, Port: dhcpClientPort}
	var (
		destination [net.IPv4len]byte
	)
	copy(destination[:], raddr.IP.To4())

	packetBytes, err := MakeRawUDPPacket(packet, *raddr, *laddr)
	if err != nil {
		return errors.Wrap(err, "error making raw udp packet")
	}

	// Create sockaddr_ll structure for sending the packet
	remoteAddr := unix.SockaddrInet4{Port: laddr.Port, Addr: destination}

	// Send the packet using the raw socket
	return unix.Sendto(fd, packetBytes, 0, &remoteAddr)
}

// Receive DHCP response packet using unix.Recvfrom
func receiveDHCPResponse(fd int, xid TransactionID) error {
	recvErrors := make(chan error, 1)
	go func(errs chan<- error) {
		// set read timeout
		timeout := unix.NsecToTimeval(DefaultReadTimeout.Nanoseconds())
		if innerErr := unix.SetsockoptTimeval(fd, unix.SOL_SOCKET, unix.SO_RCVTIMEO, &timeout); innerErr != nil {
			errs <- innerErr
			return
		}
		// loop will only exit if there is an error, or we find our reply packet
		for {
			buf := make([]byte, MaxUDPReceivedPacketSize)
			n, _, innerErr := unix.Recvfrom(fd, buf, 0)
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
			if iph.Protocol != 17 {
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

			log.Printf("potential candidate packet found %d", payload[0])

			opcode := payload[0] // opcode is first byte
			if opcode != dhcpOpCodeReply {
				continue // opcode is not a reply, so continue
			}

			txidOffset := 4 // after 4 bytes, the txid starts
			// the txid is 4 bytes, so we take four bytes after the offset
			txid := payload[txidOffset : txidOffset+4]
			log.Printf("txid found: %v, want: %v", txid, xid)
			if TransactionID(txid) == xid {
				log.Printf("success! opcode: %d", opcode)
				break
			}
		}
		// only occurs if we find our reply packet successfully
		// a nil error means a reply was found for this txid
		log.Print("received a response!")
		recvErrors <- nil
	}(recvErrors)

	select {
	case err := <-recvErrors:
		if err != nil {
			return errors.Wrap(err, "error during receiving")
		}
	case <-time.After(time.Second * 3):
		log.Fatal("timed out waiting for replies")
		return errors.New("timed out waiting for replies")
	}
	return nil

}

// Issues a DHCP Discover packet from the nic specified by mac and name ifname
// Returns nil if a reply to the transaction was received, or error if time out
// Does not return the DHCP Offer that was received from the DHCP server
func (c *DHCP) DiscoverRequest(mac net.HardwareAddr, ifname string) error {
	txid, err := GenerateTransactionID()
	if err != nil {
		return errors.Wrap(err, "failed to generate random transaction id")
	}

	// Build a DHCP discover packet
	packet, err := buildDHCPDiscover(mac, txid)
	if err != nil {
		return errors.Wrap(err, "failed to build dhcp discover packet")
	}

	// get send and receive file descriptors
	sfd, err := MakeBroadcastSocket(ifname)
	if err != nil {
		return errors.Wrap(err, "failed to make broadcast socket")
	}

	rfd, err := makeListeningSocket(ifname)
	if err != nil {
		return errors.Wrap(err, "failed to make listening socket")
	}

	// Send the DHCP discover packet
	err = sendDHCPDiscover(sfd, packet)
	if err != nil {
		return errors.Wrap(err, "failed to send dhcp discover packet")
	}

	fmt.Println("DHCP Discover packet sent successfully!")

	// Wait for DHCP response (Offer)
	res := receiveDHCPResponse(rfd, txid)
	log.Printf("Response error: %v", res)

	return nil
}
