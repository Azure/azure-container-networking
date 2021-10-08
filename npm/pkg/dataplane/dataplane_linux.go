package dataplane

import (
	"github.com/Azure/azure-container-networking/npm/pkg/dataplane/policies"
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
func (dp *DataPlane) updatePod(pod *UpdateNPMPod) error {
	return nil
}

func (dp *DataPlane) resetDataPlane() error {
	return nil
}
