// Copyright 2017 Microsoft. All rights reserved.
// MIT License

package network

import (
	"net"

	"github.com/Azure/azure-container-networking/log"
	"github.com/Azure/azure-container-networking/netio"
	"github.com/Azure/azure-container-networking/netlink"
	"github.com/Azure/azure-container-networking/network/networkutils"
	"github.com/Azure/azure-container-networking/network/ovsinfravnet"
	"github.com/Azure/azure-container-networking/network/snat"
	"github.com/Azure/azure-container-networking/ovsctl"
	"github.com/Azure/azure-container-networking/platform"
)

type OVSEndpointClient struct {
	bridgeName               string
	hostPrimaryIfName        string
	hostVethName             string
	hostPrimaryMac           string
	containerVethName        string
	containerMac             string
	snatClient               snat.Client
	infraVnetClient          ovsinfravnet.OVSInfraVnetClient
	vlanID                   int
	enableSnatOnHost         bool
	enableInfraVnet          bool
	allowInboundFromHostToNC bool
	allowInboundFromNCToHost bool
	enableSnatForDns         bool
	netlink                  netlink.NetlinkInterface
	netioshim                netio.NetIOInterface
	ovsctlClient             ovsctl.OvsInterface
	plClient                 platform.ExecClient
}

const (
	snatVethInterfacePrefix  = commonInterfacePrefix + "vint"
	infraVethInterfacePrefix = commonInterfacePrefix + "vifv"
)

func NewOVSEndpointClient(
	nw *network,
	epInfo *EndpointInfo,
	hostVethName string,
	containerVethName string,
	vlanid int,
	localIP string,
	nl netlink.NetlinkInterface,
	ovs ovsctl.OvsInterface,
	plc platform.ExecClient,
) *OVSEndpointClient {
	client := &OVSEndpointClient{
		bridgeName:               nw.extIf.BridgeName,
		hostPrimaryIfName:        nw.extIf.Name,
		hostVethName:             hostVethName,
		hostPrimaryMac:           nw.extIf.MacAddress.String(),
		containerVethName:        containerVethName,
		vlanID:                   vlanid,
		enableSnatOnHost:         epInfo.EnableSnatOnHost,
		enableInfraVnet:          epInfo.EnableInfraVnet,
		allowInboundFromHostToNC: epInfo.AllowInboundFromHostToNC,
		allowInboundFromNCToHost: epInfo.AllowInboundFromNCToHost,
		enableSnatForDns:         epInfo.EnableSnatForDns,
		netlink:                  nl,
		ovsctlClient:             ovs,
		plClient:                 plc,
		netioshim:                &netio.NetIO{},
	}

	NewInfraVnetClient(client, epInfo.Id[:7])
	client.NewSnatClient(nw.SnatBridgeIP, localIP, epInfo)

	return client
}

func (client *OVSEndpointClient) AddEndpoints(epInfo *EndpointInfo) error {
	epc := networkutils.NewNetworkUtils(client.netlink, client.plClient)
	if err := epc.CreateEndpoint(client.hostVethName, client.containerVethName, nil); err != nil {
		return err
	}

	containerIf, err := net.InterfaceByName(client.containerVethName)
	if err != nil {
		log.Printf("InterfaceByName returns error for ifname %v with error %v", client.containerVethName, err)
		return err
	}

	client.containerMac = containerIf.HardwareAddr.String()

	if err := client.AddSnatEndpoint(); err != nil {
		return err
	}

	if err := AddInfraVnetEndpoint(client); err != nil {
		return err
	}

	return nil
}

func (client *OVSEndpointClient) AddEndpointRules(epInfo *EndpointInfo) error {
	log.Printf("[ovs] Setting link %v master %v.", client.hostVethName, client.bridgeName)
	if err := client.ovsctlClient.AddPortOnOVSBridge(client.hostVethName, client.bridgeName, client.vlanID); err != nil {
		return err
	}

	log.Printf("[ovs] Get ovs port for interface %v.", client.hostVethName)
	containerOVSPort, err := client.ovsctlClient.GetOVSPortNumber(client.hostVethName)
	if err != nil {
		log.Printf("[ovs] Get ofport failed with error %v", err)
		return err
	}

	log.Printf("[ovs] Get ovs port for interface %v.", client.hostPrimaryIfName)
	hostPort, err := client.ovsctlClient.GetOVSPortNumber(client.hostPrimaryIfName)
	if err != nil {
		log.Printf("[ovs] Get ofport failed with error %v", err)
		return err
	}

	for _, ipAddr := range epInfo.IPAddresses {
		// Add Arp Reply Rules
		// Set Vlan id on arp request packet and forward it to table 1
		if err := client.ovsctlClient.AddFakeArpReply(client.bridgeName, ipAddr.IP); err != nil {
			return err
		}

		// IP SNAT Rule - Change src mac to VM Mac for packets coming from container host veth port.
		// This rule also checks if packets coming from right source ip based on the ovs port to prevent ip spoofing.
		// Otherwise it drops the packet.
		log.Printf("[ovs] Adding IP SNAT rule for egress traffic on %v.", containerOVSPort)
		if err := client.ovsctlClient.AddIPSnatRule(client.bridgeName, ipAddr.IP, client.vlanID, containerOVSPort, client.hostPrimaryMac, hostPort); err != nil {
			return err
		}

		// Add IP DNAT rule based on dst ip and vlanid - This rule changes the destination mac to corresponding container mac based on the ip and
		// forwards the packet to corresponding container hostveth port
		log.Printf("[ovs] Adding MAC DNAT rule for IP address %v on hostport %v, containerport: %v", ipAddr.IP.String(), hostPort, containerOVSPort)
		if err := client.ovsctlClient.AddMacDnatRule(client.bridgeName, hostPort, ipAddr.IP, client.containerMac, client.vlanID, containerOVSPort); err != nil {
			return err
		}
	}

	if err := AddInfraEndpointRules(client, epInfo.InfraVnetIP, hostPort); err != nil {
		return err
	}

	return client.AddSnatEndpointRules()
}

