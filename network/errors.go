package network

import "errors"

var (
	errSubnetV6NotFound = errors.New("Couldn't find ipv6 subnet in network info")
	errV6SnatRuleNotSet = errors.New("ipv6 snat rule not set. Might be VM ipv6 address missing")
)
