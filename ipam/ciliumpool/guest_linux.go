package ciliumpool

import (
	"bytes"
	"fmt"
	"net"
	"net/netip"
	"slices"

	"github.com/Azure/azure-container-networking/nmagent"
)

type Guest struct {
	VMID       string
	InstanceID string
	BootID     string
	Primary    netip.Addr
	MAC        net.HardwareAddr
	Supply     map[string]bool
}

func parseInterfaces(doc nmagent.Interfaces, mac net.HardwareAddr, subnet netip.Prefix, host []netip.Addr) (Guest, error) {
	result := Guest{MAC: mac, Supply: map[string]bool{}}
	found := false
	for _, iface := range doc.Entries {
		if !bytes.Equal([]byte(iface.MacAddress), mac) {
			continue
		}
		if found || !iface.IsPrimary {
			return Guest{}, fmt.Errorf("%w: selected NIC is not unique primary", ErrConfig)
		}
		found = true
		for _, sub := range iface.InterfaceSubnets {
			if sub.Prefix != subnet.String() {
				continue
			}
			for _, entry := range sub.IPAddress {
				ip := netip.Addr(entry.Address)
				if !ip.Is4() || !subnet.Contains(ip) {
					return Guest{}, fmt.Errorf("%w: invalid guest address", ErrConfig)
				}
				if entry.IsPrimary {
					if result.Primary.IsValid() {
						return Guest{}, fmt.Errorf("%w: multiple guest primary addresses", ErrConfig)
					}
					result.Primary = ip
				} else {
					if slices.Contains(host, ip) {
						return Guest{}, fmt.Errorf("%w: secondary address %s is configured on the host", ErrConfig, ip)
					}
					result.Supply[ip.String()] = true
				}
			}
		}
	}
	if !found || !result.Primary.IsValid() || !slices.Contains(host, result.Primary) {
		return Guest{}, fmt.Errorf("%w: primary NIC or subnet is absent", ErrConfig)
	}
	delete(result.Supply, result.Primary.String())
	return result, nil
}
