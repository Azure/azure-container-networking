package policies

import (
	"fmt"

	"github.com/Azure/azure-container-networking/common"
)

type PolicyMap struct {
	cache map[string]*NPMNetworkPolicy
}

type PolicyManager struct {
	policyMap *PolicyMap
	ioShim    *common.IOShim
}

func NewPolicyManager(ioShim *common.IOShim) *PolicyManager {
	return &PolicyManager{
		policyMap: &PolicyMap{
			cache: make(map[string]*NPMNetworkPolicy),
		},
		ioShim: ioShim,
	}
}

func (pMgr *PolicyManager) PolicyExists(name string) bool {
	_, ok := pMgr.policyMap.cache[name]
	return ok
}

func (pMgr *PolicyManager) GetPolicy(name string) (*NPMNetworkPolicy, bool) {
	policy, ok := pMgr.policyMap.cache[name]
	return policy, ok
}

func (pMgr *PolicyManager) AddPolicy(policy *NPMNetworkPolicy, endpointList []string) error {
	if err := checkForErrors(policy); err != nil {
		return fmt.Errorf("couldn't add malformed policy: %v", err)
	}
	// Call actual dataplane function to apply changes
	err := pMgr.addPolicy(policy, endpointList)
	if err != nil {
		return err
	}

	pMgr.policyMap.cache[policy.Name] = policy
	return nil
}

func (pMgr *PolicyManager) RemovePolicy(name string, endpointList []string) error {
	if !pMgr.PolicyExists(name) {
		return nil
	}
	// Call actual dataplane function to apply changes
	err := pMgr.removePolicy(name, endpointList)
	if err != nil {
		return err
	}

	delete(pMgr.policyMap.cache, name)

	return nil
}

func checkForErrors(networkPolicies ...*NPMNetworkPolicy) error {
	for _, networkPolicy := range networkPolicies {
		for _, aclPolicy := range networkPolicy.ACLs {
			if !aclPolicy.hasKnownTarget() {
				return fmt.Errorf("ACL policy %s has unknown target", aclPolicy.PolicyID)
			}
			if !aclPolicy.hasKnownDirection() {
				return fmt.Errorf("ACL policy %s has unknown direction", aclPolicy.PolicyID)
			}
			if !aclPolicy.hasKnownProtocol() {
				return fmt.Errorf("ACL policy %s has unknown protocol (set to All if desired)", aclPolicy.PolicyID)
			}
			if !aclPolicy.satisifiesPortAndProtocolConstraints() {
				return fmt.Errorf("ACL policy %s has multiple src or dst ports, so must have protocol tcp, udp, udplite, sctp, or dccp but has protocol %s", aclPolicy.PolicyID, string(aclPolicy.Protocol))
			}
			for _, portRange := range aclPolicy.DstPorts {
				if !portRange.isValidRange() {
					return fmt.Errorf("ACL policy %s has invalid port range in DstPorts (start: %d, end: %d)", aclPolicy.PolicyID, portRange.Port, portRange.EndPort)
				}
			}
			for _, portRange := range aclPolicy.DstPorts {
				if !portRange.isValidRange() {
					return fmt.Errorf("ACL policy %s has invalid port range in SrcPorts (start: %d, end: %d)", aclPolicy.PolicyID, portRange.Port, portRange.EndPort)
				}
			}
			for _, setInfo := range aclPolicy.SrcList {
				if !setInfo.hasKnownMatchType() {
					return fmt.Errorf("ACL policy %s has set %s in SrcList with unknown Match Type", aclPolicy.PolicyID, setInfo.IPSet.Name)
				}
			}
			for _, setInfo := range aclPolicy.DstList {
				if !setInfo.hasKnownMatchType() {
					return fmt.Errorf("ACL policy %s has set %s in DstList with unknown Match Type", aclPolicy.PolicyID, setInfo.IPSet.Name)
				}
			}
		}
	}
	return nil
}
