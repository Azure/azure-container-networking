package cnireconciler

import (
	"fmt"
	"net"

	"github.com/Azure/azure-container-networking/cni/api"
	"github.com/Azure/azure-container-networking/cni/client"
	"github.com/Azure/azure-container-networking/cns"
	"github.com/Azure/azure-container-networking/cns/logger"
	"github.com/Azure/azure-container-networking/cns/restserver"
	"github.com/Azure/azure-container-networking/store"
	"github.com/pkg/errors"
	"k8s.io/utils/exec"
)

// NewCNIPodInfoProvider returns an implementation of cns.PodInfoByIPProvider
// that execs out to the CNI and uses the response to build the PodInfo map.
func NewCNIPodInfoProvider() (cns.PodInfoByIPProvider, map[string]*restserver.EndpointInfo, error) {
	return newCNIPodInfoProvider(exec.New())
}

func NewCNSPodInfoProvider(endpointStore store.KeyValueStore) (cns.PodInfoByIPProvider, error) {
	return newCNSPodInfoProvider(endpointStore)
}

func newCNSPodInfoProvider(endpointStore store.KeyValueStore) (cns.PodInfoByIPProvider, error) {
	var state map[string]*restserver.EndpointInfo
	err := endpointStore.Read(restserver.EndpointStoreKey, &state)
	if err != nil {
		if errors.Is(err, store.ErrKeyNotFound) {
			// Nothing to restore.
			return cns.PodInfoByIPProviderFunc(func() (map[string]cns.PodInfo, error) {
				return endpointStateToPodInfoByIP(state)
			}), err
		}
		return nil, fmt.Errorf("failed to read endpoints state from store : %w", err)
	}
	return cns.PodInfoByIPProviderFunc(func() (map[string]cns.PodInfo, error) {
		return endpointStateToPodInfoByIP(state)
	}), nil
}

func newCNIPodInfoProvider(exec exec.Interface) (cns.PodInfoByIPProvider, map[string]*restserver.EndpointInfo, error) {
	cli := client.New(exec)
	state, err := cli.GetEndpointState()
	if err != nil {
		return nil, nil, fmt.Errorf("failed to invoke CNI client.GetEndpointState(): %w", err)
	}
	endpointState, err := cniStateToCnsEndpointState(state)
	return cns.PodInfoByIPProviderFunc(func() (map[string]cns.PodInfo, error) {
		return cniStateToPodInfoByIP(state)
	}), endpointState, nil
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
			logger.Printf("podInfoByIp [%+v]", podInfoByIP)
			ipKey := epIP.IP.String()
			if prevPodInfo, ok := podInfoByIP[ipKey]; ok {
				return nil, errors.Wrapf(cns.ErrDuplicateIP, "duplicate ip %s found for different pods: pod: %+v, pod: %+v", ipKey, podInfo, prevPodInfo)
			}

			podInfoByIP[ipKey] = podInfo
		}
	}
	logger.Printf("podInfoByIP [%+v]", podInfoByIP)
	return podInfoByIP, nil
}

func endpointStateToPodInfoByIP(state map[string]*restserver.EndpointInfo) (map[string]cns.PodInfo, error) {
	podInfoByIP := map[string]cns.PodInfo{}
	for containerID, endpointInfo := range state { // for each endpoint
		for _, ipinfo := range endpointInfo.IfnameToIPMap { // for each IP info object of the endpoint's interfaces
			for _, ipv4conf := range ipinfo.IPv4 { // for each IPv4 config of the endpoint's interfaces
				if _, ok := podInfoByIP[ipv4conf.IP.String()]; ok {
					return nil, errors.Wrap(cns.ErrDuplicateIP, ipv4conf.IP.String())
				}
				podInfoByIP[ipv4conf.IP.String()] = cns.NewPodInfo(
					containerID,
					containerID,
					endpointInfo.PodName,
					endpointInfo.PodNamespace,
				)
			}
			for _, ipv6conf := range ipinfo.IPv6 { // for each IPv6 config of the endpoint's interfaces
				if _, ok := podInfoByIP[ipv6conf.IP.String()]; ok {
					return nil, errors.Wrap(cns.ErrDuplicateIP, ipv6conf.IP.String())
				}
				podInfoByIP[ipv6conf.IP.String()] = cns.NewPodInfo(
					containerID,
					containerID,
					endpointInfo.PodName,
					endpointInfo.PodNamespace,
				)
			}
		}
	}
	return podInfoByIP, nil
}

// cniStateToCnsEndpointState converts an AzureCNIState dumped from a CNI exec
// into a EndpointInfo map, using the containerID as keys in the map.
// The map then will be saved on CNS endpoint state
func cniStateToCnsEndpointState(state *api.AzureCNIState) (map[string]*restserver.EndpointInfo, error) {
	logger.Printf("Generating CNS ENdpoint State")
	endpointState := map[string]*restserver.EndpointInfo{}
	for _, endpoint := range state.ContainerInterfaces {
		endpointInfo := &restserver.EndpointInfo{PodName: endpoint.PodName, PodNamespace: endpoint.PodNamespace, IfnameToIPMap: make(map[string]*restserver.IPInfo)}
		ipInfo := &restserver.IPInfo{}
		for _, epIP := range endpoint.IPAddresses {
			if epIP.IP.To4() == nil { // is an ipv6 address
				ipconfig := net.IPNet{IP: epIP.IP, Mask: epIP.Mask}
				for _, ipconf := range ipInfo.IPv6 {
					if ipconf.IP.Equal(ipconfig.IP) {
						logger.Printf("Found existing ipv6 ipconfig for infra container %s", endpoint.ContainerID)
						return nil, nil
					}
				}
				ipInfo.IPv6 = append(ipInfo.IPv6, ipconfig)

			} else {
				ipconfig := net.IPNet{IP: epIP.IP, Mask: epIP.Mask}
				for _, ipconf := range ipInfo.IPv4 {
					if ipconf.IP.Equal(ipconfig.IP) {
						logger.Printf("Found existing ipv4 ipconfig for infra container %s", endpoint.ContainerID)
						return nil, nil
					}
				}
				ipInfo.IPv4 = append(ipInfo.IPv4, ipconfig)
			}
		}
		endpointInfo.IfnameToIPMap["eth0"] = ipInfo
		logger.Printf("writing endpoint podName from stateful CNI %v", endpoint.PodName)
		logger.Printf("writing endpoint info from stateful CNI [%+v]", *endpointInfo)
		endpointState[endpoint.ContainerID] = endpointInfo
	}
	for containerID, endpointInfo := range endpointState {
		logger.Printf("writing endpoint state from stateful CNI [%+v]:[%+v]", containerID, *endpointInfo)
	}
	return endpointState, nil
}
