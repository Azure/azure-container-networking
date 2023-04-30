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
	// NOTE: this is currently unimplemented
	IPPolicyMode PolicyManagerMode = "IP"

	// this number is based on the implementation in chain-management_linux.go
	// it represents the number of rules unrelated to policies
	// it's technically 3 off when there are no policies since we flush the AZURE-NPM chain then
	numLinuxBaseACLRules = 11
)

type PolicyManagerCfg struct {
	// NodeIP is only used in Windows
	NodeIP string
	// PolicyMode only affects Windows
	PolicyMode PolicyManagerMode
	// PlaceAzureChainFirst only affects Linux
	PlaceAzureChainFirst bool
	// MaxBatchedACLsPerPod is the maximum number of ACLs that can be added to a Pod at once in Windows.
	// The zero value is valid.
	// A NetworkPolicy's ACLs are always in the same batch, and there will be at least one NetworkPolicy per batch.
	MaxBatchedACLsPerPod int
}

type PolicyMap struct {
	sync.RWMutex
	cache map[string]*NPMNetworkPolicy
}

type reconcileManager struct {
	sync.Mutex
	releaseLockSignal chan struct{}
}

// PolicyManager has two locks.
// The PolicyMap lock is used only in Windows to prevent concurrent write access to the PolicyMap
// from both the NetPol Controller thread and the PodController thread, accessed respectively from
// dataplane.AddPolicy()/dataplane.RemovePolicy(), and dataplane.ApplyDataplane() --> dataplane.updatePod().
// In Linux, the reconcileManager's lock is used to avoid iptables contention for adding/removing policies versus
// background cleanup of stale, ineffective chains.
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

func (pMgr *PolicyManager) ResetEndpoint(epID string) error {
	if util.IsWindowsDP() {
		return pMgr.bootup([]string{epID})
	}
	return nil
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

	if util.IsWindowsDP() && pMgr.NodeIP == "" {
		return npmerrors.Errorf(npmerrors.BootupPolicyMgr, false, "policy manager must have a configured nodeIP in Windows")
	}

	return nil
}

func (pMgr *PolicyManager) Reconcile() {
	pMgr.reconcile()
}

func (pMgr *PolicyManager) PolicyExists(policyKey string) bool {
	pMgr.policyMap.RLock()
	defer pMgr.policyMap.RUnlock()

	_, ok := pMgr.policyMap.cache[policyKey]
	return ok
}

func (pMgr *PolicyManager) GetPolicy(policyKey string) (*NPMNetworkPolicy, bool) {
	pMgr.policyMap.RLock()
	defer pMgr.policyMap.RUnlock()

	policy, ok := pMgr.policyMap.cache[policyKey]
	return policy, ok
}

func (pMgr *PolicyManager) AddPolicy(policy *NPMNetworkPolicy, endpointList map[string]string) error {
	if len(policy.ACLs) == 0 {
		klog.Infof("[DataPlane] No ACLs in policy %s to apply", policy.PolicyKey)
		return nil
	}

	NormalizePolicy(policy)
	if err := ValidatePolicy(policy); err != nil {
		msg := fmt.Sprintf("failed to validate policy: %s", err.Error())
		metrics.SendErrorLogAndMetric(util.IptmID, "error: %s", msg)
		return npmerrors.Errorf(npmerrors.AddPolicy, false, msg)
	}

	pMgr.policyMap.Lock()
	defer pMgr.policyMap.Unlock()

	// Call actual dataplane function to apply changes
	timer := metrics.StartNewTimer()
	err := pMgr.addPolicy(policy, endpointList)
	metrics.RecordACLRuleExecTime(timer) // record execution time regardless of failure
	if err != nil {
		// NOTE: in Linux, Prometheus metrics may be off at this point since some ACL rules may have been applied successfully
		// In Windows, Prometheus metrics may be off at this point since we don't know how many endpoints had rules applied successfully.
		msg := fmt.Sprintf("failed to add policy: %s", err.Error())
		metrics.SendErrorLogAndMetric(util.IptmID, "error: %s", msg)
		return npmerrors.Errorf(npmerrors.AddPolicy, false, msg)
	}

	// update Prometheus metrics on success
	if util.IsWindowsDP() {
		metrics.IncNumACLRulesBy((1 + policy.numACLRulesProducedInKernel()) * len(endpointList))
	} else {
		metrics.IncNumACLRulesBy(policy.numACLRulesProducedInKernel())
	}

	pMgr.policyMap.cache[policy.PolicyKey] = policy
	return nil
}

