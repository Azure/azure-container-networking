package ipsets

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/Azure/azure-container-networking/npm/util"
	npmerrors "github.com/Azure/azure-container-networking/npm/util/errors"
	"github.com/Microsoft/hcsshim/hcn"
	"k8s.io/klog"
)

const (
	// SetPolicyTypeNestedIPSet as a temporary measure adding it here
	// update HCSShim to version 0.9.23 or above to support nestedIPSets
	SetPolicyTypeNestedIPSet hcn.SetPolicyType = "NESTEDIPSET"
	resetIPSetsTrue                            = true
	donotResetIPSets                           = false
)

var errUnsupportedNetwork = errors.New("only 'azure' network is supported")

type networkPolicyBuilder struct {
	toAddSets    map[string]*hcn.SetPolicySetting
	toUpdateSets map[string]*hcn.SetPolicySetting
	toDeleteSets map[string]*hcn.SetPolicySetting
}

func (iMgr *IPSetManager) DoesIPSatisfySelectorIPSets(ip string, setList map[string]struct{}) (bool, error) {
	if len(setList) == 0 {
		klog.Infof("[ipset manager] unexpectedly encountered empty selector list")
		return true, nil
	}
	iMgr.Lock()
	defer iMgr.Unlock()

	if err := iMgr.validateSelectorIPSets(setList); err != nil {
		return false, err
	}

	for setName := range setList {
		set := iMgr.setMap[setName]
		if !set.isIPAffiliated(ip) {
			return false, nil
		}
	}

	return true, nil
}

// GetIPsFromSelectorIPSets will take in a map of prefixedSetNames and return an intersection of IPs
func (iMgr *IPSetManager) GetIPsFromSelectorIPSets(setList map[string]struct{}) (map[string]string, error) {
	ips := make(map[string]string)
	if len(setList) == 0 {
		return ips, nil
	}
	iMgr.Lock()
	defer iMgr.Unlock()

	if err := iMgr.validateSelectorIPSets(setList); err != nil {
		return nil, err
	}

	// the following is a space/time optimized way to get the intersection of IPs from the selector sets
	// we should always take the hash set branch because a pod selector always includes a namespace ipset,
	// which is a hash set, and we favor hash sets for firstSet
	var firstSet *IPSet
	for setName := range setList {
		set := iMgr.setMap[setName]
		if set.Kind == HashSet || firstSet == nil {
			// firstSet can be any set, but ideally is a hash set for efficiency (compare the branch for hash sets to the one for lists below)
			firstSet = set
		}
	}
	if firstSet.Kind == HashSet {
		// include every IP in firstSet that is also affiliated with every other selector set
		for ip, podKey := range firstSet.IPPodKey {
			isAffiliated := true
			for otherSetName := range setList {
				if otherSetName == firstSet.Name {
					continue
				}
				otherSet := iMgr.setMap[otherSetName]
				if !otherSet.isIPAffiliated(ip) {
					isAffiliated = false
					break
				}
			}

			if isAffiliated {
				ips[ip] = podKey
			}
		}
	} else {
		// should never reach this branch (see note above)
		// include every IP affiliated with firstSet that is also affiliated with every other selector set
		// identical to the hash set case, except we have to make space for all IPs affiliated with firstSet

		// only loop over the unique affiliated IPs
		for _, memberSet := range firstSet.MemberIPSets {
			for ip, podKey := range memberSet.IPPodKey {
				ips[ip] = podKey
			}
		}
		for ip := range ips {
			// identical to the hash set case
			isAffiliated := true
			for otherSetName := range setList {
				if otherSetName == firstSet.Name {
					continue
				}
				otherSet := iMgr.setMap[otherSetName]
				if !otherSet.isIPAffiliated(ip) {
					isAffiliated = false
					break
				}
			}

			if !isAffiliated {
				delete(ips, ip)
			}
		}
	}
	return ips, nil
}

