package policy

import (
	"encoding/json"
)

const (
	NetworkPolicy     CNIPolicyType = "NetworkPolicy"
	EndpointPolicy    CNIPolicyType = "EndpointPolicy"
	OutBoundNatPolicy CNIPolicyType = "OutBoundNAT"
	RoutePolicy       CNIPolicyType = "ROUTE"
	PortMappingPolicy CNIPolicyType = "NAT"
	ACLPolicy         CNIPolicyType = "ACL"
)

type CNIPolicyType string

type Policy struct {
	Type CNIPolicyType
	Data json.RawMessage
}
