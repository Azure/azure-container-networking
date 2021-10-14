package ipsets

import (
	"encoding/json"
	"fmt"

	"github.com/Azure/azure-container-networking/npm/util"
	"github.com/Azure/azure-container-networking/npm/util/errors"
	"github.com/Microsoft/hcsshim/hcn"
	"k8s.io/klog"
)

const (
	// SetPolicyTypeNestedIPSet as a temporary measure adding it here
	// update HCSShim to version 0.9.23 or above to support nestedIPSets
	SetPolicyTypeNestedIPSet hcn.SetPolicyType = "NESTEDIPSET"
)

func (iMgr *IPSetManager) applyIPSets() error {
	network, err := iMgr.getHCnNetwork()
	if err != nil {
		return err
	}

	setPolNames := getAllSetPolicyNames(network.Policies)

	setPolSettings, err := iMgr.calculateNewSetPolicies(setPolNames)
	if err != nil {
		return err
	}

	policyNetworkRequest := hcn.PolicyNetworkRequest{
		Policies: []hcn.NetworkPolicy{},
	}

	for _, policy := range network.Policies {
		// TODO (vamsi) use NetPolicyType constant setpolicy for below check
		// after updating HCSShim
		if policy.Type != hcn.SetPolicy {
			policyNetworkRequest.Policies = append(policyNetworkRequest.Policies, policy)
		}
	}

	for setPol := range setPolSettings {
		rawSettings, err := json.Marshal(setPolSettings[setPol])
		if err != nil {
			return err
		}
		policyNetworkRequest.Policies = append(
			policyNetworkRequest.Policies,
			hcn.NetworkPolicy{
				Type:     hcn.SetPolicy,
				Settings: rawSettings,
			},
		)
	}

	err = iMgr.ioShim.Hns.AddNetworkPolicy(network, policyNetworkRequest)
	if err != nil {
		return err
	}

	return nil
}

func (iMgr *IPSetManager) calculateNewSetPolicies(existingSets []string) (map[string]*hcn.SetPolicySetting, error) {
	// some of this below logic can be abstracted a step above
	dirtySets := iMgr.toAddOrUpdateCache

	for _, setName := range existingSets {
		dirtySets[setName] = struct{}{}
	}

	setsToUpdate := make(map[string]*hcn.SetPolicySetting)
	for setName := range dirtySets {
		set, exists := iMgr.setMap[setName] // check if the Set exists
		if !exists {
			return nil, errors.Errorf(errors.AppendIPSet, false, fmt.Sprintf("member ipset %s does not exist", setName))
		}

		setPol, err := convertToSetPolicy(set)
		if err != nil {
			return nil, err
		}
		setsToUpdate[setName] = setPol
		if set.Kind == ListSet {
			for _, memberSet := range set.MemberIPSets {
				// TODO check whats the name here, hashed or normal
				if _, ok := setsToUpdate[memberSet.Name]; ok {
					continue
				}
				setPol, err = convertToSetPolicy(memberSet)
				if err != nil {
					return nil, err
				}
				setsToUpdate[memberSet.Name] = setPol
			}
		}
	}

	return setsToUpdate, nil
}

func (iMgr *IPSetManager) getHCnNetwork() (*hcn.HostComputeNetwork, error) {
	if iMgr.iMgrCfg.networkName == "" {
		iMgr.iMgrCfg.networkName = "azure"
	}
	network, err := iMgr.ioShim.Hns.GetNetworkByName("azure")
	if err != nil {
		return nil, err
	}
	return network, nil
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
		Id:     set.HashedName,
		Name:   set.Name,
		Type:   getSetPolicyType(set),
		Values: util.SliceToString(setContents),
	}
	return setPolicy, nil
}

func getAllSetPolicyNames(networkPolicies []hcn.NetworkPolicy) []string {
	setPols := []string{}
	for _, netpol := range networkPolicies {
		if netpol.Type == hcn.SetPolicy {
			var set hcn.SetPolicySetting
			err := json.Unmarshal(netpol.Settings, &set)
			if err != nil {
				klog.Error(err.Error())
				continue
			}
			setPols = append(setPols, set.Name)
		}
	}
	return setPols
}
