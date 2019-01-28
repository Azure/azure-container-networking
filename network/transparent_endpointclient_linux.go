package network

import (
	"fmt"
	"net"

	"github.com/Azure/azure-container-networking/log"
	"github.com/Azure/azure-container-networking/netlink"
	"github.com/Azure/azure-container-networking/network/epcommon"
	"github.com/Azure/azure-container-networking/platform"
)

const (
	FAKE_GW_IP = "169.254.1.1/32"
	DEFAULT_GW = "0.0.0.0/0"
)

type TransparentEndpointClient struct {
	bridgeName        string
	hostPrimaryIfName string
	hostVethName      string
	containerVethName string
	hostPrimaryMac    net.HardwareAddr
	containerMac      net.HardwareAddr
	hostVethMac       net.HardwareAddr
	mode              string
}

func NewTransparentEndpointClient(
	extIf *externalInterface,
	hostVethName string,
	containerVethName string,
	mode string,
) *TransparentEndpointClient {

	client := &TransparentEndpointClient{
		bridgeName:        extIf.BridgeName,
		hostPrimaryIfName: extIf.Name,
		hostVethName:      hostVethName,
		containerVethName: containerVethName,
		hostPrimaryMac:    extIf.MacAddress,
		mode:              mode,
	}

	return client
}

func setArpProxy(ifName string) error {
	cmd := fmt.Sprintf("echo 1 > /proc/sys/net/ipv4/conf/%v/proxy_arp", ifName)
	_, err := platform.ExecuteCommand(cmd)
	return err
}

func (client *TransparentEndpointClient) AddEndpoints(epInfo *EndpointInfo) error {

	if _, err := net.InterfaceByName(client.hostVethName); err == nil {
		log.Printf("Deleting old host veth %v", client.hostVethName)
		if err = netlink.DeleteLink(client.hostVethName); err != nil {
			log.Printf("[net] Failed to delete old hostveth %v: %v.", client.hostVethName, err)
			return err
		}
	}

	if err := epcommon.CreateEndpoint(client.hostVethName, client.containerVethName); err != nil {
		return err
	}

	containerIf, err := net.InterfaceByName(client.containerVethName)
	if err != nil {
		return err
	}

	client.containerMac = containerIf.HardwareAddr

	hostVethIf, err := net.InterfaceByName(client.hostVethName)
	if err != nil {
		return err
	}

	client.hostVethMac = hostVethIf.HardwareAddr

	return nil
}

func (client *TransparentEndpointClient) AddEndpointRules(epInfo *EndpointInfo) error {
	var routeInfoList []RouteInfo

	// ip route add <podip> dev <hostveth>
	// This route is needed for incoming packets to pod to route via hostveth
	for _, ipAddr := range epInfo.IPAddresses {
		var routeInfo RouteInfo
		ipNet := net.IPNet{IP: ipAddr.IP, Mask: net.CIDRMask(32, 32)}
		log.Printf("[net] Adding route for the ip %v", ipNet.String())
		routeInfo.Dst = ipNet
		routeInfoList = append(routeInfoList, routeInfo)
		if err := addRoutes(client.hostVethName, routeInfoList); err != nil {
			return err
		}
	}

	log.Printf("calling setArpProxy for %v", client.hostVethName)
	if err := setArpProxy(client.hostVethName); err != nil {
		log.Printf("setArpProxy failed with: %v", err)
		return err
	}

	return nil
}

func (client *TransparentEndpointClient) DeleteEndpointRules(ep *endpoint) {
	var routeInfoList []RouteInfo

	// ip route del <podip> dev <hostveth>
	// Deleting the route set up for routing the incoming packets to pod
	for _, ipAddr := range ep.IPAddresses {
		var routeInfo RouteInfo
		ipNet := net.IPNet{IP: ipAddr.IP, Mask: net.CIDRMask(32, 32)}
		log.Printf("[net] Deleting route for the ip %v", ipNet.String())
		routeInfo.Dst = ipNet
		routeInfoList = append(routeInfoList, routeInfo)
		deleteRoutes(client.hostVethName, routeInfoList)
	}
}

func (client *TransparentEndpointClient) MoveEndpointsToContainerNS(epInfo *EndpointInfo, nsID uintptr) error {
	// Move the container interface to container's network namespace.
	log.Printf("[net] Setting link %v netns %v.", client.containerVethName, epInfo.NetNsPath)
	if err := netlink.SetLinkNetNs(client.containerVethName, nsID); err != nil {
		return err
	}

	return nil
}

func (client *TransparentEndpointClient) SetupContainerInterfaces(epInfo *EndpointInfo) error {
	if err := epcommon.SetupContainerInterface(client.containerVethName, epInfo.IfName); err != nil {
		return err
	}

	client.containerVethName = epInfo.IfName

	return nil
}

func (client *TransparentEndpointClient) ConfigureContainerInterfacesAndRoutes(epInfo *EndpointInfo) error {
	if err := epcommon.AssignIPToInterface(client.containerVethName, epInfo.IPAddresses); err != nil {
		return err
	}

	return addRoutes(client.containerVethName, epInfo.Routes)
}

func (client *TransparentEndpointClient) DeleteEndpoints(ep *endpoint) error {
	log.Printf("[net] Deleting veth pair %v %v.", ep.HostIfName, ep.IfName)
	err := netlink.DeleteLink(ep.HostIfName)
	if err != nil {
		log.Printf("[net] Failed to delete veth pair %v: %v.", ep.HostIfName, err)
		return err
	}

	return nil
}
