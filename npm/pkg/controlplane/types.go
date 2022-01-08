package controlplane

import (
	dp "github.com/Azure/azure-container-networking/npm/pkg/dataplane"
	"github.com/Azure/azure-container-networking/npm/pkg/dataplane/ipsets"
)

const (
	IpsetApply   string = "IPSETAPPLY"
	IpsetRemove  string = "IPSETREMOVE"
	PolicyApply  string = "POLICYAPPLY"
	PolicyRemove string = "POLICYREMOVE"
)

// ControllerIPSets is used in decoupled mode for controller pod to calculate
// and push to daemon pod
type ControllerIPSets struct {
	ipsets.IPSetMetadata
	// IPPodMetadata is used for setMaps to store Ips and ports as keys
	// and podMetadata as value
	IPPodMetadata map[string]*dp.PodMetadata
	// This is used for listMaps to store child IP Sets
	MemberIPSets map[string]*ipsets.IPSetMetadata
}

func (c *ControllerIPSets) GetMetadata() *ipsets.IPSetMetadata {
	return &c.IPSetMetadata
}
