package network

import (
	"fmt"
	"net"

	"github.com/Azure/azure-container-networking/log"
	"github.com/Azure/azure-container-networking/netlink"
	"github.com/Azure/azure-container-networking/ovsctl"
)

type OVSEndpointClient struct {
	bridgeName             string
	hostPrimaryIfName      string
	hostVethName           string
	hostPrimaryMac         string
	containerVethName      string
	containerMac           string
	snatVethName           string
	snatBridgeIP           string
	localIP                string
	hostInfraVethName      string
	containerInfraVethName string
	containerInfraMac      string
	vlanID                 int
	enableSnatOnHost       bool
	enableInfraVnet        bool
}

const (
	snatVethInterfacePrefix  = commonInterfacePrefix + "vint"
	infraVethInterfacePrefix = commonInterfacePrefix + "vifv"
	azureSnatIfName          = "eth1"
	azureInfraIfName         = "eth2"
)

func NewOVSEndpointClient(
	extIf *externalInterface,
	epInfo *EndpointInfo,
	hostVethName string,
	containerVethName string,
	vlanid int,
) *OVSEndpointClient {

	client := &OVSEndpointClient{
		bridgeName:        extIf.BridgeName,
		hostPrimaryIfName: extIf.Name,
		hostVethName:      hostVethName,
		hostPrimaryMac:    extIf.MacAddress.String(),
		containerVethName: containerVethName,
		vlanID:            vlanid,
		enableSnatOnHost:  epInfo.EnableSnatOnHost,
		enableInfraVnet:   epInfo.EnableInfraVnet,
	}

	if _, ok := epInfo.Data[LocalIPKey]; ok {
		client.localIP = epInfo.Data[LocalIPKey].(string)
	}

	if _, ok := epInfo.Data[SnatBridgeIPKey]; ok {
		client.snatBridgeIP = epInfo.Data[SnatBridgeIPKey].(string)
	}

	return client
}

func (client *OVSEndpointClient) AddEndpoints(epInfo *EndpointInfo) error {
	if err := createEndpoint(client.hostVethName, client.containerVethName); err != nil {
		return err
	}

	containerIf, err := net.InterfaceByName(client.containerVethName)
	if err != nil {
		log.Printf("InterfaceByName returns error for ifname %v with error %v", client.containerVethName, err)
		return err
	}

	client.containerMac = containerIf.HardwareAddr.String()

	if client.enableSnatOnHost {
		if err := createSnatBridge(client.snatBridgeIP, client.bridgeName); err != nil {
			log.Printf("creating snat bridge failed with error %v", err)
			return err
		}

		if err := addMasqueradeRule(client.snatBridgeIP); err != nil {
			log.Printf("Adding snat rule failed with error %v", err)
			return err
		}

		if err := addVlanDropRule(); err != nil {
			log.Printf("Adding vlan drop rule failed with error %v", err)
			return err
		}

		if err := addStaticRoute(imdsIP, client.bridgeName); err != nil {
			log.Printf("Adding imds static route failed with error %v", err)
			return err
		}

		hostIfName := fmt.Sprintf("%s%s", snatVethInterfacePrefix, epInfo.Id[:7])
		contIfName := fmt.Sprintf("%s%s-2", snatVethInterfacePrefix, epInfo.Id[:7])

		if err := createEndpoint(hostIfName, contIfName); err != nil {
			return err
		}

		if err := netlink.SetLinkMaster(hostIfName, snatBridgeName); err != nil {
			return err
		}

		client.snatVethName = contIfName
	}

	if client.enableInfraVnet {
		hostIfName := fmt.Sprintf("%s%s", infraVethInterfacePrefix, epInfo.Id[:7])
		contIfName := fmt.Sprintf("%s%s-2", infraVethInterfacePrefix, epInfo.Id[:7])

		if err := createEndpoint(hostIfName, contIfName); err != nil {
			return err
		}

		log.Printf("[ovs] Setting link %v master %v.", hostIfName, client.bridgeName)
		if err := ovsctl.AddPortOnOVSBridge(hostIfName, client.bridgeName, 0); err != nil {
			return err
		}

		client.hostInfraVethName = hostIfName
		client.containerInfraVethName = contIfName

		infraContainerIf, err := net.InterfaceByName(client.containerInfraVethName)
		if err != nil {
			log.Printf("InterfaceByName returns error for ifname %v with error %v", client.containerInfraVethName, err)
			return err
		}

		client.containerInfraMac = infraContainerIf.HardwareAddr.String()
	}

	return nil
}