func (pMgr *PolicyManager) isFirstPolicy() bool {
	return len(pMgr.policyMap.cache) == 0
}

func (pMgr *PolicyManager) RemovePolicy(policyKey string) error {
	policy, ok := pMgr.GetPolicy(policyKey)

	if !ok {
		return nil
	}

	if len(policy.ACLs) == 0 {
		klog.Infof("[DataPlane] No ACLs in policy %s to remove", policyKey)
		return nil
	}

	pMgr.policyMap.Lock()
	defer pMgr.policyMap.Unlock()

	// used for Prometheus metrics later
	numEndpointsBefore := len(policy.PodEndpoints)

	// Call actual dataplane function to apply changes
	err := pMgr.removePolicy(policy, nil)
	// currently we only have acl rule exec time for "adding" rules, so we skip recording here
	if err != nil {
		// NOTE: in Linux, Prometheus metrics may be off at this point since some ACL rules may have been applied successfully.
		// In Windows, Prometheus metrics may be off at this point since we don't know how many endpoints had rules applied successfully.
		msg := fmt.Sprintf("failed to remove policy: %s", err.Error())
		metrics.SendErrorLogAndMetric(util.IptmID, "error: %s", msg)
		return npmerrors.Errorf(npmerrors.RemovePolicy, false, msg)
	}

	// update Prometheus metrics on success
	if util.IsWindowsDP() {
		numEndpointsRemoved := numEndpointsBefore - len(policy.PodEndpoints)
		metrics.DecNumACLRulesBy((1 + policy.numACLRulesProducedInKernel()) * numEndpointsRemoved)
	} else {
		metrics.DecNumACLRulesBy(policy.numACLRulesProducedInKernel())
	}

	// remove policy from cache
	delete(pMgr.policyMap.cache, policyKey)
	return nil
}

// RemovePolicyForEndpoints is identical to RemovePolicy except it will not remove the policy from the cache.
// This function is intended for Windows only.
func (pMgr *PolicyManager) RemovePolicyForEndpoints(policyKey string, endpointList map[string]string) error {
	policy, ok := pMgr.GetPolicy(policyKey)

	if !ok {
		return nil
	}

	if len(policy.ACLs) == 0 {
		klog.Infof("[DataPlane] No ACLs in policy %s to remove for endpoints", policyKey)
		return nil
	}
	// Call actual dataplane function to apply changes
	err := pMgr.removePolicy(policy, endpointList)
	// currently we only have acl rule exec time for "adding" rules, so we skip recording here
	if err != nil {
		// NOTE: Prometheus metrics may be off at this point since we don't know how many endpoints had rules applied successfully.
		msg := fmt.Sprintf("failed to remove policy. endpoints: [%+v]. err: [%s]", endpointList, err.Error())
		metrics.SendErrorLogAndMetric(util.IptmID, "error: %s", msg)
		return npmerrors.Errorf(npmerrors.RemovePolicy, false, msg)
	}

	// update Prometheus metrics on success
	metrics.DecNumACLRulesBy((1 + policy.numACLRulesProducedInKernel()) * len(endpointList))

	return nil
}

func (pMgr *PolicyManager) isLastPolicy() bool {
	// if we change our code to delete more than one policy at once, we can specify numPoliciesToDelete as an argument
	numPoliciesToDelete := 1
	return len(pMgr.policyMap.cache) == numPoliciesToDelete
}
