package dataplane

import (
	"github.com/Azure/azure-container-networking/npm"
	"k8s.io/klog"
)

// initializeDataPlane should be adding required chains and rules
func (dp *DataPlane) initializeDataPlane() error {
	klog.Infof("Initializing dataplane for linux")
	return nil
}

// updatePod is no-op in Linux
func (dp *DataPlane) updatePod(pod *npm.NpmPod) error {
	return nil
}

func (dp *DataPlane) resetDataPlane() error {
	return nil
}