func (iMgr *IPSetManager) GetSelectorReferencesBySet(setName string) (map[string]struct{}, error) {
	iMgr.Lock()
	defer iMgr.Unlock()
	if !iMgr.exists(setName) {
		return nil, npmerrors.Errorf(
			npmerrors.GetSelectorReference,
			false,
			fmt.Sprintf("[ipset manager] selector ipset %s does not exist", setName))
	}
	set := iMgr.setMap[setName]
	m := make(map[string]struct{}, len(set.SelectorReference))
	for r := range set.SelectorReference {
		m[r] = struct{}{}
	}
	return m, nil
}

func (iMgr *IPSetManager) validateSelectorIPSets(setList map[string]struct{}) error {
	for setName := range setList {
		if !iMgr.exists(setName) {
			return npmerrors.Errorf(
				npmerrors.GetSelectorReference,
				false,
				fmt.Sprintf("[ipset manager] selector ipset %s does not exist", setName))
		}
		set := iMgr.setMap[setName]
		if !set.canSetBeSelectorIPSet() {
			return npmerrors.Errorf(
				npmerrors.IPSetIntersection,
				false,
				fmt.Sprintf("[IPSet] Selector IPSet cannot be of type %s", set.Type.String()))
		}
	}
	return nil
}

func (iMgr *IPSetManager) resetIPSets() error {
	klog.Infof("[IPSetManager Windows] Resetting Dataplane")
	network, err := iMgr.getHCnNetwork()
	if err != nil {
		return err
	}

	_, toDeleteSets := iMgr.segregateSetPolicies(network.Policies, resetIPSetsTrue)

	if len(toDeleteSets) == 0 {
		klog.Infof("[IPSetManager Windows] No IPSets to delete")
		return nil
	}

	klog.Infof("[IPSetManager Windows] Deleting %d Set Policies", len(toDeleteSets))
	err = iMgr.modifySetPolicies(network, hcn.RequestTypeRemove, toDeleteSets)
	if err != nil {
		klog.Infof("[IPSetManager Windows] Update set policies failed with error %s", err.Error())
		return err
	}

	return nil
}

func (iMgr *IPSetManager) applyIPSets() error {
	network, err := iMgr.getHCnNetwork()
	if err != nil {
		return err
	}

	setPolicyBuilder, err := iMgr.calculateNewSetPolicies(network.Policies)
	if err != nil {
		return err
	}

	if len(setPolicyBuilder.toAddSets) > 0 {
		err = iMgr.modifySetPolicies(network, hcn.RequestTypeAdd, setPolicyBuilder.toAddSets)
		if err != nil {
			klog.Infof("[IPSetManager Windows] Add set policies failed with error %s", err.Error())
			return err
		}
	}

	if len(setPolicyBuilder.toUpdateSets) > 0 {
		err = iMgr.modifySetPolicies(network, hcn.RequestTypeUpdate, setPolicyBuilder.toUpdateSets)
		if err != nil {
			klog.Infof("[IPSetManager Windows] Update set policies failed with error %s", err.Error())
			return err
		}
	}

	iMgr.dirtyCache.resetAddOrUpdateCache()

	if len(setPolicyBuilder.toDeleteSets) > 0 {
		err = iMgr.modifySetPolicies(network, hcn.RequestTypeRemove, setPolicyBuilder.toDeleteSets)
		if err != nil {
			klog.Infof("[IPSetManager Windows] Delete set policies failed with error %s", err.Error())
			return err
		}
	}

	klog.Info("[IPSetManager Windows] Done applying IPSets.")

	iMgr.clearDirtyCache()

	return nil
}

