package cni

import (
	"fmt"

	"github.com/Azure/azure-container-networking/cni/api"
	"github.com/Azure/azure-container-networking/cni/client"
	"github.com/Azure/azure-container-networking/cns"
	"github.com/pkg/errors"
	kexec "k8s.io/utils/exec"
)

// New returns an implementation of cns.PodInfoByIPProvider
// that execs out to the CNI and uses the response to build the PodInfo map.
func New() (cns.PodInfoByIPProvider, error) {
	return podInfoProvider(kexec.New())
}

func podInfoProvider(exec kexec.Interface) (cns.PodInfoByIPProvider, error) {
	cli := client.New(exec)
	state, err := cli.GetEndpointState()
	if err != nil {
		return nil, fmt.Errorf("failed to invoke CNI client.GetEndpointState(): %w", err)
	}
	return cns.PodInfoByIPProviderFunc(func() (map[string]cns.PodInfo, error) {
		return cniStateToPodInfoByIP(state)
	}), nil
}

// cniStateToPodInfoByIP converts an AzureCNIState dumped from a CNI exec
// into a PodInfo map, using the endpoint IPs as keys in the map.
// for pods with multiple IPs (such as in dualstack cases), this means multiple keys in the map
// will point to the same pod information.
func cniStateToPodInfoByIP(state *api.AzureCNIState) (map[string]cns.PodInfo, error) {
	podInfoByIP := map[string]cns.PodInfo{}
	for _, endpoint := range state.ContainerInterfaces {
		for _, epIP := range endpoint.IPAddresses {
			podInfo := cns.NewPodInfo(endpoint.ContainerID, endpoint.PodEndpointId, endpoint.PodName, endpoint.PodNamespace)

			ipKey := epIP.IP.String()
			if prevPodInfo, ok := podInfoByIP[ipKey]; ok {
				return nil, errors.Wrapf(cns.ErrDuplicateIP, "duplicate ip %s found for different pods: pod: %+v, pod: %+v", ipKey, podInfo, prevPodInfo)
			}

			podInfoByIP[ipKey] = podInfo
		}
	}
	return podInfoByIP, nil
}
