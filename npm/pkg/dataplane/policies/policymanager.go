package policies

import (
	"fmt"
	"sync"

	"github.com/Azure/azure-container-networking/common"
	"github.com/Azure/azure-container-networking/npm/metrics"
	"github.com/Azure/azure-container-networking/npm/util"
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

	// this number is based on the implementation in chain-management_linux.go
	// it represents the number of rules unrelated to policies
	// it's technically 3 off when there are no policies since we flush the AZURE-NPM chain then
	numLinuxBaseACLRules = 11
)

type PolicyManagerCfg struct {
	// PolicyMode only affects Windows
	PolicyMode PolicyManagerMode
	// PlaceAzureChainFirst only affects Linux
	PlaceAzureChainFirst bool
}

type PolicyMap struct {
	cache map[string]*NPMNetworkPolicy
}

type reconcileManager struct {
	sync.Mutex
	releaseLockSignal chan struct{}
}

type PolicyManager struct {
	policyMap        *PolicyMap
	ioShim           *common.IOShim
	staleChains      *staleChains
	reconcileManager *reconcileManager
	*PolicyManagerCfg
}

func NewPolicyManager(ioShim *common.IOShim, cfg *PolicyManagerCfg) *PolicyManager {
	return &PolicyManager{
		policyMap: &PolicyMap{
			cache: make(map[string]*NPMNetworkPolicy),
		},
		ioShim:      ioShim,
		staleChains: newStaleChains(),
		reconcileManager: &reconcileManager{
			releaseLockSignal: make(chan struct{}, 1),
		},
		PolicyManagerCfg: cfg,
	}
}

func (pMgr *PolicyManager) Bootup(epIDs []string) error {
	metrics.ResetNumACLRules()
	if err := pMgr.bootup(epIDs); err != nil {
		// NOTE: in Linux, Prometheus metrics may be off at this point since some ACL rules may have been applied successfully
		metrics.SendErrorLogAndMetric(util.IptmID, "error: failed to bootup policy manager: %s", err.Error())
		return npmerrors.ErrorWrapper(npmerrors.BootupPolicyMgr, false, "failed to bootup policy manager", err)
	}

	if !util.IsWindowsDP() {
		// update Prometheus metrics on success
		metrics.IncNumACLRulesBy(numLinuxBaseACLRules)
	}
	return nil
}

func (pMgr *PolicyManager) Reconcile() {
	pMgr.reconcile()
}

func (pMgr *PolicyManager) GetAllPolicies() []string {
	policyKeys := make([]string, len(pMgr.policyMap.cache))
	i := 0
	for policyKey := range pMgr.policyMap.cache {
		policyKeys[i] = policyKey
		i++
	}
	return policyKeys
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

	// TODO move this validation and normalization to controller
	NormalizePolicy(policy)
	if err := ValidatePolicy(policy); err != nil {
		msg := fmt.Sprintf("failed to validate policy: %s", err.Error())
		metrics.SendErrorLogAndMetric(util.IptmID, "error: %s", msg)
		return npmerrors.Errorf(npmerrors.AddPolicy, false, msg)
	}

	// Call actual dataplane function to apply changes
	timer := metrics.StartNewTimer()
	err := pMgr.addPolicy(policy, endpointList)
	metrics.RecordACLRuleExecTime(timer) // record execution time regardless of failure
	if err != nil {
		// NOTE: in Linux, Prometheus metrics may be off at this point since some ACL rules may have been applied successfully
		msg := fmt.Sprintf("failed to add policy: %s", err.Error())
		metrics.SendErrorLogAndMetric(util.IptmID, "error: %s", msg)
		return npmerrors.Errorf(npmerrors.AddPolicy, false, msg)
	}

	// update Prometheus metrics on success
	metrics.IncNumACLRulesBy(policy.numACLRulesProducedInKernel())

	pMgr.policyMap.cache[policy.PolicyKey] = policy
	return nil
}

func (pMgr *PolicyManager) isFirstPolicy() bool {
	return len(pMgr.policyMap.cache) == 0
}

func (pMgr *PolicyManager) RemovePolicy(policyKey string, endpointList map[string]string) error {
	policy, ok := pMgr.GetPolicy(policyKey)

	if !ok {
		return nil
	}

	if len(policy.ACLs) == 0 {
		klog.Infof("[DataPlane] No ACLs in policy %s to remove", policyKey)
		return nil
	}
	// Call actual dataplane function to apply changes
	err := pMgr.removePolicy(policy, endpointList)
	// currently we only have acl rule exec time for "adding" rules, so we skip recording here
	if err != nil {
		// NOTE: in Linux, Prometheus metrics may be off at this point since some ACL rules may have been applied successfully
		msg := fmt.Sprintf("failed to remove policy: %s", err.Error())
		metrics.SendErrorLogAndMetric(util.IptmID, "error: %s", msg)
		return npmerrors.Errorf(npmerrors.RemovePolicy, false, msg)
	}

	// update Prometheus metrics on success
	metrics.DecNumACLRulesBy(policy.numACLRulesProducedInKernel())

	delete(pMgr.policyMap.cache, policyKey)
	return nil
}

func (pMgr *PolicyManager) isLastPolicy() bool {
	// if we change our code to delete more than one policy at once, we can specify numPoliciesToDelete as an argument
	numPoliciesToDelete := 1
	return len(pMgr.policyMap.cache) == numPoliciesToDelete
}
