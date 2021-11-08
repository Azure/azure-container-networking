package policies

import (
	"fmt"

	"github.com/Azure/azure-container-networking/common"
	npmerrors "github.com/Azure/azure-container-networking/npm/util/errors"
	"k8s.io/klog"
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

func (pMgr *PolicyManager) Initialize() error {
	if err := pMgr.initialize(); err != nil {
		return npmerrors.ErrorWrapper(npmerrors.InitializePolicyMgr, false, "failed to initialize policy manager", err)
	}
	return nil
}

func (pMgr *PolicyManager) Reset() error {
	if err := pMgr.reset(); err != nil {
		return npmerrors.ErrorWrapper(npmerrors.ResetPolicyMgr, false, "failed to reset policy manager", err)
	}
	return nil
}

func (pMgr *PolicyManager) PolicyExists(name string) bool {
	_, ok := pMgr.policyMap.cache[name]
	return ok
}

func (pMgr *PolicyManager) GetPolicy(name string) (*NPMNetworkPolicy, bool) {
	policy, ok := pMgr.policyMap.cache[name]
	return policy, ok
}

func (pMgr *PolicyManager) AddPolicy(policy *NPMNetworkPolicy, endpointList map[string]string) error {
	if len(policy.ACLs) == 0 {
		klog.Infof("[DataPlane] No ACLs in policy %s to apply", policy.Name)
		return nil
	}
	normalizePolicy(policy)
	if err := checkForErrors(policy); err != nil {
		return npmerrors.Errorf(npmerrors.AddPolicy, false, fmt.Sprintf("couldn't add malformed policy: %s", err.Error()))
	}

	// Call actual dataplane function to apply changes
	err := pMgr.addPolicy(policy, endpointList)
	if err != nil {
		return npmerrors.Errorf(npmerrors.AddPolicy, false, fmt.Sprintf("failed to add policy: %v", err))
	}

	pMgr.policyMap.cache[policy.Name] = policy
	return nil
}

func (pMgr *PolicyManager) RemovePolicy(name string, endpointList map[string]string) error {
	policy, ok := pMgr.GetPolicy(name)
	if !ok {
		return nil
	}

	if len(policy.ACLs) == 0 {
		klog.Infof("[DataPlane] No ACLs in policy %s to remove", policy.Name)
		return nil
	}
	// Call actual dataplane function to apply changes
	err := pMgr.removePolicy(policy, endpointList)
	if err != nil {
		return npmerrors.Errorf(npmerrors.RemovePolicy, false, fmt.Sprintf("failed to remove policy: %v", err))
	}

	delete(pMgr.policyMap.cache, name)

	return nil
}

func normalizePolicy(networkPolicy *NPMNetworkPolicy) {
	for _, aclPolicy := range networkPolicy.ACLs {
		if aclPolicy.Protocol == "" {
			aclPolicy.Protocol = AnyProtocol
		}

		if aclPolicy.DstPorts.EndPort == 0 {
			aclPolicy.DstPorts.EndPort = aclPolicy.DstPorts.Port
		}
	}
}

// TODO do verification in controller?
func checkForErrors(networkPolicy *NPMNetworkPolicy) error {
	for _, aclPolicy := range networkPolicy.ACLs {
		if !aclPolicy.hasKnownTarget() {
			return npmerrors.SimpleError(fmt.Sprintf("ACL policy %s has unknown target", aclPolicy.PolicyID))
		}
		if !aclPolicy.hasKnownDirection() {
			return npmerrors.SimpleError(fmt.Sprintf("ACL policy %s has unknown direction", aclPolicy.PolicyID))
		}
		if !aclPolicy.hasKnownProtocol() {
			return npmerrors.SimpleError(fmt.Sprintf("ACL policy %s has unknown protocol (set to All if desired)", aclPolicy.PolicyID))
		}
		if !aclPolicy.satisifiesPortAndProtocolConstraints() {
			return npmerrors.SimpleError(fmt.Sprintf(
				"ACL policy %s has multiple src or dst ports, so must have protocol tcp, udp, udplite, sctp, or dccp but has protocol %s",
				aclPolicy.PolicyID,
				string(aclPolicy.Protocol),
			))
		}

		if !aclPolicy.DstPorts.isValidRange() {
			return npmerrors.SimpleError(fmt.Sprintf("ACL policy %s has invalid port range in DstPorts (start: %d, end: %d)", aclPolicy.PolicyID, aclPolicy.DstPorts.Port, aclPolicy.DstPorts.EndPort))
		}

		for _, setInfo := range aclPolicy.SrcList {
			if !setInfo.hasKnownMatchType() {
				return npmerrors.SimpleError(fmt.Sprintf("ACL policy %s has set %s in SrcList with unknown Match Type", aclPolicy.PolicyID, setInfo.IPSet.Name))
			}
		}
		for _, setInfo := range aclPolicy.DstList {
			if !setInfo.hasKnownMatchType() {
				return npmerrors.SimpleError(fmt.Sprintf("ACL policy %s has set %s in DstList with unknown Match Type", aclPolicy.PolicyID, setInfo.IPSet.Name))
			}
		}
	}
	return nil
}
