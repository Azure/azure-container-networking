package utils

import (
	"net/netip"
	"strings"

	"github.com/pkg/errors"
)

// ParseCIDRs parses the comma-separated list of CIDRs and returns the IPv4 and IPv6 CIDRs.
func ParseCIDRs(cidrs string) (v4IPs, v6IPs []string, err error) {
	v4IPs = []string{}
	v6IPs = []string{}
	for _, cidr := range strings.Split(cidrs, ",") {
		p, err := netip.ParsePrefix(cidr)
		if err != nil {
			return nil, nil, errors.Wrapf(err, "failed to parse CIDR %s", cidr)
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

// ParseIPAndPrefix parses the primaryIP and returns the IP address and prefix length.
func ParseIPAndPrefix(primaryIP string) (ip string, prefixSize int, err error) {
	p, err := netip.ParsePrefix(primaryIP)
	if err != nil {
		err = errors.Wrapf(err, "failed to parse IP %s", primaryIP)
		return "", 0, err
	}
	ip = p.Addr().String()
	prefixSize = p.Bits()
	return ip, prefixSize, nil
}
