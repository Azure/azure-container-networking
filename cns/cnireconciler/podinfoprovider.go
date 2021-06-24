package cnireconciler

import (
	"fmt"

	"github.com/Azure/azure-container-networking/cni/api"
	"github.com/Azure/azure-container-networking/cni/client"
	"github.com/Azure/azure-container-networking/cns"
	"k8s.io/utils/exec"
)

// NewCNIPodInfoProvider returns an implementation of cns.PodInfoByIPProvider
// that execs out to the CNI and uses the response to build the PodInfo map.
func NewCNIPodInfoProvider() (cns.PodInfoByIPProvider, error) {
	return newCNIPodInfoProvider(exec.New())
}

func newCNIPodInfoProvider(exec exec.Interface) (cns.PodInfoByIPProvider, error) {
	cli := client.New(exec)
	state, err := cli.GetEndpointState()
	if err != nil {
		return nil, fmt.Errorf("failed to invoke CNI client.GetEndpointState(): %w", err)
	}
	return cns.PodInfoByIPProviderFunc(func() map[string]cns.PodInfo {
		return cniStateToPodInfoByIP(state)
	}), nil
}

// cniStateToPodInfoByIP converts an AzureCNIState dumped from a CNI exec
// into a PodInfo map, using the first endpoint IP as the key in the map.
func cniStateToPodInfoByIP(state *api.AzureCNIState) map[string]cns.PodInfo {
	podInfoByIP := map[string]cns.PodInfo{}
	for _, endpoint := range state.ContainerInterfaces {
		podInfoByIP[endpoint.IPAddresses[0].IP.String()] = cns.NewPodInfo(
			endpoint.ContainerID,
			endpoint.PodEndpointId,
			endpoint.PodName,
			endpoint.PodNamespace,
		)
	}
	return podInfoByIP
}
