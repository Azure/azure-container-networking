package dataplane

import (
	"github.com/Azure/azure-container-networking/npm"
	"github.com/Azure/azure-container-networking/npm/pkg/dataplane/ipsets"
	"github.com/Azure/azure-container-networking/npm/pkg/dataplane/policies"
)

type DataPlane struct {
	policyMgr policies.PolicyManager
	ipsetMgr  ipsets.IPSetManager
	networkID string
	// key is PodKey
	endpointCache map[string]*NPMEndpoint
}

type NPMEndpoint struct {
	Name string
	ID   string
	// Using a map to emulate set and value as struct{} for
	// minimal memory consumption
	NetPolReference map[string]struct{}
}

func NewDataPlane() *DataPlane {
	return &DataPlane{
		policyMgr:     policies.NewPolicyManager(),
		ipsetMgr:      ipsets.NewIPSetManager(),
		endpointCache: make(map[string]*NPMEndpoint),
	}
}

// InitializeDataPlane helps in setting up dataplane for NPM
func (dp *DataPlane) InitializeDataPlane() error {
	return dp.initializeDataPlane()
}

// ResetDataPlane helps in cleaning up dataplane sets and policies programmed
// by NPM, retunring a clean slate
func (dp *DataPlane) ResetDataPlane() error {
	return dp.resetDataPlane()
}

// CreateIPSet takes in a set object and updates local cache with this set
func (dp *DataPlane) CreateIPSet(set *ipsets.IPSet) error {
	return dp.ipsetMgr.CreateIPSet(set)
}

// DeleteSet checks for members and references of the given "set" type ipset
// if not used then will delete it from cache
func (dp *DataPlane) DeleteSet(name string) error {
	return dp.ipsetMgr.DeleteSet(name)
}

// DeleteList sanity checks and deletes a list ipset
func (dp *DataPlane) DeleteList(name string) error {
	return dp.ipsetMgr.DeleteList(name)
}

// AddToSet takes in a list of IPset objects along with IP member
// and then updates it local cache
func (dp *DataPlane) AddToSet(setNames []*ipsets.IPSet, ip, podKey string) error {
	return dp.ipsetMgr.AddToSet(setNames, ip, podKey)
}

// RemoveFromSet takes in list of setnames from which a given IP member should be
// removed and will update the local cache
func (dp *DataPlane) RemoveFromSet(setNames []string, ip, podKey string) error {
	return dp.ipsetMgr.RemoveFromSet(setNames, ip, podKey)
}

// AddToList takes a list name and list of sets which are to be added as members
// to given list
func (dp *DataPlane) AddToList(listName string, setNames []string) error {
	return dp.ipsetMgr.AddToList(listName, setNames)
}

// RemoveFromList takes a list name and list of sets which are to be removed as members
// to given list
func (dp *DataPlane) RemoveFromList(listName string, setNames []string) error {
	return dp.ipsetMgr.RemoveFromList(listName, setNames)
}

// UpdatePod is to be called by pod_controller ONLY when a new pod is CREATED.
func (dp *DataPlane) UpdatePod(pod *npm.NpmPod) error {
	return dp.updatePod(pod)
}

// ApplyDataPlane all the IPSet operations just update cache and update a dirty ipset structure,
// they do not change apply changes into dataplane. This function needs to be called at the
// end of IPSet operations of a given controller event, it will check for the dirty ipset list
// and accordingly makes changes in dataplane. This function helps emulate a single call to
// dataplane instead of multiple ipset operations calls ipset operations calls to dataplane
func (dp *DataPlane) ApplyDataPlane() error {
	return dp.ipsetMgr.ApplyIPSets(dp.networkID)
}

// AddPolicies takes in a translated NPMNetworkPolicy object and applies on dataplane
func (dp *DataPlane) AddPolicies(policies *policies.NPMNetworkPolicy) error {
	return dp.policyMgr.AddPolicies(policies)
}

// RemovePolicies takes in network policy name and removes it from dataplane and cache
func (dp *DataPlane) RemovePolicies(policyName string) error {
	return dp.policyMgr.RemovePolicies(policyName)
}

// UpdatePolicies takes in updated policy object, calculates the delta and applies changes
// onto dataplane accordingly
func (dp *DataPlane) UpdatePolicies(policies *policies.NPMNetworkPolicy) error {
	return dp.policyMgr.UpdatePolicies(policies)
}
