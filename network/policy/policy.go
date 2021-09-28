package policy

import (
	"encoding/json"
)

const (
	// NcPrimaryIPKey indicates constant for the key string
	NcPrimaryIPKey string = "NCPrimaryIPKey"
)

const (
	NetworkPolicy     CNIPolicyType = "NetworkPolicy"
	EndpointPolicy    CNIPolicyType = "EndpointPolicy"
	OutBoundNatPolicy CNIPolicyType = "OutBoundNAT"
	RoutePolicy       CNIPolicyType = "ROUTE"
	PortMappingPolicy CNIPolicyType = "NAT"
	ACLPolicy         CNIPolicyType = "ACL"
	L4WFPProxyPolicy  CNIPolicyType = "L4WFPPROXY"
)

type CNIPolicyType string

type Policy struct {
	Type CNIPolicyType
	Data json.RawMessage
}
