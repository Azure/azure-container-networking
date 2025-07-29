package cns

import (
	"fmt"
	"net"
	"strings"

	"github.com/Azure/azure-container-networking/cni/api"
	"github.com/Azure/azure-container-networking/cni/client"
	"github.com/Azure/azure-container-networking/cns"
	"github.com/Azure/azure-container-networking/cns/logger"
	"github.com/Azure/azure-container-networking/cns/restserver"
	"github.com/Azure/azure-container-networking/store"
	"github.com/pkg/errors"
	kexec "k8s.io/utils/exec"
)

// New returns a PodInfoByIPProvider that reads from CNS statefile endpoint store.
func New(endpointStore store.KeyValueStore) (cns.PodInfoByIPProvider, error) {
	return podInfoProvider(endpointStore)
}

func podInfoProvider(endpointStore store.KeyValueStore) (cns.PodInfoByIPProvider, error) {
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

// MigrateCNISate returns an endpoint state of CNS by reading the CNI state file
func MigrateCNISate() (map[string]*restserver.EndpointInfo, error) {
	return migrateCNISate(kexec.New())
}

func migrateCNISate(exec kexec.Interface) (map[string]*restserver.EndpointInfo, error) {
	cli := client.New(exec)
	state, err := cli.GetEndpointState()
	if err != nil {
		return nil, fmt.Errorf("failed to invoke CNI client.GetEndpointState(): %w", err)
	}
	endpointState := cniStateToCnsEndpointState(state)
	return endpointState, nil
}

// cniStateToCnsEndpointState converts an AzureCNIState dumped from a CNI exec
// into a EndpointInfo map, using the containerID as keys in the map.
// The map then will be saved on CNS endpoint state
func cniStateToCnsEndpointState(state *api.AzureCNIState) map[string]*restserver.EndpointInfo {
	logger.Printf("Generating CNS Endpoint State")
	endpointState := map[string]*restserver.EndpointInfo{}
	for epID, endpoint := range state.ContainerInterfaces {
		endpointInfo := &restserver.EndpointInfo{PodName: endpoint.PodName, PodNamespace: endpoint.PodNamespace, IfnameToIPMap: make(map[string]*restserver.IPInfo)}
		ipInfo := &restserver.IPInfo{}
		for _, epIP := range endpoint.IPAddresses {
			if epIP.IP.To4() == nil { // is an ipv6 address
				ipconfig := net.IPNet{IP: epIP.IP, Mask: epIP.Mask}
				ipInfo.IPv6 = append(ipInfo.IPv6, ipconfig)

			} else {
				ipconfig := net.IPNet{IP: epIP.IP, Mask: epIP.Mask}
				ipInfo.IPv4 = append(ipInfo.IPv4, ipconfig)
			}
		}
		endpointID, Ifname := extractEndpointInfo(epID, endpoint.ContainerID)
		endpointInfo.IfnameToIPMap[Ifname] = ipInfo
		endpointState[endpointID] = endpointInfo
		logger.Printf("CNS endpoint state extracted from CNI: [%+v]", *endpointInfo)
	}
	return endpointState
}

// extractEndpointInfo extract Interface Name and endpointID for each endpoint based the CNI state
func extractEndpointInfo(epID, containerID string) (endpointID, interfaceName string) {
	ifName := restserver.InfraInterfaceName
	if strings.Contains(epID, "-eth") {
		ifName = epID[len(epID)-4:]
	}
	if containerID == "" {
		return epID, ifName
	}
	return containerID, ifName
}
