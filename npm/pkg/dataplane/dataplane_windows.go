package dataplane

import (
	"github.com/Azure/azure-container-networking/npm"
	"github.com/Microsoft/hcsshim/hcn"
	"k8s.io/klog"
)

const (
	// Windows specific constants
	AzureNetworkName = "azure"
)

// initializeDataPlane will help gather network and endpoint details
func (dp *DataPlane) initializeDataPlane() error {
	klog.Infof("Initializing dataplane for windows")

	// Get Network ID
	network, err := hcn.GetNetworkByName(AzureNetworkName)
	if err != nil {
		return err
	}

	dp.networkID = network.Id

	endpoints, err := hcn.ListEndpointsOfNetwork(dp.networkID)
	if err != nil {
		return err
	}

	for _, endpoint := range endpoints {
		klog.Infof("Endpoints info %+v", endpoint.Policies)
		ep := &NPMEndpoint{
			Name:            endpoint.Name,
			ID:              endpoint.Id,
			NetPolReference: make(map[string]struct{}),
		}

		dp.endpointCache[ep.Name] = ep
	}

	return nil
}

// updatePod has two responsibilities in windows
// 1. Will call into dataplane and updates endpoint references of this pod.
// 2. Will check for existing applicable network policies and applies it on endpoint
func (dp *DataPlane) updatePod(pod *npm.NpmPod) error {
	return nil
}

func (dp *DataPlane) resetDataPlane() error {
	return nil
}
