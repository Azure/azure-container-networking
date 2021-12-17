package dataplane

import (
	"fmt"
	"net"
	"strings"

	"github.com/Azure/azure-container-networking/common"
	"github.com/Azure/azure-container-networking/npm/metrics"
	"github.com/Azure/azure-container-networking/npm/pkg/dataplane/ipsets"
	"github.com/Azure/azure-container-networking/npm/pkg/dataplane/policies"
	npmerrors "github.com/Azure/azure-container-networking/npm/util/errors"
	"k8s.io/klog"
)

const (
	// AzureNetworkName is default network Azure CNI creates
	AzureNetworkName = "azure"
)

type PolicyMode string

// TODO put NodeName in Config?
type Config struct {
	*ipsets.IPSetManagerCfg
	*policies.PolicyManagerCfg
}

type DataPlane struct {
	policyMgr *policies.PolicyManager
	ipsetMgr  *ipsets.IPSetManager
	networkID string
	nodeName  string
	// Key is PodIP
	endpointCache  map[string]*NPMEndpoint
	ioShim         *common.IOShim
	updatePodCache map[string]*updateNPMPod
	*Config
}

type NPMEndpoint struct {
	Name string
	ID   string
	IP   string
	// TODO: check it may use PolicyKey instead of Policy name
	// Map with Key as Network Policy name to to emulate set
	// and value as struct{} for minimal memory consumption
	NetPolReference map[string]struct{}
}

func NewDataPlane(nodeName string, ioShim *common.IOShim, cfg *Config) (*DataPlane, error) {
	metrics.InitializeAll()
	dp := &DataPlane{
		policyMgr:      policies.NewPolicyManager(ioShim, cfg.PolicyManagerCfg),
		ipsetMgr:       ipsets.NewIPSetManager(cfg.IPSetManagerCfg, ioShim),
		endpointCache:  make(map[string]*NPMEndpoint),
		nodeName:       nodeName,
		ioShim:         ioShim,
		updatePodCache: make(map[string]*updateNPMPod),
		Config:         cfg,
	}

	err := dp.ResetDataPlane()
	if err != nil {
		klog.Errorf("Failed to reset dataplane: %v", err)
		return nil, err
	}
	return dp, nil
}

// InitializeDataPlane helps in setting up dataplane for NPM
func (dp *DataPlane) InitializeDataPlane() error {
	// TODO deprecate this function
	return nil
}

// ResetDataPlane helps in cleaning up dataplane sets and policies programmed
// by NPM, returning a clean slate
func (dp *DataPlane) ResetDataPlane() error {
	// TODO rename this function to BootupDataplane
	// NOTE: used to create an all-namespaces set, but there's no need since it will be created by the control plane
	return dp.bootupDataPlane()
}

// CreateIPSets takes in a set object and updates local cache with this set
func (dp *DataPlane) CreateIPSets(setMetadata []*ipsets.IPSetMetadata) {
	dp.ipsetMgr.CreateIPSets(setMetadata)
}

// DeleteSet checks for members and references of the given "set" type ipset
// if not used then will delete it from cache
func (dp *DataPlane) DeleteIPSet(setMetadata *ipsets.IPSetMetadata) {
	dp.ipsetMgr.DeleteIPSet(setMetadata.GetPrefixName())
}

// AddToSets takes in a list of IPSet names along with IP member
// and then updates it local cache
func (dp *DataPlane) AddToSets(setNames []*ipsets.IPSetMetadata, podMetadata *PodMetadata) error {
	err := dp.ipsetMgr.AddToSets(setNames, podMetadata.PodIP, podMetadata.PodKey)
	if err != nil {
		return fmt.Errorf("[DataPlane] error while adding to set: %w", err)
	}
	if dp.shouldUpdatePod() {
		klog.Infof("[Dataplane] Updating Sets to Add for pod key %s", podMetadata.PodKey)
		if _, ok := dp.updatePodCache[podMetadata.PodKey]; !ok {
			klog.Infof("[Dataplane] {AddToSet} pod key %s not found creating a new obj", podMetadata.PodKey)
			dp.updatePodCache[podMetadata.PodKey] = newUpdateNPMPod(podMetadata)
		}

		dp.updatePodCache[podMetadata.PodKey].updateIPSetsToAdd(setNames)
	}

	return nil
}

