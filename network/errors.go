package network

import "errors"

var (
	errSubnetV6NotFound         = errors.New("couldn't find ipv6 subnet in network info")                // nolint
	errV6SnatRuleNotSet         = errors.New("ipv6 snat rule not set. Might be VM ipv6 address missing") // nolint
	ErrEndpointStateNotFound    = errors.New("endpoint state could not be found in the statefile")
	ErrConnectionFailure        = errors.New("couldn't connect to CNS")
	ErrEndpointRemovalFailure   = errors.New("failed to remove endpoint")
	ErrEndpointRetrievalFailure = errors.New("failed to obtain endpoint")
	ErrGetEndpointStateFailure  = errors.New("failure to obtain the endpoint state")
)
