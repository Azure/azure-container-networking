package ipsets

import (
	"encoding/json"
	"fmt"

	"github.com/Azure/azure-container-networking/npm/util"
	"github.com/Azure/azure-container-networking/npm/util/errors"
	"github.com/Microsoft/hcsshim/hcn"
	"k8s.io/klog"
)

// SetPolicyTypes associated with SetPolicy. Value is IPSET.
type SetPolicyType string

const (
	SetPolicyTypeIpSet       SetPolicyType = "IPSET"
	SetPolicyTypeNestedIpSet SetPolicyType = "NESTEDIPSET"
)

// SetPolicySetting creates IPSets on network
type SetPolicySetting struct {
	Id     string
	Name   string
	Type   SetPolicyType
	Values string
}

func (iMgr *IPSetManager) applyIPSets(networkID string) error {
	network, err := hcn.GetNetworkByID(networkID)
	if err != nil {
		return err
	}

	setPolNames, err := getAllSetPolicyNames(network.Policies)
	if err != nil {
		return err
	}

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
		if policy.Type != "SetPolicy" {
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
				Type:     "SetPolicy",
				Settings: rawSettings,
			},
		)
	}

	err = network.AddPolicy(policyNetworkRequest)
	if err != nil {
		return err
	}

	return nil
}

func (iMgr *IPSetManager) calculateNewSetPolicies(existingSets []string) (map[string]SetPolicySetting, error) {
	// some of this below logic can be abstracted a step above
	dirtySets := iMgr.dirtyCaches

	for _, setName := range existingSets {
		dirtySets[setName] = struct{}{}
	}

	setsToUpdate := make(map[string]SetPolicySetting)
	for setName := range dirtySets {
		set, exists := iMgr.setMap[setName] // check if the Set exists
		if !exists {
			return nil, errors.Errorf(errors.AppendIPSet, false, fmt.Sprintf("member ipset %s does not exist", setName))
		}
		if !set.UsedByNetPol() {
			continue
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

func isValidIPSet(set *IPSet) error {
	if set.Name == "" {
		return fmt.Errorf("IPSet " + set.Name + " is missing Name")
	}

	if set.Type == Unknown {
		return fmt.Errorf("IPSet " + set.Type.String() + " is missing Type")
	}

	if set.HashedName == "" {
		return fmt.Errorf("IPSet " + set.HashedName + " is missing HashedName")
	}

	return nil
}

func getSetPolicyType(set *IPSet) SetPolicyType {
	switch set.Kind {
	case ListSet:
		return SetPolicyTypeNestedIpSet
	case HashSet:
		return SetPolicyTypeIpSet
	default:
		return "Unknown"
	}
}

func convertToSetPolicy(set *IPSet) (SetPolicySetting, error) {
	err := isValidIPSet(set)
	if err != nil {
		return SetPolicySetting{}, err
	}

	setContents, err := set.GetSetContents()
	if err != nil {
		return SetPolicySetting{}, err
	}

	setPolicy := SetPolicySetting{
		Id:     set.HashedName,
		Name:   set.Name,
		Type:   getSetPolicyType(set),
		Values: util.SliceToString(setContents),
	}
	return setPolicy, nil
}

func getAllSetPolicyNames(networkPolicies []hcn.NetworkPolicy) ([]string, error) {
	setPols := []string{}
	for _, netpol := range networkPolicies {
		if netpol.Type == "SetPolicy" {
			var set SetPolicySetting
			err := json.Unmarshal(netpol.Settings, &set)
			if err != nil {
				klog.Error(err.Error())
				continue
			}
			setPols = append(setPols, set.Name)
		}
	}
	return setPols, nil
}
