package controlplane

import (
	dp "github.com/Azure/azure-container-networking/npm/pkg/dataplane"
	"github.com/Azure/azure-container-networking/npm/pkg/dataplane/ipsets"
)

const (
	IpsetApply      string = "IPSETAPPLY"
	IpsetRemove     string = "IPSETREMOVE"
	PolicyApply     string = "POLICYAPPLY"
	PolicyRemove    string = "POLICYREMOVE"
	ListReference   string = "LISTREFERENCE"
	PolicyReference string = "POLICYREFERENCE"
)

// ControllerIPSets is used in fan-out design for controller pod to calculate
// and push to daemon pod
type ControllerIPSets struct {
	*ipsets.IPSetMetadata
	// IPPodMetadata is used for setMaps to store Ips and ports as keys
	// and podMetadata as value
	IPPodMetadata map[string]*dp.PodMetadata
	// MemberIPSets is used for listMaps to store child IP Sets
	MemberIPSets map[string]*ipsets.IPSetMetadata
	// ipsetReference keeps track of how many lists in the cache refer to this ipset
	ipsetReference map[string]struct{}
	// NetPolReference holds networkpolicy names where this IPSet
	// is being referred as part of rules
	// NetpolReference is not used currently, depending on testing we may decide to keep it
	// or delete it
	NetPolReference map[string]struct{}
}

func NewControllerIPSets(metadata *ipsets.IPSetMetadata) *ControllerIPSets {
	return &ControllerIPSets{
		IPSetMetadata:   metadata,
		IPPodMetadata:   make(map[string]*dp.PodMetadata),
		MemberIPSets:    make(map[string]*ipsets.IPSetMetadata),
		ipsetReference:  make(map[string]struct{}),
		NetPolReference: make(map[string]struct{}),
	}
}

// GetMetadata returns the metadata of the ipset
func (c *ControllerIPSets) GetMetadata() *ipsets.IPSetMetadata {
	return c.IPSetMetadata
}

// HasReferences checks if an ipset has references
func (c *ControllerIPSets) HasReferences() bool {
	return len(c.ipsetReference) > 0 || len(c.NetPolReference) > 0
}

// CanDelete checks for references and members
func (c *ControllerIPSets) CanDelete() bool {
	return c.HasReferences() &&
		(len(c.IPPodMetadata) > 0 || len(c.MemberIPSets) > 0)
}

func (c *ControllerIPSets) AddReference(referenceName, referenceType string) {
	switch referenceType {
	case ListReference:
		c.ipsetReference[referenceName] = struct{}{}
	case PolicyReference:
		c.NetPolReference[referenceName] = struct{}{}
	}
}

func (c *ControllerIPSets) DeleteReference(referenceName, referenceType string) {
	switch referenceType {
	case ListReference:
		delete(c.ipsetReference, referenceName)
	case PolicyReference:
		delete(c.NetPolReference, referenceName)
	}
}
