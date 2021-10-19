package wireserver

import (
	"net"

	"github.com/pkg/errors"
)

var (
	// ErrNoPrimaryInterface indicates the wireserver respnose does not have a primary interface indicated.
	ErrNoPrimaryInterface = errors.New("no primary interface found")
	// ErrInsufficientAddressSpace indicates that the CIDR space is too small to include a gateway IP; it is 1 IP.
	ErrInsufficientAddressSpace = errors.New("insufficient address space to generate gateway IP")
)

func GetPrimaryInterfaceFromResult(res *GetInterfacesResult) (*InterfaceInfo, error) {
	interfaceInfo := &InterfaceInfo{}
	found := false
	// For each interface.
	for _, i := range res.Interface {
		// skip if not primary
		if !i.IsPrimary {
			continue
		}
		interfaceInfo.IsPrimary = true

		// skip if no subnets
		if len(i.IPSubnet) == 0 {
			continue
		}

		// get the first subnet
		s := i.IPSubnet[0]
		interfaceInfo.Subnet = s.Prefix
		gw, err := calculateGatewayIP(s.Prefix)
		if err != nil {
			return nil, err
		}
		interfaceInfo.Gateway = gw.String()
		for _, ip := range s.IPAddress {
			if ip.IsPrimary {
				interfaceInfo.PrimaryIP = ip.Address
			}
		}

		found = true
		break
	}

	if !found {
		return nil, ErrNoPrimaryInterface
	}

	return interfaceInfo, nil
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
