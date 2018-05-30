package network

import (
	"github.com/Azure/azure-container-networking/log"
	"github.com/Azure/azure-container-networking/ovsctl"
)

type OVSEndpointClient struct {
	bridgeName        string
	hostPrimaryIfName string
	hostVethName      string
	hostPrimaryMac    string
	containerMac      string
	vlanID            int
}

func NewOVSEndpointClient(
	bridgeName string,
	hostPrimaryIfName string,
	hostVethName string,
	hostPrimaryMac string,
	containerMac string,
	vlanid int) *OVSEndpointClient {

	client := &OVSEndpointClient{
		bridgeName:        bridgeName,
		hostPrimaryIfName: hostPrimaryIfName,
		hostVethName:      hostVethName,
		hostPrimaryMac:    hostPrimaryMac,
		containerMac:      containerMac,
		vlanID:            vlanid,
	}

	return client
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
	log.Printf("[net] Get ovs port for interface %v.", ep.HostIfName)
	containerPort, err := ovsctl.GetOVSPortNumber(client.hostVethName)
	if err != nil {
		log.Printf("[net] Get portnum failed with error %v", err)
	}

	log.Printf("[net] Get ovs port for interface %v.", client.hostPrimaryIfName)
	hostPort, err := ovsctl.GetOVSPortNumber(client.hostPrimaryIfName)
	if err != nil {
		log.Printf("[net] Get portnum failed with error %v", err)
	}

	// Delete IP SNAT
	log.Printf("[net] Deleting IP SNAT for port %v", containerPort)
	ovsctl.DeleteIPSnatRule(client.bridgeName, containerPort)

	// Delete Arp Reply Rules for container
	log.Printf("[net] Deleting ARP reply rule for ip %v vlanid %v for container port", ep.IPAddresses[0].IP.String(), ep.VlanID, containerPort)
	ovsctl.DeleteArpReplyRule(client.bridgeName, containerPort, ep.IPAddresses[0].IP, ep.VlanID)

	// Delete MAC address translation rule.
	log.Printf("[net] Deleting MAC DNAT rule for IP address %v and vlan %v.", ep.IPAddresses[0].IP.String(), ep.VlanID)
	ovsctl.DeleteMacDnatRule(client.bridgeName, hostPort, ep.IPAddresses[0].IP, ep.VlanID)

	// Delete port from ovs bridge
	log.Printf("[net] Deleting interface %v from bridge %v", client.hostVethName, client.bridgeName)
	ovsctl.DeletePortFromOVS(client.bridgeName, client.hostVethName)
}
