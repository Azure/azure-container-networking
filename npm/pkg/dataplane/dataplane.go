package dataplane

import (
	"fmt"

	"github.com/Azure/azure-container-networking/npm"
	"github.com/Azure/azure-container-networking/npm/pkg/dataplane/ipsets"
	"github.com/Azure/azure-container-networking/npm/pkg/dataplane/policies"
)

type DataPlane struct {
	policyMgr *policies.PolicyManager
	ipsetMgr  *ipsets.IPSetManager
	networkID string
	// key is PodKey
	endpointCache map[string]*NPMEndpoint
}

type NPMEndpoint struct {
	Name string
	ID   string
	// Map with Key as Network Policy name to to emulate set
	// and value as struct{} for minimal memory consumption
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
func (dp *DataPlane) CreateIPSet(setName string, setType ipsets.SetType) error {
	err := dp.ipsetMgr.CreateIPSet(setName, setType)
	if err != nil {
		return fmt.Errorf("[DataPlane] error while creating set: %w", err)
	}
	return nil
}

// DeleteSet checks for members and references of the given "set" type ipset
// if not used then will delete it from cache
func (dp *DataPlane) DeleteIPSet(name string) error {
	err := dp.ipsetMgr.DeleteIPSet(name)
	if err != nil {
		return fmt.Errorf("[DataPlane] error while deleting set: %w", err)
	}
	return nil
}

// AddToSet takes in a list of IPSet names along with IP member
// and then updates it local cache
func (dp *DataPlane) AddToSet(setNames []string, ip, podKey string) error {
	err := dp.ipsetMgr.AddToSet(setNames, ip, podKey)
	if err != nil {
		return fmt.Errorf("[DataPlane] error while adding to set: %w", err)
	}
	return nil
}

// RemoveFromSet takes in list of setnames from which a given IP member should be
// removed and will update the local cache
func (dp *DataPlane) RemoveFromSet(setNames []string, ip, podKey string) error {
	err := dp.ipsetMgr.RemoveFromSet(setNames, ip, podKey)
	if err != nil {
		return fmt.Errorf("[DataPlane] error while removing from set: %w", err)
	}
	return nil
}

// AddToList takes a list name and list of sets which are to be added as members
// to given list
func (dp *DataPlane) AddToList(listName string, setNames []string) error {
	err := dp.ipsetMgr.AddToList(listName, setNames)
	if err != nil {
		return fmt.Errorf("[DataPlane] error while adding to list: %w", err)
	}
	return nil
}

// RemoveFromList takes a list name and list of sets which are to be removed as members
// to given list
func (dp *DataPlane) RemoveFromList(listName string, setNames []string) error {
	err := dp.ipsetMgr.RemoveFromList(listName, setNames)
	if err != nil {
		return fmt.Errorf("[DataPlane] error while removing from list: %w", err)
	}
	return nil
}

// UpdatePod is to be called by pod_controller ONLY when a new pod is CREATED.
func (dp *DataPlane) UpdatePod(pod *npm.NpmPod) error {
	err := dp.updatePod(pod)
	if err != nil {
		return fmt.Errorf("[DataPlane] error while updating pod: %w", err)
	}
	return nil
}

// ApplyDataPlane all the IPSet operations just update cache and update a dirty ipset structure,
// they do not change apply changes into dataplane. This function needs to be called at the
// end of IPSet operations of a given controller event, it will check for the dirty ipset list
// and accordingly makes changes in dataplane. This function helps emulate a single call to
// dataplane instead of multiple ipset operations calls ipset operations calls to dataplane
func (dp *DataPlane) ApplyDataPlane() error {
	err := dp.ipsetMgr.ApplyIPSets(dp.networkID)
	if err != nil {
		return fmt.Errorf("[DataPlane] error while applying IPSets: %w", err)
	}
	return nil
}

// AddPolicy takes in a translated NPMNetworkPolicy object and applies on dataplane
func (dp *DataPlane) AddPolicy(policies *policies.NPMNetworkPolicy) error {
	err := dp.policyMgr.AddPolicy(policies)
	if err != nil {
		return fmt.Errorf("[DataPlane] error while adding policy: %w", err)
	}
	return nil
}

// RemovePolicy takes in network policy name and removes it from dataplane and cache
func (dp *DataPlane) RemovePolicy(policyName string) error {
	err := dp.policyMgr.RemovePolicy(policyName)
	if err != nil {
		return fmt.Errorf("[DataPlane] error while removing policy: %w", err)
	}
	return nil
}

// UpdatePolicy takes in updated policy object, calculates the delta and applies changes
// onto dataplane accordingly
func (dp *DataPlane) UpdatePolicy(policies *policies.NPMNetworkPolicy) error {
	err := dp.policyMgr.UpdatePolicy(policies)
	if err != nil {
		return fmt.Errorf("[DataPlane] error while updating policy: %w", err)
	}
	return nil
}
