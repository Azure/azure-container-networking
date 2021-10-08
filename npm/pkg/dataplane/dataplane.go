package dataplane

import (
	"fmt"

	"github.com/Azure/azure-container-networking/npm/pkg/dataplane/ipsets"
	"github.com/Azure/azure-container-networking/npm/pkg/dataplane/policies"
	npmerrors "github.com/Azure/azure-container-networking/npm/util/errors"
	"k8s.io/klog"
)

const (
	// AzureNetworkName is default network Azure CNI creates
	AzureNetworkName = "azure"
)

type DataPlane struct {
	policyMgr *policies.PolicyManager
	ipsetMgr  *ipsets.IPSetManager
	networkID string
	nodeName  string
	// key is PodKey
	endpointCache map[string]*NPMEndpoint
}

type NPMEndpoint struct {
	Name string
	ID   string
	IP   string
	// Map with Key as Network Policy name to to emulate set
	// and value as struct{} for minimal memory consumption
	NetPolReference map[string]struct{}
}

// UpdateNPMPod pod controller will populate and send this datastructure to dataplane
// to update the dataplane with the latest pod information
// this helps in calculating if any update needs to have policies applied or removed
type UpdateNPMPod struct {
	Name           string
	Namespace      string
	PodIP          string
	NodeName       string
	IPSetsToAdd    []string
	IPSetsToRemove []string
}