// calculateNewSetPolicies will take in existing setPolicies on network in HNS and the dirty cache, will return back
// networkPolicyBuild which contains the new setPolicies to be added, updated and deleted
// Assumes that the dirty cache is locked (or equivalently, the ipsetmanager itself).
// toAddSets:
//      this function will loop through the dirty cache and adds non-existing sets to toAddSets
// toUpdateSets:
//      this function will loop through the dirty cache and adds existing sets in HNS to toUpdateSets
//      this function will update all existing sets in HNS with their latest goal state irrespective of any change to the object
// toDeleteSets:
//      this function will loop through the dirty delete cache and adds existing set obj in HNS to toDeleteSets
func (iMgr *IPSetManager) calculateNewSetPolicies(networkPolicies []hcn.NetworkPolicy) (*networkPolicyBuilder, error) {
	setPolicyBuilder := &networkPolicyBuilder{
		toAddSets:    map[string]*hcn.SetPolicySetting{},
		toUpdateSets: map[string]*hcn.SetPolicySetting{},
		toDeleteSets: map[string]*hcn.SetPolicySetting{},
	}
	existingSets, toDeleteSets := iMgr.segregateSetPolicies(networkPolicies, donotResetIPSets)
	// some of this below logic can be abstracted a step above
	toAddUpdateSetNames := iMgr.dirtyCache.setsToAddOrUpdate()
	setPolicyBuilder.toDeleteSets = toDeleteSets

	// for faster look up changing a slice to map
	existingSetNames := make(map[string]struct{})
	for _, setName := range existingSets {
		existingSetNames[setName] = struct{}{}
	}

	for setName := range toAddUpdateSetNames {
		set, exists := iMgr.setMap[setName] // check if the Set exists
		if !exists {
			return nil, npmerrors.Errorf(npmerrors.AppendIPSet, false, fmt.Sprintf("ipset %s does not exist", setName))
		}

		setPol, err := convertToSetPolicy(set)
		if err != nil {
			return nil, err
		}
		// TODO we should add members first and then the Lists
		_, ok := existingSetNames[setName]
		if ok {
			setPolicyBuilder.toUpdateSets[setName] = setPol
		} else {
			setPolicyBuilder.toAddSets[setName] = setPol
		}
		if set.Kind == ListSet {
			for _, memberSet := range set.MemberIPSets {
				// Always use prefixed name because we read setpolicy Name from HNS
				if setPolicyBuilder.setNameExists(memberSet.Name) {
					continue
				}
				setPol, err = convertToSetPolicy(memberSet)
				if err != nil {
					return nil, err
				}
				_, ok := existingSetNames[memberSet.Name]
				if !ok {
					setPolicyBuilder.toAddSets[memberSet.Name] = setPol
				}
			}
		}
	}

	return setPolicyBuilder, nil
}

func (iMgr *IPSetManager) getHCnNetwork() (*hcn.HostComputeNetwork, error) {
	if iMgr.iMgrCfg.NetworkName == "" {
		iMgr.iMgrCfg.NetworkName = util.AzureNetworkName
	}
	if iMgr.iMgrCfg.NetworkName != util.AzureNetworkName {
		return nil, errUnsupportedNetwork
	}
	network, err := iMgr.ioShim.Hns.GetNetworkByName(iMgr.iMgrCfg.NetworkName)
	if err != nil {
		return nil, err
	}
	return network, nil
}

func (iMgr *IPSetManager) modifySetPolicies(network *hcn.HostComputeNetwork, operation hcn.RequestType, setPolicies map[string]*hcn.SetPolicySetting) error {
	klog.Infof("[IPSetManager Windows] %s operation on set policies is called", operation)
	/*
		Due to complexities in HNS, we need to do the following:
		for (Add)
			1. Add 1st level set policies to HNS
			2. then add nested set policies to HNS

		for (delete)
			1. delete nested set policies from HNS
			2. then delete 1st level set policies from HNS
	*/
	policySettingsOrder := []hcn.SetPolicyType{hcn.SetPolicyTypeIpSet, SetPolicyTypeNestedIPSet}
	if operation == hcn.RequestTypeRemove {
		policySettingsOrder = []hcn.SetPolicyType{SetPolicyTypeNestedIPSet, hcn.SetPolicyTypeIpSet}
	}
	for _, policyType := range policySettingsOrder {
		policyRequest, err := getPolicyNetworkRequestMarshal(setPolicies, policyType)
		if err != nil {
			klog.Infof("[IPSetManager Windows] Failed to marshal %s operations sets with error %s", operation, err.Error())
			return err
		}

		if policyRequest == nil {
			continue
		}

		requestMessage := &hcn.ModifyNetworkSettingRequest{
			ResourceType: hcn.NetworkResourceTypePolicy,
			RequestType:  operation,
			Settings:     policyRequest,
		}

		klog.Infof("[IPSetManager Windows] modifying network settings. operation: %s, policyType: %s", operation, policyType)
		err = iMgr.ioShim.Hns.ModifyNetworkSettings(network, requestMessage)
		if err != nil {
			klog.Infof("[IPSetManager Windows] %s operation has failed with error %s", operation, err.Error())
			return err
		}
	}
	return nil
}

