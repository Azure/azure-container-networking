package network

import (
	"github.com/Azure/azure-container-networking/log"
	"github.com/Azure/azure-container-networking/netio"
	"github.com/Azure/azure-container-networking/netlink"
	"github.com/Azure/azure-container-networking/network/networkutils"
	"github.com/Azure/azure-container-networking/platform"
	"github.com/pkg/errors"
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

func (client *SecondaryEndpointClient) AddEndpoints(epInfo *EndpointInfo, _ *endpoint) error {
	iface, err := client.netioshim.GetNetworkInterfaceByMac(epInfo.MacAddress)
	if err != nil {
		return newErrorSecondaryEndpointClient(err)
	}

	epInfo.IfName = iface.Name
	if _, exists := client.ep.SecondaryInterfaces[iface.Name]; exists {
		return newErrorSecondaryEndpointClient(errors.New(iface.Name + " already exists"))
	}
	client.ep.SecondaryInterfaces[iface.Name] = &InterfaceInfo{
		Name:              iface.Name,
		MacAddress:        epInfo.MacAddress,
		IPAddress:         epInfo.IPAddresses,
		NICType:           epInfo.NICType,
		SkipDefaultRoutes: epInfo.SkipDefaultRoutes,
	}

	return nil
}

func (client *SecondaryEndpointClient) AddEndpointRules(_ *EndpointInfo) error {
	return nil
}

func (client *SecondaryEndpointClient) DeleteEndpointRules(_ *endpoint) {
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
	if err := client.netlink.SetLinkState(epInfo.IfName, true); err != nil {
		return newErrorSecondaryEndpointClient(err)
	}

	return nil
}

func (client *SecondaryEndpointClient) ConfigureContainerInterfacesAndRoutes(epInfo *EndpointInfo) error {
	if err := client.netUtilsClient.AssignIPToInterface(epInfo.IfName, epInfo.IPAddresses); err != nil {
		return newErrorSecondaryEndpointClient(err)
	}

	ifInfo, exists := client.ep.SecondaryInterfaces[epInfo.IfName]
	if !exists {
		return newErrorSecondaryEndpointClient(errors.New(epInfo.IfName + " does not exist"))
	}

	if len(epInfo.Routes) < 1 {
		return newErrorSecondaryEndpointClient(errors.New("routes expected for " + epInfo.IfName))
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
