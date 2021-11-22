package dataplane

import (
	"github.com/Azure/azure-container-networking/npm/pkg/dataplane/policies"
	npmerrors "github.com/Azure/azure-container-networking/npm/util/errors"
	"k8s.io/klog"
)

// initializeDataPlane should be adding required chains and rules
func (dp *DataPlane) initializeDataPlane() error {
	klog.Infof("Initializing dataplane for linux")
	return nil
}

func (dp *DataPlane) getEndpointsToApplyPolicy(policy *policies.NPMNetworkPolicy) (map[string]string, error) {
	// NOOP in Linux at the moment
	return nil, nil
}

func (dp *DataPlane) shouldUpdatePod() bool {
	return false
}

// updatePod is no-op in Linux
func (dp *DataPlane) updatePod(pod *updateNPMPod) error {
	return nil
}

func (dp *DataPlane) resetDataPlane() error {
	// It is important to keep order to clean-up ACLs before ipsets. Otherwise we won't be able to delete ipsets referenced by ACLs
	if err := dp.policyMgr.Reset(nil); err != nil {
		return npmerrors.ErrorWrapper(npmerrors.ResetDataPlane, false, "failed to reset policy dataplane", err)
	}
	if err := dp.ipsetMgr.ResetIPSets(); err != nil {
		return npmerrors.ErrorWrapper(npmerrors.ResetDataPlane, false, "failed to reset ipsets dataplane", err)
	}
	return nil
}
