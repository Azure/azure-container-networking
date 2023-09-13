package network

import (
	"fmt"
	"net"

	"github.com/Azure/azure-container-networking/ebtables"
	"github.com/Azure/azure-container-networking/netio"
	"github.com/Azure/azure-container-networking/netlink"
	"github.com/Azure/azure-container-networking/network/networkutils"
	"github.com/Azure/azure-container-networking/platform"
	"go.uber.org/zap"
)

const (
	defaultV6VnetCidr = "2001:1234:5678:9abc::/64"
	defaultV6HostGw   = "fe80::1234:5678:9abc"
	defaultHostGwMac  = "12:34:56:78:9a:bc"
)

type LinuxBridgeEndpointClient struct {
	bridgeName        string
	hostPrimaryIfName string
	hostVethName      string
	containerVethName string
	hostPrimaryMac    net.HardwareAddr
	containerMac      net.HardwareAddr
	hostIPAddresses   []*net.IPNet
	mode              string
	netlink           netlink.NetlinkInterface
	plClient          platform.ExecClient
	netioshim         netio.NetIOInterface
	nuc               networkutils.NetworkUtils
}

func NewLinuxBridgeEndpointClient(
	extIf *externalInterface,
	hostVethName string,
	containerVethName string,
	mode string,
	nl netlink.NetlinkInterface,
	plc platform.ExecClient,
) *LinuxBridgeEndpointClient {
	client := &LinuxBridgeEndpointClient{
		bridgeName:        extIf.BridgeName,
		hostPrimaryIfName: extIf.Name,
		hostVethName:      hostVethName,
		containerVethName: containerVethName,
		hostPrimaryMac:    extIf.MacAddress,
		hostIPAddresses:   []*net.IPNet{},
		mode:              mode,
		netlink:           nl,
		plClient:          plc,
		netioshim:         &netio.NetIO{},
	}

	client.hostIPAddresses = append(client.hostIPAddresses, extIf.IPAddresses...)
	client.nuc = networkutils.NewNetworkUtils(nl, plc)
	return client
}

func (client *LinuxBridgeEndpointClient) AddEndpoints(epInfo *EndpointInfo) error {
	if err := client.nuc.CreateEndpoint(client.hostVethName, client.containerVethName, nil); err != nil {
		return err
	}

	containerIf, err := net.InterfaceByName(client.containerVethName)
	if err != nil {
		return err
	}

	client.containerMac = containerIf.HardwareAddr
	return nil
}

func (client *LinuxBridgeEndpointClient) AddEndpointRules(epInfo *EndpointInfo) error {
	var err error

	logger.Info("Setting link master", zap.String("hostVethName", client.hostVethName), zap.String("bridgeName", client.bridgeName))
	if err := client.netlink.SetLinkMaster(client.hostVethName, client.bridgeName); err != nil {
		return err
	}

	for _, ipAddr := range epInfo.IPAddresses {
		if ipAddr.IP.To4() != nil {
			// Add ARP reply rule.
			logger.Info("Adding ARP reply rule for IP address", zap.String("address", ipAddr.String()))
			if err = ebtables.SetArpReply(ipAddr.IP, client.getArpReplyAddress(client.containerMac), ebtables.Append); err != nil {
				return err
			}
		}

		// Add MAC address translation rule.
		logger.Info("Adding MAC DNAT rule for IP address", zap.String("address", ipAddr.String()))
		if err := ebtables.SetDnatForIPAddress(client.hostPrimaryIfName, ipAddr.IP, client.containerMac, ebtables.Append); err != nil {
			return err
		}

		if client.mode != opModeTunnel && ipAddr.IP.To4() != nil {
			logger.Info("Adding static arp for IP address and MAC in VM", zap.String("address", ipAddr.String()), zap.String("MAC", client.containerMac.String()))
			linkInfo := netlink.LinkInfo{
				Name:       client.bridgeName,
				IPAddr:     ipAddr.IP,
				MacAddress: client.containerMac,
			}

			if err := client.netlink.SetOrRemoveLinkAddress(linkInfo, netlink.ADD, netlink.NUD_PERMANENT); err != nil {
				logger.Info("Failed setting arp in vm with", zap.Error(err))
			}
		}
	}

	addRuleToRouteViaHost(epInfo)

	logger.Info("Setting hairpin for ", zap.String("hostveth", client.hostVethName))
	if err := client.netlink.SetLinkHairpin(client.hostVethName, true); err != nil {
		logger.Info("Setting up hairpin failed for interface error", zap.String("interfaceName", client.hostVethName), zap.Error(err))
		return err
	}

	return nil
}

