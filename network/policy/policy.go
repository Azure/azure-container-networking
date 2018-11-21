package policy

import (
	"encoding/json"
)

const (
	NetworkPolicy     CNIPolicyType = "NetworkPolicy"
	EndpointPolicy    CNIPolicyType = "EndpointPolicy"
	OutBoundNatPolicy CNIPolicyType = "OutBoundNAT"
)

type CNIPolicyType string

type Policy struct {
	Type CNIPolicyType
	Data json.RawMessage
}
