// +build unit integration

package client

import (
	"net"

	"github.com/Azure/azure-container-networking/cni/api"
)

func testGetPodNetworkInterfaceInfo(podendpointid, podname, podnamespace, containerid, ipwithcidr string) api.PodNetworkInterfaceInfo {
	ip, ipnet, _ := net.ParseCIDR(ipwithcidr)
	ipnet.IP = ip
	return api.PodNetworkInterfaceInfo{
		PodName:       podname,
		PodNamespace:  podnamespace,
		PodEndpointId: podendpointid,
		ContainerID:   containerid,
		IPAddresses: []net.IPNet{
			*ipnet,
		},
	}
}