func (client *LinuxBridgeEndpointClient) DeleteEndpointRules(ep *endpoint) {
	// Delete rules for IP addresses on the container interface.
	for _, ipAddr := range ep.IPAddresses {
		if ipAddr.IP.To4() != nil {
			// Delete ARP reply rule.
			logger.Info("Deleting ARP reply rule for IP address on", zap.String("address", ipAddr.String()), zap.String("id", ep.Id))
			err := ebtables.SetArpReply(ipAddr.IP, client.getArpReplyAddress(ep.MacAddress), ebtables.Delete)
			if err != nil {
				logger.Error("Failed to delete ARP reply rule for IP address", zap.String("address", ipAddr.String()), zap.Error(err))
			}
		}

		// Delete MAC address translation rule.
		logger.Info("Deleting MAC DNAT rule for IP address on", zap.String("address", ipAddr.String()), zap.String("id", ep.Id))
		err := ebtables.SetDnatForIPAddress(client.hostPrimaryIfName, ipAddr.IP, ep.MacAddress, ebtables.Delete)
		if err != nil {
			logger.Error("Failed to delete MAC DNAT rule for IP address", zap.String("address", ipAddr.String()), zap.Error(err))
		}

		if client.mode != opModeTunnel && ipAddr.IP.To4() != nil {
			logger.Info("Removing static arp for IP address and MAC from VM", zap.String("address", ipAddr.String()), zap.String("MAC", ep.MacAddress.String()))
			linkInfo := netlink.LinkInfo{
				Name:       client.bridgeName,
				IPAddr:     ipAddr.IP,
				MacAddress: ep.MacAddress,
			}
			err := client.netlink.SetOrRemoveLinkAddress(linkInfo, netlink.REMOVE, netlink.NUD_INCOMPLETE)
			if err != nil {
				logger.Error("Failed removing arp from vm with", zap.Error(err))
			}
		}
	}
}

// getArpReplyAddress returns the MAC address to use in ARP replies.
func (client *LinuxBridgeEndpointClient) getArpReplyAddress(epMacAddress net.HardwareAddr) net.HardwareAddr {
	var macAddress net.HardwareAddr

	if client.mode == opModeTunnel {
		// In tunnel mode, resolve all IP addresses to the virtual MAC address for hairpinning.
		macAddress, _ = net.ParseMAC(virtualMacAddress)
	} else {
		// Otherwise, resolve to actual MAC address.
		macAddress = epMacAddress
	}

	return macAddress
}

func (client *LinuxBridgeEndpointClient) MoveEndpointsToContainerNS(epInfo *EndpointInfo, nsID uintptr) error {
	// Move the container interface to container's network namespace.
	logger.Info("Setting link netns", zap.String("containerVethName", client.containerVethName), zap.String("NetNsPath", epInfo.NetNsPath))
	if err := client.netlink.SetLinkNetNs(client.containerVethName, nsID); err != nil {
		return newErrorLinuxBridgeClient(err.Error())
	}

	return nil
}

func (client *LinuxBridgeEndpointClient) SetupContainerInterfaces(epInfo *EndpointInfo) error {
	if err := client.nuc.SetupContainerInterface(client.containerVethName, epInfo.IfName); err != nil {
		return err
	}

	client.containerVethName = epInfo.IfName

	return nil
}

func (client *LinuxBridgeEndpointClient) ConfigureContainerInterfacesAndRoutes(epInfo *EndpointInfo) error {
	if err := client.nuc.AssignIPToInterface(client.containerVethName, epInfo.IPAddresses); err != nil {
		return err
	}

	if err := addRoutes(client.netlink, client.netioshim, client.containerVethName, epInfo.Routes); err != nil {
		return err
	}

	if err := client.setupIPV6Routes(epInfo); err != nil {
		return err
	}

	if err := client.setIPV6NeighEntry(epInfo); err != nil {
		return err
	}

	return nil
}

