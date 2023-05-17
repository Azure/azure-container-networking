package dataplane

import (
	"github.com/Azure/azure-container-networking/npm/pkg/dataplane/policies"
	"github.com/Azure/azure-container-networking/npm/util"
	npmerrors "github.com/Azure/azure-container-networking/npm/util/errors"
)

func (dp *DataPlane) getEndpointsToApplyPolicy(policy *policies.NPMNetworkPolicy) (map[string]string, error) {
	// NOOP in Linux
	return nil, nil
}

func (dp *DataPlane) shouldUpdatePod() bool {
	return false
}

func (dp *DataPlane) updatePod(pod *updateNPMPod) error {
	// NOOP in Linux
	return nil
}

func (dp *DataPlane) bootupDataPlane() error {
	util.DetectIptablesVersion(dp.ioShim)

	// It is important to keep order to clean-up ACLs before ipsets. Otherwise we won't be able to delete ipsets referenced by ACLs
	if err := dp.policyMgr.Bootup(nil); err != nil {
		return npmerrors.ErrorWrapper(npmerrors.BootupDataplane, false, "failed to reset policy dataplane", err)
	}
	if err := dp.ipsetMgr.ResetIPSets(); err != nil {
		return npmerrors.ErrorWrapper(npmerrors.BootupDataplane, false, "failed to reset ipsets dataplane", err)
	}
	return nil
}

func (dp *DataPlane) refreshPodEndpoints() error {
	// NOOP in Linux
	return nil
}
