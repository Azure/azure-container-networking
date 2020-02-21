package network

import (
	"fmt"
	"net"

	"github.com/Azure/azure-container-networking/ebtables"
	"github.com/Azure/azure-container-networking/log"
	"github.com/Azure/azure-container-networking/netlink"
	"github.com/Azure/azure-container-networking/network/epcommon"
)

type LinuxBridgeEndpointClient struct {
	bridgeName        string
	hostPrimaryIfName string
	hostVethName      string
	containerVethName string
	hostPrimaryMac    net.HardwareAddr
	containerMac      net.HardwareAddr
	mode              string
}

func NewLinuxBridgeEndpointClient(
	extIf *externalInterface,
	hostVethName string,
	containerVethName string,
	mode string,
) *LinuxBridgeEndpointClient {

	client := &LinuxBridgeEndpointClient{
		bridgeName:        extIf.BridgeName,
		hostPrimaryIfName: extIf.Name,
		hostVethName:      hostVethName,
		containerVethName: containerVethName,
		hostPrimaryMac:    extIf.MacAddress,
		mode:              mode,
	}

	return client
}

func (client *LinuxBridgeEndpointClient) AddEndpoints(epInfo *EndpointInfo) error {
	if err := epcommon.CreateEndpoint(client.hostVethName, client.containerVethName); err != nil {
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

	log.Printf("[net] Setting link %v master %v.", client.hostVethName, client.bridgeName)
	if err := netlink.SetLinkMaster(client.hostVethName, client.bridgeName); err != nil {
		return err
	}

	for _, ipAddr := range epInfo.IPAddresses {
		// Add ARP reply rule.
		log.Printf("[net] Adding ARP reply rule for IP address %v", ipAddr.String())
		if err = ebtables.SetArpReply(ipAddr.IP, client.getArpReplyAddress(client.containerMac), ebtables.Append); err != nil {
			return err
		}

		// Add MAC address translation rule.
		log.Printf("[net] Adding MAC DNAT rule for IP address %v", ipAddr.String())
		if err := ebtables.SetDnatForIPAddress(client.hostPrimaryIfName, ipAddr.IP, client.containerMac, ebtables.Append); err != nil {
			return err
		}

		if client.mode != opModeTunnel {
			log.Printf("[net] Adding static arp for IP address %v and MAC %v in VM", ipAddr.String(), client.containerMac.String())
			if err := netlink.AddOrRemoveStaticArp(netlink.ADD, client.bridgeName, ipAddr.IP, client.containerMac); err != nil {
				log.Printf("Failed setting arp in vm: %v", err)
			}
		}
	}

	addRuleToRouteViaHost(epInfo)

	log.Printf("[net] Setting hairpin for hostveth %v", client.hostVethName)
	if err := netlink.SetLinkHairpin(client.hostVethName, true); err != nil {
		log.Printf("Setting up hairpin failed for interface %v error %v", client.hostVethName, err)
		return err
	}

	return nil
}

func (client *LinuxBridgeEndpointClient) DeleteEndpointRules(ep *endpoint) {
	// Delete rules for IP addresses on the container interface.
	for _, ipAddr := range ep.IPAddresses {
		// Delete ARP reply rule.
		log.Printf("[net] Deleting ARP reply rule for IP address %v on %v.", ipAddr.String(), ep.Id)
		err := ebtables.SetArpReply(ipAddr.IP, client.getArpReplyAddress(ep.MacAddress), ebtables.Delete)
		if err != nil {
			log.Printf("[net] Failed to delete ARP reply rule for IP address %v: %v.", ipAddr.String(), err)
		}

		// Delete MAC address translation rule.
		log.Printf("[net] Deleting MAC DNAT rule for IP address %v on %v.", ipAddr.String(), ep.Id)
		err = ebtables.SetDnatForIPAddress(client.hostPrimaryIfName, ipAddr.IP, ep.MacAddress, ebtables.Delete)
		if err != nil {
			log.Printf("[net] Failed to delete MAC DNAT rule for IP address %v: %v.", ipAddr.String(), err)
		}

		if client.mode != opModeTunnel {
			log.Printf("[net] Removing static arp for IP address %v and MAC %v from VM", ipAddr.String(), ep.MacAddress.String())
			netlink.AddOrRemoveStaticArp(netlink.REMOVE, client.bridgeName, ipAddr.IP, ep.MacAddress)
			if err != nil {
				log.Printf("Failed removing arp from vm: %v", err)
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
	log.Printf("[net] Setting link %v netns %v.", client.containerVethName, epInfo.NetNsPath)
	if err := netlink.SetLinkNetNs(client.containerVethName, nsID); err != nil {
		return err
	}

	return nil
}

func (client *LinuxBridgeEndpointClient) SetupContainerInterfaces(epInfo *EndpointInfo) error {
	if err := epcommon.SetupContainerInterface(client.containerVethName, epInfo.IfName); err != nil {
		return err
	}

	client.containerVethName = epInfo.IfName

	return nil
}

func (client *LinuxBridgeEndpointClient) ConfigureContainerInterfacesAndRoutes(epInfo *EndpointInfo) error {
	if err := epcommon.AssignIPToInterface(client.containerVethName, epInfo.IPAddresses); err != nil {
		return err
	}

	if err := addRoutes(client.containerVethName, epInfo.Routes); err != nil {
		return err
	}

	return nil
}

func (client *LinuxBridgeEndpointClient) DeleteEndpoints(ep *endpoint) error {
	log.Printf("[net] Deleting veth pair %v %v.", ep.HostIfName, ep.IfName)
	err := netlink.DeleteLink(ep.HostIfName)
	if err != nil {
		log.Printf("[net] Failed to delete veth pair %v: %v.", ep.HostIfName, err)
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
		log.Printf("[net] Checking if EB rule %s already exists in table %s chain %s", rule, tableName, chainName)
		exists, err := ebtables.EbTableRuleExists(tableName, chainName, rule)
		if err != nil {
			log.Printf("[net] Failed to check if EB table rule exists: %v", err)
			return err
		}

		if exists {
			// EB rule already exists.
			log.Printf("[net] EB rule %s already exists in table %s chain %s.", rule, tableName, chainName)
		} else {
			// Add EB rule to route via host.
			log.Printf("[net] Adding EB rule to route via host for IP address %v", ipAddr)
			if err := ebtables.SetBrouteAccept(ipAddr, ebtables.Append); err != nil {
				log.Printf("[net] Failed to add EB rule to route via host: %v", err)
				return err
			}
		}
	}

	return nil
}