func NewDataPlane(nodeName string) *DataPlane {
	return &DataPlane{
		policyMgr:     policies.NewPolicyManager(),
		ipsetMgr:      ipsets.NewIPSetManager(AzureNetworkName),
		endpointCache: make(map[string]*NPMEndpoint),
		nodeName:      nodeName,
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
func (dp *DataPlane) CreateIPSet(setName string, setType ipsets.SetType) {
	dp.ipsetMgr.CreateIPSet(setName, setType)
}

// DeleteSet checks for members and references of the given "set" type ipset
// if not used then will delete it from cache
func (dp *DataPlane) DeleteIPSet(name string) {
	dp.ipsetMgr.DeleteIPSet(name)
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

// ShouldUpdatePod will let controller know if its needs to aggregate pod data for update pod call.
func (dp *DataPlane) ShouldUpdatePod() bool {
	return dp.shouldUpdatePod()
}

// UpdatePod is to be called by pod_controller ONLY when a new pod is CREATED.
func (dp *DataPlane) UpdatePod(pod *UpdateNPMPod) error {
	// TODO check pod is in this Node if yes continue
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
func (dp *DataPlane) AddPolicy(policy *policies.NPMNetworkPolicy) error {
	klog.Infof("[DataPlane] Add Policy called for %s", policy.Name)
	// Create and add references for Selector IPSets first
	err := dp.addIPSetReferences(policy.PodSelectorIPSets, policy.Name, ipsets.SelectorType)
	if err != nil {
		klog.Infof("[DataPlane] error while adding Selector IPSet references: %s", err.Error())
		return fmt.Errorf("[DataPlane] error while adding Selector IPSet references: %w", err)
	}

	// Create and add references for Rule IPSets
	err = dp.addIPSetReferences(policy.RuleIPSets, policy.Name, ipsets.NetPolType)
	if err != nil {
		klog.Infof("[DataPlane] error while adding Rule IPSet references: %s", err.Error())
		return fmt.Errorf("[DataPlane] error while adding Rule IPSet references: %w", err)
	}

	err = dp.ApplyDataPlane()
	if err != nil {
		return fmt.Errorf("[DataPlane] error while applying dataplane: %w", err)
	}
	// TODO calculate endpoints to apply policy on
	endpointList, err := dp.getEndpointsToApplyPolicy(policy)
	if err != nil {
		return err
	}

	policy.PodEndpoints = endpointList
	err = dp.policyMgr.AddPolicy(policy, nil)
	if err != nil {
		return fmt.Errorf("[DataPlane] error while adding policy: %w", err)
	}
	return nil
}

// RemovePolicy takes in network policy name and removes it from dataplane and cache
func (dp *DataPlane) RemovePolicy(policyName string) error {
	klog.Infof("[DataPlane] Remove Policy called for %s", policyName)
	// because policy Manager will remove from policy from cache
	// keep a local copy to remove references for ipsets
	policy, ok := dp.policyMgr.GetPolicy(policyName)
	if !ok {
		klog.Infof("[DataPlane] Policy %s is not found. Might been deleted already", policyName)
		return nil
	}
	// Use the endpoint list saved in cache for this network policy to remove
	err := dp.policyMgr.RemovePolicy(policy.Name, nil)
	if err != nil {
		return fmt.Errorf("[DataPlane] error while removing policy: %w", err)
	}
	// Remove references for Rule IPSets first
	err = dp.deleteIPSetReferences(policy.RuleIPSets, policy.Name, ipsets.NetPolType)
	if err != nil {
		return err
	}

	// Remove references for Selector IPSets
	err = dp.deleteIPSetReferences(policy.PodSelectorIPSets, policy.Name, ipsets.SelectorType)
	if err != nil {
		return err
	}

	err = dp.ApplyDataPlane()
	if err != nil {
		return fmt.Errorf("[DataPlane] error while applying dataplane: %w", err)
	}

	return nil
}

// UpdatePolicy takes in updated policy object, calculates the delta and applies changes
// onto dataplane accordingly
func (dp *DataPlane) UpdatePolicy(policy *policies.NPMNetworkPolicy) error {
	klog.Infof("[DataPlane] Update Policy called for %s", policy.Name)
	ok := dp.policyMgr.PolicyExists(policy.Name)
	if !ok {
		klog.Infof("[DataPlane] Policy %s is not found. Might been deleted already", policy.Name)
		return dp.AddPolicy(policy)
	}

	// TODO it would be ideal to calculate a diff of policies
	// and remove/apply only the delta of IPSets and policies

	// Taking the easy route here, delete existing policy
	err := dp.policyMgr.RemovePolicy(policy.Name, nil)
	if err != nil {
		return fmt.Errorf("[DataPlane] error while updating policy: %w", err)
	}
	// and add the new updated policy
	err = dp.policyMgr.AddPolicy(policy, nil)
	if err != nil {
		return fmt.Errorf("[DataPlane] error while updating policy: %w", err)
	}
	return nil
}

func (dp *DataPlane) addIPSetReferences(sets map[string]*ipsets.IPSet, netpolName string, referenceType ipsets.ReferenceType) error {
	// Create IPSets first along with reference updates
	for _, set := range sets {
		dp.ipsetMgr.CreateIPSet(set.Name, set.Type)
		err := dp.ipsetMgr.AddReference(set.Name, netpolName, referenceType)
		if err != nil {
			npmErrorString := npmerrors.AddSelectorReference
			if referenceType == ipsets.NetPolType {
				npmErrorString = npmerrors.AddNetPolReference
			}
			return npmerrors.Errorf(npmErrorString, false, fmt.Sprintf("[dataplane] failed to add reference with err: %s", err.Error()))
		}
	}

	// TODO is there a possibility for a list set of selector referencing rule ipset?
	// if so this below addition would throw an error because rule ipsets are not created
	// Check if any list sets are provided with members to add
	for _, set := range sets {
		// Check if any 2nd level IPSets are generated by Controller with members
		// Apply members to the list set
		if set.Kind == ipsets.ListSet {
			if len(set.MemberIPSets) > 0 {
				memberList := []string{}
				for _, memberSet := range set.MemberIPSets {
					memberList = append(memberList, memberSet.Name)
				}
				err := dp.ipsetMgr.AddToList(set.Name, memberList)
				if err != nil {
					npmErrorString := npmerrors.AddSelectorReference
					if referenceType == ipsets.NetPolType {
						npmErrorString = npmerrors.AddNetPolReference
					}
					return npmerrors.Errorf(npmErrorString, false, fmt.Sprintf("[dataplane] failed to AddToList in addIPSetReferences with err: %s", err.Error()))

				}
			}
		}
	}

	return nil
}

func (dp *DataPlane) deleteIPSetReferences(sets map[string]*ipsets.IPSet, netpolName string, referenceType ipsets.ReferenceType) error {
	for _, set := range sets {
		// TODO ignore set does not exist error
		// TODO add delete ipset after removing members
		err := dp.ipsetMgr.DeleteReference(set.Name, netpolName, referenceType)
		if err != nil {
			npmErrorString := npmerrors.DeleteSelectorReference
			if referenceType == ipsets.NetPolType {
				npmErrorString = npmerrors.DeleteNetPolReference
			}
			return npmerrors.Errorf(npmErrorString, false, fmt.Sprintf("[dataplane] failed to deleteIPSetReferences with err: %s", err.Error()))
		}
	}

	// Check if any list sets are provided with members to add
	// TODO for nested IPsets check if we are safe to remove members
	// if k1:v0:v1 is created by two network policies
	// and both have same members
	// then we should not delete k1:v0:v1 members ( special case for nested ipsets )
	for _, set := range sets {
		// Delete if any 2nd level IPSets are generated by Controller with members
		if set.Kind == ipsets.ListSet {
			if len(set.MemberIPSets) > 0 {
				memberList := []string{}
				for _, memberSet := range set.MemberIPSets {
					memberList = append(memberList, memberSet.Name)
				}
				err := dp.ipsetMgr.RemoveFromList(set.Name, memberList)
				if err != nil {
					npmErrorString := npmerrors.DeleteSelectorReference
					if referenceType == ipsets.NetPolType {
						npmErrorString = npmerrors.DeleteNetPolReference
					}
					return npmerrors.Errorf(npmErrorString, false, fmt.Sprintf("[dataplane] failed to RemoveFromList in deleteIPSetReferences with err: %s", err.Error()))
				}
			}
		}

		// Try to delete these IPSets
		dp.ipsetMgr.DeleteIPSet(set.Name)
	}
	return nil
}
