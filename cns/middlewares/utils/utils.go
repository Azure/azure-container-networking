package utils

import (
	"fmt"
	"net/netip"
	"strings"
)

// ParseCIDRs parses the comma separated list of CIDRs and returns the IPv4 and IPv6 CIDRs.
func ParseCIDRs(cidrs string) (v4IPs, v6IPs []string, err error) {
	v4IPs = []string{}
	v6IPs = []string{}
	for _, cidr := range strings.Split(cidrs, ",") {
		p, err := netip.ParsePrefix(cidr)
		if err != nil {
			return nil, nil, fmt.Errorf("failed to parse CIDR %s : %w", cidr, err)
		}
		ip := p.Addr()
		if ip.Is4() {
			v4IPs = append(v4IPs, cidr)
		} else {
			v6IPs = append(v6IPs, cidr)
		}
	}
	return v4IPs, v6IPs, nil
}
