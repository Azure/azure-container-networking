package ipsets

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/Azure/azure-container-networking/npm/util"
	"github.com/Azure/azure-container-networking/npm/util/errors"
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

type networkPolicyBuilder struct {
	toAddSets    map[string]*hcn.SetPolicySetting
	toUpdateSets map[string]*hcn.SetPolicySetting
	toDeleteSets map[string]*hcn.SetPolicySetting
}

func (iMgr *IPSetManager) resetIPSets() error {
	klog.Infof("[IPSetManager Windows] Resetting Dataplane")
	network, err := iMgr.getHCnNetwork()
	if err != nil {
		return err
	}

	// TODO delete 2nd level sets first and then 1st level sets
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

	iMgr.toAddOrUpdateCache = make(map[string]struct{})

	if len(setPolicyBuilder.toDeleteSets) > 0 {
		err = iMgr.modifySetPolicies(network, hcn.RequestTypeRemove, setPolicyBuilder.toDeleteSets)
		if err != nil {
			klog.Infof("[IPSetManager Windows] Delete set policies failed with error %s", err.Error())
			return err
		}
	}

	iMgr.clearDirtyCache()

	return nil
}

// calculateNewSetPolicies will take in existing setPolicies on network in HNS and the dirty cache, will return back
// networkPolicyBuild which contains the new setPolicies to be added, updated and deleted
// TODO: This function is not thread safe.
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
	toAddUpdateSetNames := iMgr.toAddOrUpdateCache
	setPolicyBuilder.toDeleteSets = toDeleteSets

	// for faster look up changing a slice to map
	existingSetNames := make(map[string]struct{})
	for _, setName := range existingSets {
		existingSetNames[setName] = struct{}{}
	}
	// (TODO) remove this log line later
	klog.Infof("toAddUpdateSetNames %+v \n ", toAddUpdateSetNames)
	klog.Infof("existingSetNames %+v \n ", existingSetNames)
	for setName := range toAddUpdateSetNames {
		set, exists := iMgr.setMap[setName] // check if the Set exists
		if !exists {
			return nil, errors.Errorf(errors.AppendIPSet, false, fmt.Sprintf("ipset %s does not exist", setName))
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
		iMgr.iMgrCfg.NetworkName = "azure"
	}
	network, err := iMgr.ioShim.Hns.GetNetworkByName("azure")
	if err != nil {
		return nil, err
	}
	return network, nil
}

func (iMgr *IPSetManager) modifySetPolicies(network *hcn.HostComputeNetwork, operation hcn.RequestType, setPolicies map[string]*hcn.SetPolicySetting) error {
	klog.Infof("[IPSetManager Windows] %s operation on set policies is called", operation)
	policyRequest, err := getPolicyNetworkRequestMarshal(setPolicies, operation)
	if err != nil {
		klog.Infof("[IPSetManager Windows] Failed to marshal %s operations sets with error %s", operation, err.Error())
		return err
	}

	if policyRequest == nil {
		klog.Infof("[IPSetManager Windows] No Policies to apply")
		return nil
	}

	requestMessage := &hcn.ModifyNetworkSettingRequest{
		ResourceType: hcn.NetworkResourceTypePolicy,
		RequestType:  operation,
		Settings:     policyRequest,
	}

	err = iMgr.ioShim.Hns.ModifyNetworkSettings(network, requestMessage)
	if err != nil {
		klog.Infof("[IPSetManager Windows] %s operation has failed with error %s", operation, err.Error())
		return err
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
		_, ok := iMgr.toDeleteCache[set.Name]
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

func getPolicyNetworkRequestMarshal(setPolicySettings map[string]*hcn.SetPolicySetting, operation hcn.RequestType) ([]byte, error) {
	policyNetworkRequest := &hcn.PolicyNetworkRequest{
		Policies: make([]hcn.NetworkPolicy, len(setPolicySettings)),
	}

	if len(setPolicySettings) == 0 {
		klog.Info("[Dataplane Windows] no set policies to apply on network")
		return nil, nil
	}

	idx := 0
	policySettingsOrder := []hcn.SetPolicyType{SetPolicyTypeNestedIPSet, hcn.SetPolicyTypeIpSet}
	if operation == hcn.RequestTypeRemove {
		policySettingsOrder = []hcn.SetPolicyType{hcn.SetPolicyTypeIpSet, SetPolicyTypeNestedIPSet}
	}
	for _, policyType := range policySettingsOrder {
		for setPol := range setPolicySettings {
			if setPolicySettings[setPol].PolicyType != policyType {
				continue
			}
			klog.Infof("Found set pol %+v", setPolicySettings[setPol])
			rawSettings, err := json.Marshal(setPolicySettings[setPol])
			if err != nil {
				return nil, err
			}
			policyNetworkRequest.Policies[idx] = hcn.NetworkPolicy{
				Type:     hcn.SetPolicy,
				Settings: rawSettings,
			}
			idx++
		}
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