func (client *OVSEndpointClient) DeleteEndpointRules(ep *endpoint) {
	log.Printf("[ovs] Get ovs port for interface %v.", ep.HostIfName)
	containerPort, err := client.ovsctlClient.GetOVSPortNumber(client.hostVethName)
	if err != nil {
		log.Printf("[ovs] Get portnum failed with error %v", err)
	}

	log.Printf("[ovs] Get ovs port for interface %v.", client.hostPrimaryIfName)
	hostPort, err := client.ovsctlClient.GetOVSPortNumber(client.hostPrimaryIfName)
	if err != nil {
		log.Printf("[ovs] Get portnum failed with error %v", err)
	}

	// Delete IP SNAT
	log.Printf("[ovs] Deleting IP SNAT for port %v", containerPort)
	client.ovsctlClient.DeleteIPSnatRule(client.bridgeName, containerPort)

	// Delete Arp Reply Rules for container
	log.Printf("[ovs] Deleting ARP reply rule for ip %v vlanid %v for container port %v", ep.IPAddresses[0].IP.String(), ep.VlanID, containerPort)
	client.ovsctlClient.DeleteArpReplyRule(client.bridgeName, containerPort, ep.IPAddresses[0].IP, ep.VlanID)

	// Delete MAC address translation rule.
	log.Printf("[ovs] Deleting MAC DNAT rule for IP address %v and vlan %v.", ep.IPAddresses[0].IP.String(), ep.VlanID)
	client.ovsctlClient.DeleteMacDnatRule(client.bridgeName, hostPort, ep.IPAddresses[0].IP, ep.VlanID)

	// Delete port from ovs bridge
	log.Printf("[ovs] Deleting interface %v from bridge %v", client.hostVethName, client.bridgeName)
	if err := client.ovsctlClient.DeletePortFromOVS(client.bridgeName, client.hostVethName); err != nil {
		log.Printf("[ovs] Deletion of interface %v from bridge %v failed", client.hostVethName, client.bridgeName)
	}

	client.DeleteSnatEndpointRules()
	DeleteInfraVnetEndpointRules(client, ep, hostPort)
}

func (client *OVSEndpointClient) MoveEndpointsToContainerNS(epInfo *EndpointInfo, nsID uintptr) error {
	// Move the container interface to container's network namespace.
	log.Printf("[ovs] Setting link %v netns %v.", client.containerVethName, epInfo.NetNsPath)
	if err := client.netlink.SetLinkNetNs(client.containerVethName, nsID); err != nil {
		return err
	}

	if err := client.MoveSnatEndpointToContainerNS(epInfo.NetNsPath, nsID); err != nil {
		return err
	}

	return MoveInfraEndpointToContainerNS(client, epInfo.NetNsPath, nsID)
}

func (client *OVSEndpointClient) SetupContainerInterfaces(epInfo *EndpointInfo) error {
	nuc := networkutils.NewNetworkUtils(client.netlink, client.plClient)
	if err := nuc.SetupContainerInterface(client.containerVethName, epInfo.IfName); err != nil {
		return err
	}

	client.containerVethName = epInfo.IfName

	if err := client.SetupSnatContainerInterface(); err != nil {
		return err
	}

	return SetupInfraVnetContainerInterface(client)
}

func (client *OVSEndpointClient) ConfigureContainerInterfacesAndRoutes(epInfo *EndpointInfo) error {
	nuc := networkutils.NewNetworkUtils(client.netlink, client.plClient)
	if err := nuc.AssignIPToInterface(client.containerVethName, epInfo.IPAddresses); err != nil {
		return err
	}

	if err := client.ConfigureSnatContainerInterface(); err != nil {
		return err
	}

	if err := ConfigureInfraVnetContainerInterface(client, epInfo.InfraVnetIP); err != nil {
		return err
	}

	return addRoutes(client.netlink, client.netioshim, client.containerVethName, epInfo.Routes)
}

func (client *OVSEndpointClient) DeleteEndpoints(ep *endpoint) error {
	log.Printf("[ovs] Deleting veth pair %v %v.", ep.HostIfName, ep.IfName)
	err := client.netlink.DeleteLink(ep.HostIfName)
	if err != nil {
		log.Printf("[ovs] Failed to delete veth pair %v: %v.", ep.HostIfName, err)
		return err
	}

	if err := client.DeleteSnatEndpoint(); err != nil {
		return err
	}
	return DeleteInfraVnetEndpoint(client, ep.Id[:7])
}
