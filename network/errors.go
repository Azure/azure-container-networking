package network

import "errors"

var (
	errSubnetV6NotFound        = errors.New("Couldn't find ipv6 subnet in network info")                // nolint
	errV6SnatRuleNotSet        = errors.New("ipv6 snat rule not set. Might be VM ipv6 address missing") // nolint
	ErrEndpointStateNotFound   = errors.New("endpoint state could not be found in the statefile")
	ErrConnectionFailure       = errors.New("couldn't connect to CNS")
	ErrGetEndpointStateFailure = errors.New("failure to obtain the endpoint state")
	ErrMissingHNSID            = errors.New("HNS ID missing")
)
