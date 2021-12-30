package policies

import (
	"fmt"
	"sync"
	"time"

	"github.com/Azure/azure-container-networking/common"
	npmerrors "github.com/Azure/azure-container-networking/npm/util/errors"
	"k8s.io/klog"
)

// PolicyManagerMode will be used in windows to decide if
// SetPolicies should be used or not
type PolicyManagerMode string

const (
	// IPSetPolicyMode will references IPSets in policies
	IPSetPolicyMode PolicyManagerMode = "IPSet"
	// IPPolicyMode will replace ipset names with their value IPs in policies
	IPPolicyMode PolicyManagerMode = "IP"

	reconcileTimeInMinutes = 5
)

type PolicyManagerCfg struct {
	// PolicyMode only affects Windows
	PolicyMode PolicyManagerMode
}

type PolicyMap struct {
	cache map[string]*NPMNetworkPolicy
}

type PolicyManager struct {
	policyMap   *PolicyMap
	ioShim      *common.IOShim
	staleChains *staleChains
	*PolicyManagerCfg
	sync.Mutex
}

func NewPolicyManager(ioShim *common.IOShim, cfg *PolicyManagerCfg) *PolicyManager {
	return &PolicyManager{
		policyMap: &PolicyMap{
			cache: make(map[string]*NPMNetworkPolicy),
		},
		ioShim:           ioShim,
		staleChains:      newStaleChains(),
		PolicyManagerCfg: cfg,
	}
}

func (pMgr *PolicyManager) Bootup(epIDs []string) error {
	if err := pMgr.bootup(epIDs); err != nil {
		return npmerrors.ErrorWrapper(npmerrors.BootupPolicyMgr, false, "failed to bootup policy manager", err)
	}
	return nil
}

// TODO call this function in DP

func (pMgr *PolicyManager) Reconcile(stopChannel <-chan struct{}) {
	go func() {
		ticker := time.NewTicker(time.Minute * time.Duration(reconcileTimeInMinutes))
		defer ticker.Stop()

		for {
			select {
			case <-stopChannel:
				return
			case <-ticker.C:
				pMgr.Lock()
				defer pMgr.Unlock()
				pMgr.reconcile()
			}
		}
	}()
}

func (pMgr *PolicyManager) PolicyExists(policyKey string) bool {
	_, ok := pMgr.policyMap.cache[policyKey]
	return ok
}

func (pMgr *PolicyManager) GetPolicy(policyKey string) (*NPMNetworkPolicy, bool) {
	policy, ok := pMgr.policyMap.cache[policyKey]
	return policy, ok
}

func (pMgr *PolicyManager) AddPolicy(policy *NPMNetworkPolicy, endpointList map[string]string) error {
	if len(policy.ACLs) == 0 {
		klog.Infof("[DataPlane] No ACLs in policy %s to apply", policy.PolicyKey)
		return nil
	}
	normalizePolicy(policy)
	if err := validatePolicy(policy); err != nil {
		return npmerrors.Errorf(npmerrors.AddPolicy, false, fmt.Sprintf("couldn't add malformed policy: %s", err.Error()))
	}
	klog.Infof("PRINTING-CONTENTS-FOR-ADDING-POLICY:\n%s", policy.String())

	// Call actual dataplane function to apply changes
	err := pMgr.addPolicy(policy, endpointList)
	if err != nil {
		return npmerrors.Errorf(npmerrors.AddPolicy, false, fmt.Sprintf("failed to add policy: %v", err))
	}

	pMgr.policyMap.cache[policy.PolicyKey] = policy
	return nil
}

func (pMgr *PolicyManager) isFirstPolicy() bool {
	return len(pMgr.policyMap.cache) == 0
}

func (pMgr *PolicyManager) RemovePolicy(policyKey string, endpointList map[string]string) error {
	policy, ok := pMgr.GetPolicy(policyKey)
	klog.Infof("PRINTING-CONTENTS-FOR-REMOVING-POLICY:\n%s", policy.String())

	if !ok {
		klog.Infof("DEBUGME-POLICY-DOESN'T-EXIST-WHEN-DELETING")
		klog.Infof("POLICY-CACHE: %+v", pMgr.policyMap.cache)
		return nil
	}

	if len(policy.ACLs) == 0 {
		klog.Infof("[DataPlane] No ACLs in policy %s to remove", policyKey)
		return nil
	}
	// Call actual dataplane function to apply changes
	err := pMgr.removePolicy(policy, endpointList)
	if err != nil {
		return npmerrors.Errorf(npmerrors.RemovePolicy, false, fmt.Sprintf("failed to remove policy: %v", err))
	}

	delete(pMgr.policyMap.cache, policyKey)
	return nil
}

func (pMgr *PolicyManager) isLastPolicy() bool {
	// if change our code to delete more than one policy at once, we can specify numPoliciesToDelete as an argument
	numPoliciesToDelete := 1
	return len(pMgr.policyMap.cache) == numPoliciesToDelete
}

func normalizePolicy(networkPolicy *NPMNetworkPolicy) {
	for _, aclPolicy := range networkPolicy.ACLs {
		if aclPolicy.Protocol == "" {
			aclPolicy.Protocol = UnspecifiedProtocol
		}

		if aclPolicy.DstPorts.EndPort == 0 {
			aclPolicy.DstPorts.EndPort = aclPolicy.DstPorts.Port
		}
	}
}

// TODO do verification in controller?
func validatePolicy(networkPolicy *NPMNetworkPolicy) error {
	for _, aclPolicy := range networkPolicy.ACLs {
		if !aclPolicy.hasKnownTarget() {
			return npmerrors.SimpleError(fmt.Sprintf("ACL policy %s has unknown target [%s]", aclPolicy.PolicyID, aclPolicy.Target))
		}
		if !aclPolicy.hasKnownDirection() {
			return npmerrors.SimpleError(fmt.Sprintf("ACL policy %s has unknown direction [%s]", aclPolicy.PolicyID, aclPolicy.Direction))
		}
		if !aclPolicy.hasKnownProtocol() {
			return npmerrors.SimpleError(fmt.Sprintf("ACL policy %s has unknown protocol [%s]", aclPolicy.PolicyID, aclPolicy.Protocol))
		}
		if !aclPolicy.satisifiesPortAndProtocolConstraints() {
			return npmerrors.SimpleError(fmt.Sprintf(
				"ACL policy %s has dst port(s) (Port or Port and EndPort), so must have protocol tcp, udp, udplite, sctp, or dccp but has protocol %s",
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