func (iMgr *IPSetManager) segregateSetPolicies(networkPolicies []hcn.NetworkPolicy, reset bool) (toUpdateSets []string, toDeleteSets map[string]*hcn.SetPolicySetting) {
	toDeleteSets = make(map[string]*hcn.SetPolicySetting)
	toUpdateSets = make([]string, 0)
	for _, netpol := range networkPolicies {
		if netpol.Type != hcn.SetPolicy {
			continue
		}
		var set hcn.SetPolicySetting
		err := json.Unmarshal(netpol.Settings, &set)
		if err != nil {
			klog.Error(err.Error())
			continue
		}
		if !strings.HasPrefix(set.Id, util.AzureNpmPrefix) {
			continue
		}
		ok := iMgr.dirtyCache.isSetToDelete(set.Name)
		if !ok && !reset {
			// if the set is not in delete cache, go ahead and add it to update cache
			toUpdateSets = append(toUpdateSets, set.Name)
			continue
		}
		// if set is in delete cache, add it to deleteSets
		toDeleteSets[set.Name] = &set
	}
	return
}

func (setPolicyBuilder *networkPolicyBuilder) setNameExists(setName string) bool {
	_, ok := setPolicyBuilder.toAddSets[setName]
	if ok {
		return true
	}
	_, ok = setPolicyBuilder.toUpdateSets[setName]
	return ok
}

func getPolicyNetworkRequestMarshal(setPolicySettings map[string]*hcn.SetPolicySetting, policyType hcn.SetPolicyType) ([]byte, error) {
	if len(setPolicySettings) == 0 {
		klog.Info("[Dataplane Windows] no set policies to apply on network")
		return nil, nil
	}
	klog.Infof("[Dataplane Windows] marshalling %s(s)", policyType)
	policyNetworkRequest := &hcn.PolicyNetworkRequest{
		Policies: make([]hcn.NetworkPolicy, 0),
	}

	for _, setPol := range setPolicySettings {
		if setPol.PolicyType != policyType {
			continue
		}
		rawSettings, err := json.Marshal(setPol)
		if err != nil {
			return nil, err
		}
		policyNetworkRequest.Policies = append(
			policyNetworkRequest.Policies,
			hcn.NetworkPolicy{
				Type:     hcn.SetPolicy,
				Settings: rawSettings,
			},
		)
	}

	if len(policyNetworkRequest.Policies) == 0 {
		klog.Infof("[Dataplane Windows] no %s type of sets to apply", policyType)
		return nil, nil
	}

	policyReqSettings, err := json.Marshal(policyNetworkRequest)
	if err != nil {
		return nil, err
	}
	return policyReqSettings, nil
}

func isValidIPSet(set *IPSet) error {
	if set.Name == "" {
		return fmt.Errorf("IPSet " + set.Name + " is missing Name")
	}

	if set.Type == UnknownType {
		return fmt.Errorf("IPSet " + set.Type.String() + " is missing Type")
	}

	if set.HashedName == "" {
		return fmt.Errorf("IPSet " + set.HashedName + " is missing HashedName")
	}

	return nil
}

func getSetPolicyType(set *IPSet) hcn.SetPolicyType {
	switch set.Kind {
	case ListSet:
		return SetPolicyTypeNestedIPSet
	case HashSet:
		return hcn.SetPolicyTypeIpSet
	default:
		return "Unknown"
	}
}

func convertToSetPolicy(set *IPSet) (*hcn.SetPolicySetting, error) {
	err := isValidIPSet(set)
	if err != nil {
		return &hcn.SetPolicySetting{}, err
	}

	setContents, err := set.GetSetContents()
	if err != nil {
		return &hcn.SetPolicySetting{}, err
	}

	setPolicy := &hcn.SetPolicySetting{
		Id:         set.HashedName,
		Name:       set.Name,
		PolicyType: getSetPolicyType(set),
		Values:     util.SliceToString(setContents),
	}
	return setPolicy, nil
}
