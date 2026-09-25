package restserver

import (
	"net"
	"slices"
)

func resolveEndpointStateKey(state map[string]*EndpointInfo, endpointID string) (string, bool) {
	if _, ok := state[endpointID]; ok {
		return endpointID, true
	}
	if len(endpointID) < ContainerIDLength {
		return "", false
	}
	legacyEndpointID := endpointID[:ContainerIDLength] + "-" + InfraInterfaceName
	if _, ok := state[legacyEndpointID]; ok {
		return legacyEndpointID, true
	}
	return "", false
}

func cloneEndpointState(state map[string]*EndpointInfo) map[string]*EndpointInfo {
	cloned := make(map[string]*EndpointInfo, len(state))
	for containerID, endpointInfo := range state {
		if endpointInfo == nil {
			cloned[containerID] = nil
			continue
		}
		info := &EndpointInfo{
			PodName:       endpointInfo.PodName,
			PodNamespace:  endpointInfo.PodNamespace,
			IfnameToIPMap: make(map[string]*IPInfo, len(endpointInfo.IfnameToIPMap)),
		}
		for ifName, ipInfo := range endpointInfo.IfnameToIPMap {
			if ipInfo == nil {
				info.IfnameToIPMap[ifName] = nil
				continue
			}
			info.IfnameToIPMap[ifName] = &IPInfo{
				IPv4:               cloneIPNets(ipInfo.IPv4),
				IPv6:               cloneIPNets(ipInfo.IPv6),
				HnsEndpointID:      ipInfo.HnsEndpointID,
				HnsNetworkID:       ipInfo.HnsNetworkID,
				HostVethName:       ipInfo.HostVethName,
				MacAddress:         ipInfo.MacAddress,
				NetworkContainerID: ipInfo.NetworkContainerID,
				NICType:            ipInfo.NICType,
			}
		}
		cloned[containerID] = info
	}
	return cloned
}

func cloneIPNets(ipNets []net.IPNet) []net.IPNet {
	cloned := slices.Clone(ipNets)
	for i := range cloned {
		cloned[i].IP = slices.Clone(ipNets[i].IP)
		cloned[i].Mask = slices.Clone(ipNets[i].Mask)
	}
	return cloned
}
