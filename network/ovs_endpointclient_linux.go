// Copyright 2017 Microsoft. All rights reserved.
// MIT License

package network

import (
	"net"

	"github.com/Azure/azure-container-networking/netio"
	"github.com/Azure/azure-container-networking/netlink"
	"github.com/Azure/azure-container-networking/network/networkutils"
	"github.com/Azure/azure-container-networking/network/ovsinfravnet"
	"github.com/Azure/azure-container-networking/network/snat"
	"github.com/Azure/azure-container-networking/ovsctl"
	"github.com/Azure/azure-container-networking/platform"
	"go.uber.org/zap"
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
		logger.Error("InterfaceByName returns error for ifname", zap.String("containerVethName", client.containerVethName), zap.Error(err))
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
	logger.Info("[ovs] Setting link master", zap.String("hostVethName", client.hostVethName), zap.String("bridgeName", client.bridgeName))
	if err := client.ovsctlClient.AddPortOnOVSBridge(client.hostVethName, client.bridgeName, client.vlanID); err != nil {
		return err
	}

	logger.Info("[ovs] Get ovs port for interface", zap.String("hostVethName", client.hostVethName))
	containerOVSPort, err := client.ovsctlClient.GetOVSPortNumber(client.hostVethName)
	if err != nil {
		logger.Error("[ovs] Get ofport failed with", zap.Error(err))
		return err
	}

	logger.Info("[ovs] Get ovs port for interface", zap.String("hostPrimaryIfName", client.hostPrimaryIfName))
	hostPort, err := client.ovsctlClient.GetOVSPortNumber(client.hostPrimaryIfName)
	if err != nil {
		logger.Error("[ovs] Get ofport failed with", zap.Error(err))
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
		logger.Info("[ovs] Adding IP SNAT rule for egress traffic on", zap.String("containerOVSPort", containerOVSPort))
		if err := client.ovsctlClient.AddIPSnatRule(client.bridgeName, ipAddr.IP, client.vlanID, containerOVSPort, client.hostPrimaryMac, hostPort); err != nil {
			return err
		}

		// Add IP DNAT rule based on dst ip and vlanid - This rule changes the destination mac to corresponding container mac based on the ip and
		// forwards the packet to corresponding container hostveth port
		logger.Info("[ovs] Adding MAC DNAT rule for IP address on hostport, containerport", zap.String("address", ipAddr.IP.String()),
			zap.String("hostPort", hostPort), zap.String("containerOVSPort", containerOVSPort))
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
	logger.Info("[ovs] Get ovs port for interface", zap.String("HostIfName", ep.HostIfName))
	containerPort, err := client.ovsctlClient.GetOVSPortNumber(client.hostVethName)
	if err != nil {
		logger.Error("[ovs] Get portnum failed with", zap.Error(err))
	}

	logger.Info("Get ovs port for interface", zap.String("hostPrimaryIfName", client.hostPrimaryIfName))
	hostPort, err := client.ovsctlClient.GetOVSPortNumber(client.hostPrimaryIfName)
	if err != nil {
		logger.Error("Get portnum failed with", zap.Error(err))
	}

	// Delete IP SNAT
	logger.Info("[ovs] Deleting IP SNAT for port", zap.String("containerPort", containerPort))
	client.ovsctlClient.DeleteIPSnatRule(client.bridgeName, containerPort)

	// Delete Arp Reply Rules for container
	logger.Info("[ovs] Deleting ARP reply rule for ip vlanid for container port", zap.String("address", ep.IPAddresses[0].IP.String()),
		zap.Any("VlanID", ep.VlanID), zap.String("containerPort", containerPort))
	client.ovsctlClient.DeleteArpReplyRule(client.bridgeName, containerPort, ep.IPAddresses[0].IP, ep.VlanID)

	// Delete MAC address translation rule.
	logger.Info("[ovs] Deleting MAC DNAT rule for IP address and vlan", zap.String("address", ep.IPAddresses[0].IP.String()),
		zap.Any("VlanID", ep.VlanID))
	client.ovsctlClient.DeleteMacDnatRule(client.bridgeName, hostPort, ep.IPAddresses[0].IP, ep.VlanID)

	// Delete port from ovs bridge
	logger.Info("[ovs] Deleting MAC DNAT rule for IP address and vlan", zap.String("address", ep.IPAddresses[0].IP.String()),
		zap.Any("VlanID", ep.VlanID))
	if err := client.ovsctlClient.DeletePortFromOVS(client.bridgeName, client.hostVethName); err != nil {
		logger.Error("[ovs] Deletion of interface from bridge failed", zap.String("hostVethName", client.hostVethName), zap.String("bridgeName", client.bridgeName))
	}

	client.DeleteSnatEndpointRules()
	DeleteInfraVnetEndpointRules(client, ep, hostPort)
}

func (client *OVSEndpointClient) MoveEndpointsToContainerNS(epInfo *EndpointInfo, nsID uintptr) error {
	// Move the container interface to container's network namespace.
	logger.Info("[ovs] Setting link netns", zap.String("containerVethName", client.containerVethName), zap.Any("NetNsPath", epInfo.NetNsPath))
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
	logger.Info("[ovs] Deleting veth pair", zap.String("HostIfName", ep.HostIfName), zap.String("IfName", ep.IfName))
	err := client.netlink.DeleteLink(ep.HostIfName)
	if err != nil {
		logger.Error("[ovs] Failed to delete veth pair", zap.String("HostIfName", ep.HostIfName), zap.Error(err))
		return err
	}

	if err := client.DeleteSnatEndpoint(); err != nil {
		return err
	}
	return DeleteInfraVnetEndpoint(client, ep.Id[:7])
}
