package restserver

import "net"

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
				IPv4:               append([]net.IPNet(nil), ipInfo.IPv4...),
				IPv6:               append([]net.IPNet(nil), ipInfo.IPv6...),
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