// RemoveFromSets takes in list of setnames from which a given IP member should be
// removed and will update the local cache
func (dp *DataPlane) RemoveFromSets(setNames []*ipsets.IPSetMetadata, podMetadata *PodMetadata) error {
	err := dp.ipsetMgr.RemoveFromSets(setNames, podMetadata.PodIP, podMetadata.PodKey)
	if err != nil {
		return fmt.Errorf("[DataPlane] error while removing from set: %w", err)
	}

	if dp.shouldUpdatePod() {
		klog.Infof("[Dataplane] Updating Sets to Remove for pod key %s", podMetadata.PodKey)
		if _, ok := dp.updatePodCache[podMetadata.PodKey]; !ok {
			klog.Infof("[Dataplane] {RemoveFromSet} pod key %s not found creating a new obj", podMetadata.PodKey)
			dp.updatePodCache[podMetadata.PodKey] = newUpdateNPMPod(podMetadata)
		}

		dp.updatePodCache[podMetadata.PodKey].updateIPSetsToRemove(setNames)
	}

	return nil
}

// AddToLists takes a list name and list of sets which are to be added as members
// to given list
func (dp *DataPlane) AddToLists(listName, setNames []*ipsets.IPSetMetadata) error {
	err := dp.ipsetMgr.AddToLists(listName, setNames)
	if err != nil {
		return fmt.Errorf("[DataPlane] error while adding to list: %w", err)
	}
	return nil
}

// RemoveFromList takes a list name and list of sets which are to be removed as members
// to given list
func (dp *DataPlane) RemoveFromList(listName *ipsets.IPSetMetadata, setNames []*ipsets.IPSetMetadata) error {
	err := dp.ipsetMgr.RemoveFromList(listName, setNames)
	if err != nil {
		return fmt.Errorf("[DataPlane] error while removing from list: %w", err)
	}
	return nil
}

// ApplyDataPlane all the IPSet operations just update cache and update a dirty ipset structure,
// they do not change apply changes into dataplane. This function needs to be called at the
// end of IPSet operations of a given controller event, it will check for the dirty ipset list
// and accordingly makes changes in dataplane. This function helps emulate a single call to
// dataplane instead of multiple ipset operations calls ipset operations calls to dataplane
func (dp *DataPlane) ApplyDataPlane() error {
	err := dp.ipsetMgr.ApplyIPSets()
	if err != nil {
		return fmt.Errorf("[DataPlane] error while applying IPSets: %w", err)
	}

	if dp.shouldUpdatePod() {
		for podKey, pod := range dp.updatePodCache {
			err := dp.updatePod(pod)
			if err != nil {
				return fmt.Errorf("[DataPlane] error while updating pod: %w", err)
			}
			delete(dp.updatePodCache, podKey)
		}
	}
	return nil
}

// AddPolicy takes in a translated NPMNetworkPolicy object and applies on dataplane
func (dp *DataPlane) AddPolicy(policy *policies.NPMNetworkPolicy) error {
	klog.Infof("[DataPlane] Add Policy called for %s", policy.PolicyKey)
	// Create and add references for Selector IPSets first
	err := dp.createIPSetsAndReferences(policy.PodSelectorIPSets, policy.PolicyKey, ipsets.SelectorType)
	if err != nil {
		klog.Infof("[DataPlane] error while adding Selector IPSet references: %s", err.Error())
		return fmt.Errorf("[DataPlane] error while adding Selector IPSet references: %w", err)
	}

	// Create and add references for Rule IPSets
	err = dp.createIPSetsAndReferences(policy.RuleIPSets, policy.PolicyKey, ipsets.NetPolType)
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

	err = dp.policyMgr.AddPolicy(policy, endpointList)
	if err != nil {
		return fmt.Errorf("[DataPlane] error while adding policy: %w", err)
	}
	return nil
}

