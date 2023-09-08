package network

import (
	"net"

	"github.com/pkg/errors"

	"github.com/Azure/azure-container-networking/log"
	"github.com/Azure/azure-container-networking/netio"
	"github.com/Azure/azure-container-networking/netlink"
	"github.com/Azure/azure-container-networking/network/networkutils"
	"github.com/Azure/azure-container-networking/platform"
)

var errorSecondaryEndpointClient = errors.New("SecondaryEndpointClient Error")

func newErrorSecondaryEndpointClient(err error) error {
	return errors.Wrapf(err, "%s", errorSecondaryEndpointClient)
}

type SecondaryEndpointClient struct {
	netlink        netlink.NetlinkInterface
	netioshim      netio.NetIOInterface
	plClient       platform.ExecClient
	netUtilsClient networkutils.NetworkUtils
	ep             *endpoint
}

func NewSecondaryEndpointClient(
	nl netlink.NetlinkInterface,
	plc platform.ExecClient,
	endpoint *endpoint,
) *SecondaryEndpointClient {
	client := &SecondaryEndpointClient{
		netlink:        nl,
		netioshim:      &netio.NetIO{},
		plClient:       plc,
		netUtilsClient: networkutils.NewNetworkUtils(nl, plc),
		ep:             endpoint,
	}

	return client
}

func (client *SecondaryEndpointClient) AddEndpoints(epInfo *EndpointInfo) error {
	iface, err := client.netioshim.GetNetworkInterfaceByMac(epInfo.MacAddress)
	if err != nil {
		return newErrorSecondaryEndpointClient(err)
	}

	epInfo.IfName = iface.Name
	if _, exists := client.ep.SecondaryInterfaces[iface.Name]; exists {
		return newErrorSecondaryEndpointClient(errors.New(iface.Name + " already exists"))
	}
	client.ep.SecondaryInterfaces[iface.Name] = &InterfaceInfo{
		Name:               iface.Name,
		MacAddress:         epInfo.MacAddress,
		IPAddress:          epInfo.IPAddresses,
		AddressType:        epInfo.AddressType,
		IsDefaultInterface: epInfo.IsDefaultInterface,
	}

	return nil
}

func (client *SecondaryEndpointClient) AddEndpointRules(_ *EndpointInfo) error {
	return nil
}

func (client *SecondaryEndpointClient) DeleteEndpointRules(ep *endpoint) {
	// ip route del <multitenant ip> dev <ethX>
	// Deleting the route set up for routing the incoming packets to pod
	for ifName, ifInfo := range ep.SecondaryInterfaces {
		log.Printf("[net] deleting routes on %s: %+v", ifName, ifInfo.Routes)
		if err := deleteRoutes(client.netlink, client.netioshim, ifName, ifInfo.Routes); err != nil {
			log.Printf("[net] Failed to delete routes %+v on %s: %v", ifInfo.Routes, ifName, err)
		}
	}
}

func (client *SecondaryEndpointClient) MoveEndpointsToContainerNS(epInfo *EndpointInfo, nsID uintptr) error {
	// Move the container interface to container's network namespace.
	log.Printf("[net] Setting link %v netns %v.", epInfo.IfName, epInfo.NetNsPath)
	if err := client.netlink.SetLinkNetNs(epInfo.IfName, nsID); err != nil {
		return newErrorSecondaryEndpointClient(err)
	}

	return nil
}

func (client *SecondaryEndpointClient) SetupContainerInterfaces(epInfo *EndpointInfo) error {
	log.Printf("[net] Setting link %v state up.", epInfo.IfName)

	return client.netlink.SetLinkState(epInfo.IfName, true)
}

func (client *SecondaryEndpointClient) ConfigureContainerInterfacesAndRoutes(epInfo *EndpointInfo) error {
	if err := client.netUtilsClient.AssignIPToInterface(epInfo.IfName, epInfo.IPAddresses); err != nil {
		return newErrorSecondaryEndpointClient(err)
	}

	ifInfo, exists := client.ep.SecondaryInterfaces[epInfo.IfName]
	if !exists {
		return newErrorSecondaryEndpointClient(errors.New(epInfo.IfName + " does not exist"))
	}

	if epInfo.IsDefaultInterface {
		// add route for virtualgwip
		// ip route add 169.254.1.1/32 dev ethX
		virtualGwIP, virtualGwNet, _ := net.ParseCIDR(virtualGwIPString)
		routeInfo := RouteInfo{
			Dst:   *virtualGwNet,
			Scope: netlink.RT_SCOPE_LINK,
		}
		if err := addRoutes(client.netlink, client.netioshim, epInfo.IfName, []RouteInfo{routeInfo}); err != nil {
			return newErrorSecondaryEndpointClient(err)
		}

		// TO-DO: will default routes need to be cleaned up even though they're added in pod netns?
		// looks like interface goes back to default state (down without routes) after deleting pod
		ifInfo.Routes = append(ifInfo.Routes, routeInfo)

		// ip route add default via 169.254.1.1 dev ethX
		_, defaultIPNet, _ := net.ParseCIDR(defaultGwCidr)
		dstIP := net.IPNet{IP: net.ParseIP(defaultGw), Mask: defaultIPNet.Mask}
		routeInfo = RouteInfo{
			Dst: dstIP,
			Gw:  virtualGwIP,
		}
		if err := addRoutes(client.netlink, client.netioshim, epInfo.IfName, []RouteInfo{routeInfo}); err != nil {
			return newErrorSecondaryEndpointClient(err)
		}

		// TO-DO: will default routes need to be cleaned up even though they're added in pod netns?
		// looks like interface goes back to default state (down without routes) after deleting pod
		ifInfo.Routes = append(ifInfo.Routes, routeInfo)
	}

	if err := addRoutes(client.netlink, client.netioshim, epInfo.IfName, epInfo.Routes); err != nil {
		return newErrorSecondaryEndpointClient(err)
	}

	ifInfo.Routes = append(ifInfo.Routes, epInfo.Routes...)

	return nil
}

func (client *SecondaryEndpointClient) DeleteEndpoints(_ *endpoint) error {
	// TO-DO: try to clean up and move back to default ns?
	// looks like interface goes back to default state (down without routes) after deleting pod
	return nil
}
