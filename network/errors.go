package network

import "errors"

var (
	errSubnetV6NotFound        = errors.New("Couldn't find ipv6 subnet in network info")
	errV6SnatRuleNotSet        = errors.New("ipv6 snat rule not set. Might be VM ipv6 address missing")
	ErrEndpointStateNotFound   = errors.New("endpoint state could not be found in the statefile")
	ErrConnectionFailure       = errors.New("couldn't connect to CNS daemonset")
	ErrGetEndpointStateFailure = errors.New("failure to obtain the endpoint state")
)
