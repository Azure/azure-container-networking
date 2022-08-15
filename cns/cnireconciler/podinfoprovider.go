package cnireconciler

import (
	"fmt"

	"github.com/Azure/azure-container-networking/cni/api"
	"github.com/Azure/azure-container-networking/cni/client"
	"github.com/Azure/azure-container-networking/cns"
	"github.com/Azure/azure-container-networking/cns/restserver"
	"github.com/Azure/azure-container-networking/store"
	"github.com/pkg/errors"
	"k8s.io/utils/exec"
)

// NewCNIPodInfoProvider returns an implementation of cns.PodInfoByIPProvider
// that execs out to the CNI and uses the response to build the PodInfo map.
func NewCNIPodInfoProvider() (cns.PodInfoByIPProvider, error) {
	return newCNIPodInfoProvider(exec.New())
}

func NewCNSPodInfoProvider(endpointStore store.KeyValueStore) (cns.PodInfoByIPProvider, error) {
	return newCNSPodInfoProvider(endpointStore)
}

func newCNSPodInfoProvider(endpointStore store.KeyValueStore) (cns.PodInfoByIPProvider, error) {
	var state map[string]*restserver.EndpointInfo
	err := endpointStore.Read("endpoints", state)
	if err != nil {
		return nil, fmt.Errorf("failed to read endpoints state from store : %w", err)
	}
	return cns.PodInfoByIPProviderFunc(func() (map[string]cns.PodInfo, error) {
		return endpointStateToPodInfoByIP(state)
	}), nil
}

func newCNIPodInfoProvider(exec exec.Interface) (cns.PodInfoByIPProvider, error) {
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
// into a PodInfo map, using the first endpoint IP as the key in the map.
func cniStateToPodInfoByIP(state *api.AzureCNIState) (map[string]cns.PodInfo, error) {
	podInfoByIP := map[string]cns.PodInfo{}
	for _, endpoint := range state.ContainerInterfaces {
		if _, ok := podInfoByIP[endpoint.IPAddresses[0].IP.String()]; ok {
			return nil, errors.Wrap(cns.ErrDuplicateIP, endpoint.IPAddresses[0].IP.String())
		}
		podInfoByIP[endpoint.IPAddresses[0].IP.String()] = cns.NewPodInfo(
			endpoint.ContainerID,
			endpoint.PodEndpointId,
			endpoint.PodName,
			endpoint.PodNamespace,
		)
	}
	return podInfoByIP, nil
}

func endpointStateToPodInfoByIP(state map[string]*restserver.EndpointInfo) (map[string]cns.PodInfo, error) {
	podInfoByIP := map[string]cns.PodInfo{}
	for containerID, endpointInfo := range state {
		for ifname, ipinfo := range endpointInfo.IfnameToIPMap {
			if _, ok := podInfoByIP[ipinfo.IPv4.IP.String()]; ok {
				return nil, errors.Wrap(cns.ErrDuplicateIP, ipinfo.IPv4.IP.String())
			}
			podInfoByIP[ipinfo.IPv4.IP.String()] = cns.NewPodInfo(
				containerID,
				ifname,
				endpointInfo.PodName,
				endpointInfo.PodNamespace,
			)
			if ipinfo.IPv6 != nil { // check for IPv6
				if _, ok := podInfoByIP[ipinfo.IPv6.IP.String()]; ok {
					return nil, errors.Wrap(cns.ErrDuplicateIP, ipinfo.IPv6.IP.String())
				}
				podInfoByIP[ipinfo.IPv6.IP.String()] = cns.NewPodInfo(
					containerID,
					ifname,
					endpointInfo.PodName,
					endpointInfo.PodNamespace,
				)
			}
		}
	}
	return podInfoByIP, nil
}
