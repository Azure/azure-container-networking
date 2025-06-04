package v2

import (
	"github.com/Azure/azure-container-networking/cns"
	"github.com/Azure/azure-container-networking/crd/nodenetworkconfig/api/v1alpha"
	v1 "k8s.io/api/core/v1"
)

var _ cns.IPAMPoolMonitor = (*adapter)(nil)

type adapter struct {
	nncSink chan<- v1alpha.NodeNetworkConfig
	*Monitor
}

func (m *Monitor) AsV1(nncSink chan<- v1alpha.NodeNetworkConfig) cns.IPAMPoolMonitor {
	return &adapter{
		nncSink: nncSink,
		Monitor: m,
	}
}

func (m *adapter) Update(nnc *v1alpha.NodeNetworkConfig) error {
	m.nncSink <- *nnc
	return nil
}

func (m *adapter) GetStateSnapshot() cns.IpamPoolMonitorStateSnapshot {
	return cns.IpamPoolMonitorStateSnapshot{}
}

func PodIPDemandListener(ch chan<- int) func([]v1.Pod) {
	return func(pods []v1.Pod) {
		// Filter out Pods in terminal phases (Succeeded/Failed) since they no longer
		// have network sandboxes and don't contribute to IP demand
		activePods := 0
		for i := range pods {
			if pods[i].Status.Phase != v1.PodSucceeded && pods[i].Status.Phase != v1.PodFailed {
				activePods++
			}
		}
		ch <- activePods
	}
}
