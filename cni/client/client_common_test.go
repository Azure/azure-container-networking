// +build unit integration

package client

import (
	"net"

	"github.com/Azure/azure-container-networking/cni/api"
)

func testGetNetworkInterfaceInfo(podinterfaceid, podname, podnamespace, containerid, ipwithcidr string) api.NetworkInterfaceInfo {
	ip, ipnet, _ := net.ParseCIDR(ipwithcidr)
	ipnet.IP = ip
	return api.NetworkInterfaceInfo{
		PodName:        podname,
		PodNamespace:   podnamespace,
		PodInterfaceID: podinterfaceid,
		ContainerID:    containerid,
		IPAddresses: []net.IPNet{
			*ipnet,
		},
	}
}
