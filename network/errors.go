package network

import "errors"

var (
	errSubnetV6NotFound         = errors.New("Couldn't find ipv6 subnet in network info")                // nolint
	errV6SnatRuleNotSet         = errors.New("IPv6 snat rule not set. Might be VM ipv6 address missing") // nolint
	ErrEndpointStateNotFound    = errors.New("Endpoint state could not be found in the statefile")
	ErrConnectionFailure        = errors.New("Couldn't connect to CNS")
	ErrEndpointRemovalFailure   = errors.New("Failed to remove endpoint")
	ErrEndpointRetrievalFailure = errors.New("Failed to obtain endpoint")
	ErrGetEndpointStateFailure  = errors.New("Failure to obtain the endpoint state")
)