func (client *OVSEndpointClient) AddEndpointRules(epInfo *EndpointInfo) error {
	log.Printf("[ovs] Setting link %v master %v.", client.hostVethName, client.bridgeName)
	if err := ovsctl.AddPortOnOVSBridge(client.hostVethName, client.bridgeName, client.vlanID); err != nil {
		return err
	}

	log.Printf("[ovs] Get ovs port for interface %v.", client.hostVethName)
	containerPort, err := ovsctl.GetOVSPortNumber(client.hostVethName)
	if err != nil {
		log.Printf("[ovs] Get ofport failed with error %v", err)
		return err
	}

	log.Printf("[ovs] Get ovs port for interface %v.", client.hostPrimaryIfName)
	hostPort, err := ovsctl.GetOVSPortNumber(client.hostPrimaryIfName)
	if err != nil {
		log.Printf("[ovs] Get ofport failed with error %v", err)
		return err
	}

	// IP SNAT Rule
	log.Printf("[ovs] Adding IP SNAT rule for egress traffic on %v.", containerPort)
	if err := ovsctl.AddIpSnatRule(client.bridgeName, containerPort, client.hostPrimaryMac); err != nil {
		return err
	}

	if client.enableInfraVnet {
		infraContainerPort, err := ovsctl.GetOVSPortNumber(client.hostInfraVethName)
		if err != nil {
			log.Printf("[ovs] Get ofport failed with error %v", err)
			return err
		}

		if err := ovsctl.AddIpSnatRule(client.bridgeName, infraContainerPort, client.hostPrimaryMac); err != nil {
			return err
		}

		if err := ovsctl.AddMacDnatRule(client.bridgeName, hostPort, epInfo.InfraVnetIP.IP, client.containerInfraMac, 0); err != nil {
			return err
		}
	}

	for _, ipAddr := range epInfo.IPAddresses {
		// Add Arp Reply Rules
		// Set Vlan id on arp request packet and forward it to table 1
		if err := ovsctl.AddFakeArpReply(client.bridgeName, ipAddr.IP); err != nil {
			return err
		}

		// Add IP DNAT rule based on dst ip and vlanid
		log.Printf("[ovs] Adding MAC DNAT rule for IP address %v on %v.", ipAddr.IP.String(), hostPort)
		if err := ovsctl.AddMacDnatRule(client.bridgeName, hostPort, ipAddr.IP, client.containerMac, client.vlanID); err != nil {
			return err
		}
	}

	return nil
}

func (client *OVSEndpointClient) DeleteEndpointRules(ep *endpoint) {
	log.Printf("[ovs] Get ovs port for interface %v.", ep.HostIfName)
	containerPort, err := ovsctl.GetOVSPortNumber(client.hostVethName)
	if err != nil {
		log.Printf("[ovs] Get portnum failed with error %v", err)
	}

	log.Printf("[ovs] Get ovs port for interface %v.", client.hostPrimaryIfName)
	hostPort, err := ovsctl.GetOVSPortNumber(client.hostPrimaryIfName)
	if err != nil {
		log.Printf("[ovs] Get portnum failed with error %v", err)
	}

	// Delete IP SNAT
	log.Printf("[ovs] Deleting IP SNAT for port %v", containerPort)
	ovsctl.DeleteIPSnatRule(client.bridgeName, containerPort)

	// Delete Arp Reply Rules for container
	log.Printf("[ovs] Deleting ARP reply rule for ip %v vlanid %v for container port %v", ep.IPAddresses[0].IP.String(), ep.VlanID, containerPort)
	ovsctl.DeleteArpReplyRule(client.bridgeName, containerPort, ep.IPAddresses[0].IP, ep.VlanID)

	// Delete MAC address translation rule.
	log.Printf("[ovs] Deleting MAC DNAT rule for IP address %v and vlan %v.", ep.IPAddresses[0].IP.String(), ep.VlanID)
	ovsctl.DeleteMacDnatRule(client.bridgeName, hostPort, ep.IPAddresses[0].IP, ep.VlanID)

	// Delete port from ovs bridge
	log.Printf("[ovs] Deleting interface %v from bridge %v", client.hostVethName, client.bridgeName)
	ovsctl.DeletePortFromOVS(client.bridgeName, client.hostVethName)

	if client.enableInfraVnet {
		hostInfraVethName := fmt.Sprintf("%s%s", infraVethInterfacePrefix, ep.Id[:7])

		log.Printf("[ovs] Deleting MAC DNAT rule for infravnet IP address %v", ep.IPAddresses[0].IP.String())
		ovsctl.DeleteMacDnatRule(client.bridgeName, hostPort, ep.InfraVnetIP.IP, 0)

		log.Printf("[ovs] Get ovs port for infravnet interface %v.", hostInfraVethName)
		infraContainerPort, err := ovsctl.GetOVSPortNumber(hostInfraVethName)
		if err != nil {
			log.Printf("[ovs] Get infravnet portnum failed with error %v", err)
		}

		log.Printf("[ovs] Deleting IP SNAT for infravnet port %v", infraContainerPort)
		ovsctl.DeleteIPSnatRule(client.bridgeName, infraContainerPort)

		log.Printf("[ovs] Deleting infravnet interface %v from bridge %v", hostInfraVethName, client.bridgeName)
		ovsctl.DeletePortFromOVS(client.bridgeName, hostInfraVethName)
	}
}