// RemovePolicy takes in network policyKey (namespace/name of network policy) and removes it from dataplane and cache
func (dp *DataPlane) RemovePolicy(policyKey string) error {
	klog.Infof("[DataPlane] Remove Policy called for %s", policyKey)
	// because policy Manager will remove from policy from cache
	// keep a local copy to remove references for ipsets
	policy, ok := dp.policyMgr.GetPolicy(policyKey)
	if !ok {
		klog.Infof("[DataPlane] Policy %s is not found. Might been deleted already", policyKey)
		return nil
	}
	// Use the endpoint list saved in cache for this network policy to remove
	err := dp.policyMgr.RemovePolicy(policy.PolicyKey, nil)
	if err != nil {
		return fmt.Errorf("[DataPlane] error while removing policy: %w", err)
	}
	// Remove references for Rule IPSets first
	err = dp.deleteIPSetsAndReferences(policy.RuleIPSets, policy.PolicyKey, ipsets.NetPolType)
	if err != nil {
		return err
	}

	// Remove references for Selector IPSets
	err = dp.deleteIPSetsAndReferences(policy.PodSelectorIPSets, policy.PolicyKey, ipsets.SelectorType)
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
	klog.Infof("[DataPlane] Update Policy called for %s", policy.PolicyKey)
	ok := dp.policyMgr.PolicyExists(policy.PolicyKey)
	if !ok {
		klog.Infof("[DataPlane] Policy %s is not found. Might been deleted already", policy.PolicyKey)
		return dp.AddPolicy(policy)
	}

	// TODO it would be ideal to calculate a diff of policies
	// and remove/apply only the delta of IPSets and policies

	// Taking the easy route here, delete existing policy
	err := dp.RemovePolicy(policy.PolicyKey)
	if err != nil {
		return fmt.Errorf("[DataPlane] error while updating policy: %w", err)
	}
	// and add the new updated policy
	err = dp.AddPolicy(policy)
	if err != nil {
		return fmt.Errorf("[DataPlane] error while updating policy: %w", err)
	}
	return nil
}

func (dp *DataPlane) createIPSetsAndReferences(sets []*ipsets.TranslatedIPSet, netpolName string, referenceType ipsets.ReferenceType) error {
	// Create IPSets first along with reference updates
	npmErrorString := npmerrors.AddSelectorReference
	if referenceType == ipsets.NetPolType {
		npmErrorString = npmerrors.AddNetPolReference
	}
	for _, set := range sets {
		dp.ipsetMgr.CreateIPSets([]*ipsets.IPSetMetadata{set.Metadata})
		err := dp.ipsetMgr.AddReference(set.Metadata.GetPrefixName(), netpolName, referenceType)
		if err != nil {
			return npmerrors.Errorf(npmErrorString, false, fmt.Sprintf("[dataplane] failed to add reference with err: %s", err.Error()))
		}
	}

	// TODO is there a possibility for a list set of selector referencing rule ipset?
	// if so this below addition would throw an error because rule ipsets are not created
	// Check if any list sets are provided with members to add
	for _, set := range sets {
		// Check if any CIDR block IPSets needs to be applied
		setType := set.Metadata.Type
		if setType == ipsets.CIDRBlocks {
			// ipblock can have either cidr (CIDR in IPBlock) or "cidr + " " (space) + nomatch" (Except in IPBlock)
			// (TODO) need to revise it for windows
			for _, ipblock := range set.Members {
				err := validateIPBlock(ipblock)
				if err != nil {
					return npmerrors.Errorf(npmErrorString, false, fmt.Sprintf("[dataplane] failed to parseCIDR in addIPSetReferences with err: %s", err.Error()))
				}
				err = dp.ipsetMgr.AddToSets([]*ipsets.IPSetMetadata{set.Metadata}, ipblock, "")
				if err != nil {
					return npmerrors.Errorf(npmErrorString, false, fmt.Sprintf("[dataplane] failed to AddToSet in addIPSetReferences with err: %s", err.Error()))
				}
			}
		} else if setType == ipsets.NestedLabelOfPod && len(set.Members) > 0 {
			// Check if any 2nd level IPSets are generated by Controller with members
			// Apply members to the list set
			err := dp.ipsetMgr.AddToLists([]*ipsets.IPSetMetadata{set.Metadata}, getMembersOfTranslatedSets(set.Members))
			if err != nil {
				return npmerrors.Errorf(npmErrorString, false, fmt.Sprintf("[dataplane] failed to AddToList in addIPSetReferences with err: %s", err.Error()))
			}
		}
	}

	return nil
}

