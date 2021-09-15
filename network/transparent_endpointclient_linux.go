package network

import (
	"errors"
	"fmt"
	"net"

	"github.com/Azure/azure-container-networking/log"
	"github.com/Azure/azure-container-networking/netlink"
	"github.com/Azure/azure-container-networking/network/epcommon"
	"github.com/Azure/azure-container-networking/network/netlinkinterface"
	"github.com/Azure/azure-container-networking/platform"
)

const (
	virtualGwIPString = "169.254.1.1/32"
	defaultGwCidr     = "0.0.0.0/0"
	defaultGw         = "0.0.0.0"
)

var errorTransparentEndpointClient = errors.New("TransparentEndpointClient Error")

func newErrorTransparentEndpointClient(errStr string) error {
	return fmt.Errorf("%w : %s", errorTransparentEndpointClient, errStr)
}

type TransparentEndpointClient struct {
	bridgeName        string
	hostPrimaryIfName string
	hostVethName      string
	containerVethName string
	hostPrimaryMac    net.HardwareAddr
	containerMac      net.HardwareAddr
	hostVethMac       net.HardwareAddr
	mode              string
	netlink           netlinkinterface.NetlinkInterface
}

func NewTransparentEndpointClient(
	extIf *externalInterface,
	hostVethName string,
	containerVethName string,
	mode string,
	nl netlinkinterface.NetlinkInterface,
) *TransparentEndpointClient {

	client := &TransparentEndpointClient{
		bridgeName:        extIf.BridgeName,
		hostPrimaryIfName: extIf.Name,
		hostVethName:      hostVethName,
		containerVethName: containerVethName,
		hostPrimaryMac:    extIf.MacAddress,
		mode:              mode,
		netlink:           nl,
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
		if err = client.netlink.DeleteLink(client.hostVethName); err != nil {
			log.Printf("[net] Failed to delete old hostveth %v: %v.", client.hostVethName, err)
			return newErrorTransparentEndpointClient(err.Error())
		}
	}

	epc := epcommon.NewEPCommon(client.netlink)
	if err := epc.CreateEndpoint(client.hostVethName, client.containerVethName); err != nil {
		return newErrorTransparentEndpointClient(err.Error())
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
		if err := addRoutes(client.netlink, client.hostVethName, routeInfoList); err != nil {
			return newErrorTransparentEndpointClient(err.Error())
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
	var err error
	// ip route del <podip> dev <hostveth>
	// Deleting the route set up for routing the incoming packets to pod
	for _, ipAddr := range ep.IPAddresses {
		var routeInfo RouteInfo
		ipNet := net.IPNet{IP: ipAddr.IP, Mask: net.CIDRMask(32, 32)}
		log.Printf("[net] Deleting route for the ip %v", ipNet.String())
		routeInfo.Dst = ipNet
		routeInfoList = append(routeInfoList, routeInfo)
		err = deleteRoutes(client.netlink, client.hostVethName, routeInfoList)
		if err != nil {
			log.Printf("[net] Failed to delete route for the ip %v: %v", ipNet.String(), err)
		}
	}
}

func (client *TransparentEndpointClient) MoveEndpointsToContainerNS(epInfo *EndpointInfo, nsID uintptr) error {
	// Move the container interface to container's network namespace.
	log.Printf("[net] Setting link %v netns %v.", client.containerVethName, epInfo.NetNsPath)
	if err := client.netlink.SetLinkNetNs(client.containerVethName, nsID); err != nil {
		return newErrorTransparentEndpointClient(err.Error())
	}

	return nil
}

func (client *TransparentEndpointClient) SetupContainerInterfaces(epInfo *EndpointInfo) error {
	epc := epcommon.NewEPCommon(client.netlink)
	if err := epc.SetupContainerInterface(client.containerVethName, epInfo.IfName); err != nil {
		return err
	}

	client.containerVethName = epInfo.IfName

	return nil
}

func (client *TransparentEndpointClient) ConfigureContainerInterfacesAndRoutes(epInfo *EndpointInfo) error {
	epc := epcommon.NewEPCommon(client.netlink)
	if err := epc.AssignIPToInterface(client.containerVethName, epInfo.IPAddresses); err != nil {
		return newErrorTransparentEndpointClient(err.Error())
	}

	// ip route del 10.240.0.0/12 dev eth0 (removing kernel subnet route added by above call)
	for _, ipAddr := range epInfo.IPAddresses {
		_, ipnet, _ := net.ParseCIDR(ipAddr.String())
		routeInfo := RouteInfo{
			Dst:      *ipnet,
			Scope:    netlink.RT_SCOPE_LINK,
			Protocol: netlink.RTPROT_KERNEL,
		}
		if err := deleteRoutes(client.netlink, client.containerVethName, []RouteInfo{routeInfo}); err != nil {
			return newErrorTransparentEndpointClient(err.Error())
		}
	}

	// add route for virtualgwip
	// ip route add 169.254.1.1/32 dev eth0
	virtualGwIP, virtualGwNet, _ := net.ParseCIDR(virtualGwIPString)
	routeInfo := RouteInfo{
		Dst:   *virtualGwNet,
		Scope: netlink.RT_SCOPE_LINK,
	}
	if err := addRoutes(client.netlink, client.containerVethName, []RouteInfo{routeInfo}); err != nil {
		return newErrorTransparentEndpointClient(err.Error())
	}

	// ip route add default via 169.254.1.1 dev eth0
	_, defaultIPNet, _ := net.ParseCIDR(defaultGwCidr)
	dstIP := net.IPNet{IP: net.ParseIP(defaultGw), Mask: defaultIPNet.Mask}
	routeInfo = RouteInfo{
		Dst: dstIP,
		Gw:  virtualGwIP,
	}
	if err := addRoutes(client.netlink, client.containerVethName, []RouteInfo{routeInfo}); err != nil {
		return err
	}

	// arp -s 169.254.1.1 e3:45:f4:ac:34:12 - add static arp entry for virtualgwip to hostveth interface mac
	log.Printf("[net] Adding static arp for IP address %v and MAC %v in Container namespace", virtualGwNet.String(), client.hostVethMac)
	err := client.netlink.AddOrRemoveStaticArp(netlink.ADD, client.containerVethName, virtualGwNet.IP, client.hostVethMac, false)
	if err != nil {
		return newErrorTransparentEndpointClient(err.Error())
	}
	return nil
}

func (client *TransparentEndpointClient) DeleteEndpoints(ep *endpoint) error {
	return nil
}
