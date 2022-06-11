package dataplane

import "github.com/Microsoft/hcsshim/hcn"

const (
	unspecifiedPodKey        = ""
	minutesToKeepStalePodKey = 10
)

// npmEndpoint holds info relevant for endpoints in windows
type npmEndpoint struct {
	name   string
	id     string
	ip     string
	podKey string
	// stalePodKey is used to keep track of the previous pod that had this IP
	stalePodKey *staleKey
	// Map with Key as Network Policy name to to emulate set
	// and value as struct{} for minimal memory consumption
	netPolReference map[string]struct{}
}

type staleKey struct {
	key string
	// timestamp represents the Unix time this struct was created
	timestamp int64
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

func (ep *npmEndpoint) isStalePodKey(podKey string) bool {
	return ep.stalePodKey != nil && ep.stalePodKey.key == podKey
}