func (dp *DataPlane) deleteIPSetsAndReferences(sets []*ipsets.TranslatedIPSet, netpolName string, referenceType ipsets.ReferenceType) error {
	npmErrorString := npmerrors.DeleteSelectorReference
	if referenceType == ipsets.NetPolType {
		npmErrorString = npmerrors.DeleteNetPolReference
	}
	for _, set := range sets {
		// TODO ignore set does not exist error
		// TODO add delete ipset after removing members
		err := dp.ipsetMgr.DeleteReference(set.Metadata.GetPrefixName(), netpolName, referenceType)
		if err != nil {
			return npmerrors.Errorf(npmErrorString, false, fmt.Sprintf("[dataplane] failed to deleteIPSetReferences with err: %s", err.Error()))
		}
	}

	// Check if any list sets are provided with members to add
	// TODO for nested IPsets check if we are safe to remove members
	// if k1:v0:v1 is created by two network policies
	// and both have same members
	// then we should not delete k1:v0:v1 members ( special case for nested ipsets )
	for _, set := range sets {
		// Check if any CIDR block IPSets needs to be applied
		setType := set.Metadata.Type
		if setType == ipsets.CIDRBlocks {
			// ipblock can have either cidr (CIDR in IPBlock) or "cidr + " " (space) + nomatch" (Except in IPBlock)
			// (TODO) need to revise it for windows
			for _, ipblock := range set.Members {
				err := validateIPBlock(ipblock)
				if err != nil {
					return npmerrors.Errorf(npmErrorString, false, fmt.Sprintf("[dataplane] failed to parseCIDR in deleteIPSetReferences with err: %s", err.Error()))
				}
				err = dp.ipsetMgr.RemoveFromSets([]*ipsets.IPSetMetadata{set.Metadata}, ipblock, "")
				if err != nil {
					return npmerrors.Errorf(npmErrorString, false, fmt.Sprintf("[dataplane] failed to RemoveFromSet in deleteIPSetReferences with err: %s", err.Error()))
				}
			}
		} else if set.Metadata.GetSetKind() == ipsets.ListSet && len(set.Members) > 0 {
			// Delete if any 2nd level IPSets are generated by Controller with members
			err := dp.ipsetMgr.RemoveFromList(set.Metadata, getMembersOfTranslatedSets(set.Members))
			if err != nil {
				return npmerrors.Errorf(npmErrorString, false, fmt.Sprintf("[dataplane] failed to RemoveFromList in deleteIPSetReferences with err: %s", err.Error()))
			}

		}

		// Try to delete these IPSets
		dp.ipsetMgr.DeleteIPSet(set.Metadata.GetPrefixName())
	}
	return nil
}

// TODO: This is an adhoc approach for linux, but need to refactor data structure for better management.
func validateIPBlock(ipblock string) error {
	// TODO: This is fragile code with strong dependency with " "(space).
	// onlyCidr has only cidr without "space" and "nomatch" in case except ipblock to validate cidr format.
	onlyCidr := strings.Split(ipblock, " ")[0]
	_, _, err := net.ParseCIDR(onlyCidr)
	if err != nil {
		return npmerrors.SimpleErrorWrapper("failed to parse CIDR", err)
	}
	return nil
}

func getMembersOfTranslatedSets(members []string) []*ipsets.IPSetMetadata {
	memberList := make([]*ipsets.IPSetMetadata, len(members))
	i := 0
	for _, setName := range members {
		// translate engine only returns KeyValueLabelOfPod as member
		memberSet := ipsets.NewIPSetMetadata(setName, ipsets.KeyValueLabelOfPod)
		memberList[i] = memberSet
		i++
	}
	return memberList
}
