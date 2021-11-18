package policy

import (
	"encoding/json"

	"github.com/pkg/errors"
)

const (
	NetworkPolicy     CNIPolicyType = "NetworkPolicy"
	EndpointPolicy    CNIPolicyType = "EndpointPolicy"
	OutBoundNatPolicy CNIPolicyType = "OutBoundNAT"
	RoutePolicy       CNIPolicyType = "ROUTE"
	PortMappingPolicy CNIPolicyType = "NAT"
	ACLPolicy         CNIPolicyType = "ACL"
	L4WFPProxyPolicy  CNIPolicyType = "L4WFPPROXY"
	LoopbackDSRPolicy CNIPolicyType = "LoopbackDSR"
)

type CNIPolicyType string

type Policy struct {
	Type CNIPolicyType
	Data json.RawMessage
}

// NATInfo contains information about NAT rules
type NATInfo struct {
	Destinations []string
	VirtualIP    string
}

// ErrIpv4NotFound ipv4 address not found in endpoint info
var ErrIpv4NotFound = errors.New("ipv4 not found in endpoint info")
