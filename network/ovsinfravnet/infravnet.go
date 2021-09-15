package ovsinfravnet

import (
	"errors"
	"fmt"
	"net"

	"github.com/Azure/azure-container-networking/log"

	"github.com/Azure/azure-container-networking/network/epcommon"
	"github.com/Azure/azure-container-networking/network/netlinkinterface"
	"github.com/Azure/azure-container-networking/ovsctl"
)

const (
	azureInfraIfName = "eth2"
)

var errorOVSInfraVnetClient = errors.New("OVSInfraVnetClient Error")

func newErrorOVSInfraVnetClient(errStr string) error {
	return fmt.Errorf("%w : %s", errorOVSInfraVnetClient, errStr)
}

type OVSInfraVnetClient struct {
	hostInfraVethName      string
	ContainerInfraVethName string
	containerInfraMac      string
	netlink                netlinkinterface.NetlinkInterface
}

func NewInfraVnetClient(hostIfName string, contIfName string, nl netlinkinterface.NetlinkInterface) OVSInfraVnetClient {
	infraVnetClient := OVSInfraVnetClient{
		hostInfraVethName:      hostIfName,
		ContainerInfraVethName: contIfName,
		netlink:                nl,
	}

	log.Printf("Initialize new infravnet client %+v", infraVnetClient)

	return infraVnetClient
}

func (client *OVSInfraVnetClient) CreateInfraVnetEndpoint(bridgeName string) error {
	epc := epcommon.NewEPCommon(client.netlink)
	if err := epc.CreateEndpoint(client.hostInfraVethName, client.ContainerInfraVethName); err != nil {
		log.Printf("Creating infraep failed with error %v", err)
		return err
	}

	log.Printf("[ovs] Adding port %v master %v.", client.hostInfraVethName, bridgeName)
	if err := ovsctl.AddPortOnOVSBridge(client.hostInfraVethName, bridgeName, 0); err != nil {
		log.Printf("Adding infraveth to ovsbr failed with error %v", err)
		return err
	}

	infraContainerIf, err := net.InterfaceByName(client.ContainerInfraVethName)
	if err != nil {
		log.Printf("InterfaceByName returns error for ifname %v with error %v", client.ContainerInfraVethName, err)
		return err
	}

	client.containerInfraMac = infraContainerIf.HardwareAddr.String()

	return nil
}

func (client *OVSInfraVnetClient) CreateInfraVnetRules(
	bridgeName string,
	infraIP net.IPNet,
	hostPrimaryMac string,
	hostPort string) error {

	infraContainerPort, err := ovsctl.GetOVSPortNumber(client.hostInfraVethName)
	if err != nil {
		log.Printf("[ovs] Get ofport failed with error %v", err)
		return err
	}

	// 0 signifies not to add vlan tag to this traffic
	if err := ovsctl.AddIpSnatRule(bridgeName, infraIP.IP, 0, infraContainerPort, hostPrimaryMac, hostPort); err != nil {
		log.Printf("[ovs] AddIpSnatRule failed with error %v", err)
		return err
	}

	// 0 signifies not to match traffic based on vlan tag
	if err := ovsctl.AddMacDnatRule(bridgeName, hostPort, infraIP.IP, client.containerInfraMac, 0, infraContainerPort); err != nil {
		log.Printf("[ovs] AddMacDnatRule failed with error %v", err)
		return err
	}

	return nil
}

func (client *OVSInfraVnetClient) MoveInfraEndpointToContainerNS(netnsPath string, nsID uintptr) error {
	log.Printf("[ovs] Setting link %v netns %v.", client.ContainerInfraVethName, netnsPath)
	err := client.netlink.SetLinkNetNs(client.ContainerInfraVethName, nsID)
	if err != nil {
		return newErrorOVSInfraVnetClient(err.Error())
	}
	return nil
}

func (client *OVSInfraVnetClient) SetupInfraVnetContainerInterface() error {
	epc := epcommon.NewEPCommon(client.netlink)
	if err := epc.SetupContainerInterface(client.ContainerInfraVethName, azureInfraIfName); err != nil {
		return newErrorOVSInfraVnetClient(err.Error())
	}

	client.ContainerInfraVethName = azureInfraIfName

	return nil
}

func (client *OVSInfraVnetClient) ConfigureInfraVnetContainerInterface(infraIP net.IPNet) error {
	log.Printf("[ovs] Adding IP address %v to link %v.", infraIP.String(), client.ContainerInfraVethName)
	err := client.netlink.AddIPAddress(client.ContainerInfraVethName, infraIP.IP, &infraIP)
	if err != nil {
		return newErrorOVSInfraVnetClient(err.Error())
	}
	return nil
}

func (client *OVSInfraVnetClient) DeleteInfraVnetRules(
	bridgeName string,
	infraIP net.IPNet,
	hostPort string) {

	log.Printf("[ovs] Deleting MAC DNAT rule for infravnet IP address %v", infraIP.IP.String())
	ovsctl.DeleteMacDnatRule(bridgeName, hostPort, infraIP.IP, 0)

	log.Printf("[ovs] Get ovs port for infravnet interface %v.", client.hostInfraVethName)
	infraContainerPort, err := ovsctl.GetOVSPortNumber(client.hostInfraVethName)
	if err != nil {
		log.Printf("[ovs] Get infravnet portnum failed with error %v", err)
	}

	log.Printf("[ovs] Deleting IP SNAT for infravnet port %v", infraContainerPort)
	ovsctl.DeleteIPSnatRule(bridgeName, infraContainerPort)

	log.Printf("[ovs] Deleting infravnet interface %v from bridge %v", client.hostInfraVethName, bridgeName)
	ovsctl.DeletePortFromOVS(bridgeName, client.hostInfraVethName)
}

func (client *OVSInfraVnetClient) DeleteInfraVnetEndpoint() error {
	log.Printf("[ovs] Deleting Infra veth pair %v.", client.hostInfraVethName)
	err := client.netlink.DeleteLink(client.hostInfraVethName)
	if err != nil {
		log.Printf("[ovs] Failed to delete veth pair %v: %v.", client.hostInfraVethName, err)
		return newErrorOVSInfraVnetClient(err.Error())
	}

	return nil
}