func (client *LinuxBridgeEndpointClient) DeleteEndpoints(ep *endpoint) error {
	logger.Info("Deleting veth pair", zap.String("hostIfName", ep.HostIfName), zap.String("interfaceName", ep.IfName))
	err := client.netlink.DeleteLink(ep.HostIfName)
	if err != nil {
		logger.Error("Failed to delete veth pair", zap.String("hostIfName", ep.HostIfName), zap.Error(err))
		return err
	}

	return nil
}

func addRuleToRouteViaHost(epInfo *EndpointInfo) error {
	for _, ipAddr := range epInfo.IPsToRouteViaHost {
		tableName := "broute"
		chainName := "BROUTING"
		rule := fmt.Sprintf("-p IPv4 --ip-dst %s -j redirect", ipAddr)

		// Check if EB rule exists
		logger.Info("Checking if EB rule already exists in table chain", zap.String("rule", rule), zap.String("tableName", tableName), zap.String("chainName", chainName))
		exists, err := ebtables.EbTableRuleExists(tableName, chainName, rule)
		if err != nil {
			logger.Error("Failed to check if EB table rule exists", zap.Error(err))
			return err
		}

		if exists {
			// EB rule already exists.
			logger.Info("EB rule already exists in table chain", zap.String("rule", rule), zap.String("tableName", tableName), zap.String("chainName", chainName))
		} else {
			// Add EB rule to route via host.
			logger.Info("Adding EB rule to route via host for IP", zap.Any("address", ipAddr))
			if err := ebtables.SetBrouteAccept(ipAddr, ebtables.Append); err != nil {
				logger.Error("Failed to add EB rule to route via host with", zap.Error(err))
				return err
			}
		}
	}

	return nil
}

func (client *LinuxBridgeEndpointClient) setupIPV6Routes(epInfo *EndpointInfo) error {
	if epInfo.IPV6Mode != "" {
		if epInfo.VnetCidrs == "" {
			epInfo.VnetCidrs = defaultV6VnetCidr
		}

		routes := []RouteInfo{}
		_, v6IpNet, _ := net.ParseCIDR(epInfo.VnetCidrs)
		v6Gw := net.ParseIP(defaultV6HostGw)
		vnetRoute := RouteInfo{
			Dst:      *v6IpNet,
			Gw:       v6Gw,
			Priority: 101,
		}

		var vmV6Route RouteInfo

		for _, ipAddr := range client.hostIPAddresses {
			if ipAddr.IP.To4() == nil {
				vmV6Route = RouteInfo{
					Dst:      *ipAddr,
					Priority: 100,
				}
			}
		}

		_, defIPNet, _ := net.ParseCIDR("::/0")
		defaultV6Route := RouteInfo{
			Dst: *defIPNet,
			Gw:  v6Gw,
		}

		routes = append(routes, vnetRoute)
		routes = append(routes, vmV6Route)
		routes = append(routes, defaultV6Route)

		logger.Info("Adding ipv6 routes in", zap.Any("container", routes))
		if err := addRoutes(client.netlink, client.netioshim, client.containerVethName, routes); err != nil {
			return nil
		}
	}

	return nil
}

func (client *LinuxBridgeEndpointClient) setIPV6NeighEntry(epInfo *EndpointInfo) error {
	if epInfo.IPV6Mode != "" {
		logger.Info("Add neigh entry for host gw ip")
		hardwareAddr, _ := net.ParseMAC(defaultHostGwMac)
		hostGwIp := net.ParseIP(defaultV6HostGw)
		linkInfo := netlink.LinkInfo{
			Name:       client.containerVethName,
			IPAddr:     hostGwIp,
			MacAddress: hardwareAddr,
		}
		if err := client.netlink.SetOrRemoveLinkAddress(linkInfo, netlink.ADD, netlink.NUD_PERMANENT); err != nil {
			logger.Error("Failed setting neigh entry in", zap.Any("container", err.Error()))
			return err
		}
	}

	return nil
}
