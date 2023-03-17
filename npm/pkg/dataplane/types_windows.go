package dataplane

import "github.com/Microsoft/hcsshim/hcn"

const unspecifiedPodKey = ""

const (
	hcnEndpointStateCreated = iota + 1
	hcnEndpointStateAttached
	hcnEndpointStateAttachedSharing
	hcnEndpointStateDetached
	hcnEndpointStateDegraded
	hcnEndpointStateDestroyed
)

// npmEndpoint holds info relevant for endpoints in windows
type npmEndpoint struct {
	name   string
	id     string
	ip     string
	podKey string
	// previousIncorrectPodKey represents a Pod that was previously and incorrectly assigned to this endpoint (see issue 1729)
	previousIncorrectPodKey string
	// Map with Key as Network Policy name to to emulate set
	// and value as struct{} for minimal memory consumption
	netPolReference map[string]struct{}
}

// newNPMEndpoint initializes npmEndpoint and copies relevant information from hcn.HostComputeEndpoint.
// This function must be defined in a file with a windows build tag for proper vendoring since it uses the hcn pkg
func newNPMEndpoint(endpoint *hcn.HostComputeEndpoint) *npmEndpoint {
	return &npmEndpoint{
		name:            endpoint.Name,
		id:              endpoint.Id,
		podKey:          unspecifiedPodKey,
		netPolReference: make(map[string]struct{}),
		ip:              endpoint.IpConfigurations[0].IpAddress,
	}
}

type endpointQuery struct {
	query hcn.HostComputeQuery
}
