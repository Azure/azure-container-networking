package wireserver

import (
	"net"
	"strings"

	"github.com/pkg/errors"
)

var (
	// ErrNoPrimaryInterface indicates the wireserver response does not have a primary interface indicated.
	ErrNoPrimaryInterface = errors.New("no primary interface found")
	// ErrNoSecondaryInterface indicates the wireserver response does not have secondary interface on the node
	ErrNoSecondaryInterface = errors.New("no secondary interface found")
	// ErrInsufficientAddressSpace indicates that the CIDR space is too small to include a gateway IP; it is 1 IP.
	ErrInsufficientAddressSpace = errors.New("insufficient address space to generate gateway IP")
)

func GetPrimaryInterfaceFromResult(res *GetInterfacesResult) (*InterfaceInfo, error) {
	for _, i := range res.Interface {
		// skip if not primary
		if !i.IsPrimary {
			continue
		}

		// skip if no subnets
		if len(i.IPSubnet) == 0 {
			continue
		}

		// get the first subnet
		s := i.IPSubnet[0]
		gw, err := calculateGatewayIP(s.Prefix)
		if err != nil {
			return nil, err
		}

		primaryIP := ""
		for _, ip := range s.IPAddress {
			if ip.IsPrimary {
				primaryIP = ip.Address
			}
		}

		return &InterfaceInfo{
			Subnet:    s.Prefix,
			IsPrimary: true,
			Gateway:   gw.String(),
			PrimaryIP: primaryIP,
		}, nil
	}
	return nil, ErrNoPrimaryInterface
}

// Gets secondary interface details for swiftv2 secondary nics scenario
func GetSecondaryInterfaceFromResult(res *GetInterfacesResult, macAddress string) (*InterfaceInfo, error) {
	for _, i := range res.Interface {
		// skip if primary
		if i.IsPrimary {
			continue
		}

		// skip if no subnets
		if len(i.IPSubnet) == 0 {
			continue
		}

		if macAddressesEqual(i.MacAddress, macAddress) {
			// get the second subnet
			s := i.IPSubnet[0]
			gw, err := calculateGatewayIP(s.Prefix)
			if err != nil {
				return nil, err
			}

			secondaryIP := ""
			for _, ip := range s.IPAddress {
				if !ip.IsPrimary {
					secondaryIP = ip.Address
					break
				}
			}
			var secondaryIPs []string
			secondaryIPs = append(secondaryIPs, secondaryIP)

			return &InterfaceInfo{
				Subnet:       s.Prefix,
				IsPrimary:    false,
				Gateway:      gw.String(),
				SecondaryIPs: secondaryIPs,
			}, nil
		}
	}
	return nil, ErrNoSecondaryInterface
}

// calculateGatewayIP parses the passed CIDR string and returns the first IP in the range.
func calculateGatewayIP(cidr string) (net.IP, error) {
	_, subnet, err := net.ParseCIDR(cidr)
	if err != nil {
		return nil, errors.Wrap(err, "received malformed subnet from host")
	}

	// check if we have enough address space to calculate a gateway IP
	// we need at least 2 IPs (eg the IPv4 mask cannot be greater than 31)
	// since the zeroth is reserved and the gateway is the first.
	mask, bits := subnet.Mask.Size()
	if mask == bits {
		return nil, ErrInsufficientAddressSpace
	}

	// the subnet IP is the zero base address, so we need to increment it by one to get the gateway.
	gw := make([]byte, len(subnet.IP))
	copy(gw, subnet.IP)
	for idx := len(gw) - 1; idx >= 0; idx-- {
		gw[idx]++
		// net.IP is a binary byte array, check if we have overflowed and need to continue incrementing to the left
		// along the arary or if we're done.
		// it's like if we have a 9 in base 10, and add 1, it rolls over to 0 so we're not done - we need to move
		// left and increment that digit also.
		if gw[idx] != 0 {
			break
		}
	}
	return gw, nil
}

func macAddressesEqual(macAddress1, macAddress2 string) bool {
	macAddress1 = strings.ToLower(strings.ReplaceAll(macAddress1, ":", ""))
	macAddress2 = strings.ToLower(strings.ReplaceAll(macAddress2, ":", ""))

	return macAddress1 == macAddress2
}