func (client *OVSEndpointClient) MoveEndpointsToContainerNS(epInfo *EndpointInfo, nsID uintptr) error {
	// Move the container interface to container's network namespace.
	log.Printf("[ovs] Setting link %v netns %v.", client.containerVethName, epInfo.NetNsPath)
	if err := netlink.SetLinkNetNs(client.containerVethName, nsID); err != nil {
		return err
	}

	if client.enableSnatOnHost {
		log.Printf("[ovs] Setting link %v netns %v.", client.snatVethName, epInfo.NetNsPath)
		if err := netlink.SetLinkNetNs(client.snatVethName, nsID); err != nil {
			return err
		}
	}

	if client.enableInfraVnet {
		log.Printf("[ovs] Setting link %v netns %v.", client.containerInfraVethName, epInfo.NetNsPath)
		if err := netlink.SetLinkNetNs(client.containerInfraVethName, nsID); err != nil {
			return err
		}
	}

	return nil
}

func (client *OVSEndpointClient) SetupContainerInterfaces(epInfo *EndpointInfo) error {

	if err := setupContainerInterface(client.containerVethName, epInfo.IfName); err != nil {
		return err
	}

	client.containerVethName = epInfo.IfName

	if client.enableSnatOnHost {
		if err := setupContainerInterface(client.snatVethName, azureSnatIfName); err != nil {
			return err
		}
		client.snatVethName = azureSnatIfName
	}

	if client.enableInfraVnet {
		if err := setupContainerInterface(client.containerInfraVethName, azureInfraIfName); err != nil {
			return err
		}
		client.containerInfraVethName = azureInfraIfName
	}

	return nil
}

func (client *OVSEndpointClient) ConfigureContainerInterfacesAndRoutes(epInfo *EndpointInfo) error {
	if err := assignIPToInterface(client.containerVethName, epInfo.IPAddresses); err != nil {
		return err
	}

	if client.enableSnatOnHost {
		log.Printf("[ovs] Adding IP address %v to link %v.", client.localIP, client.snatVethName)
		ip, intIpAddr, _ := net.ParseCIDR(client.localIP)
		if err := netlink.AddIpAddress(client.snatVethName, ip, intIpAddr); err != nil {
			return err
		}
	}

	if client.enableInfraVnet {
		log.Printf("[ovs] Adding IP address %v to link %v.", epInfo.InfraVnetIP.String(), client.containerInfraVethName)
		if err := netlink.AddIpAddress(client.containerInfraVethName, epInfo.InfraVnetIP.IP, &epInfo.InfraVnetIP); err != nil {
			return err
		}
	}

	if err := addRoutes(client.containerVethName, epInfo.Routes); err != nil {
		return err
	}

	return nil
}

func (client *OVSEndpointClient) DeleteEndpoints(ep *endpoint) error {
	log.Printf("[ovs] Deleting veth pair %v %v.", ep.HostIfName, ep.IfName)
	err := netlink.DeleteLink(ep.HostIfName)
	if err != nil {
		log.Printf("[ovs] Failed to delete veth pair %v: %v.", ep.HostIfName, err)
		return err
	}

	if client.enableSnatOnHost {
		hostIfName := fmt.Sprintf("%s%s", snatVethInterfacePrefix, ep.Id[:7])
		log.Printf("[ovs] Deleting snat veth pair %v.", hostIfName)
		err = netlink.DeleteLink(hostIfName)
		if err != nil {
			log.Printf("[ovs] Failed to delete veth pair %v: %v.", hostIfName, err)
			return err
		}
	}

	if client.enableInfraVnet {
		hostIfName := fmt.Sprintf("%s%s", infraVethInterfacePrefix, ep.Id[:7])
		log.Printf("[ovs] Deleting Infra veth pair %v.", hostIfName)
		err = netlink.DeleteLink(hostIfName)
		if err != nil {
			log.Printf("[ovs] Failed to delete veth pair %v: %v.", hostIfName, err)
			return err
		}
	}

	return nil
}
