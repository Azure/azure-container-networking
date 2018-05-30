package network

import (
	"net"

	"github.com/Azure/azure-container-networking/ebtables"
	"github.com/Azure/azure-container-networking/log"
	"github.com/Azure/azure-container-networking/netlink"
)

type LinuxBridgeClient struct {
	bridgeName        string
	hostInterfaceName string
	mode              string
}

func NewLinuxBridgeClient(bridgeName string, hostInterfaceName string, mode string) *LinuxBridgeClient {
	client := &LinuxBridgeClient{
		bridgeName:        bridgeName,
		mode:              mode,
		hostInterfaceName: hostInterfaceName,
	}

	return client
}

func (client *LinuxBridgeClient) CreateBridge() error {
	log.Printf("[net] Creating bridge %v.", client.bridgeName)

	link := netlink.BridgeLink{
		LinkInfo: netlink.LinkInfo{
			Type: netlink.LINK_TYPE_BRIDGE,
			Name: client.bridgeName,
		},
	}

	return netlink.AddLink(&link)
}

func (client *LinuxBridgeClient) DeleteBridge() error {

	// Disconnect external interface from its bridge.
	err := netlink.SetLinkMaster(client.hostInterfaceName, "")
	if err != nil {
		log.Printf("[net] Failed to disconnect interface %v from bridge, err:%v.", client.hostInterfaceName, err)
	}

	// Delete the bridge.
	err = netlink.DeleteLink(client.bridgeName)
	if err != nil {
		log.Printf("[net] Failed to delete bridge %v, err:%v.", client.bridgeName, err)
	}

	return nil
}

func (client *LinuxBridgeClient) AddBridgeRules(extIf *externalInterface) error {

	hostIf, err := net.InterfaceByName(client.hostInterfaceName)
	if err != nil {
		return err
	}

	// Add SNAT rule to translate container egress traffic.
	log.Printf("[net] Adding SNAT rule for egress traffic on %v.", client.hostInterfaceName)
	if err := ebtables.SetSnatForInterface(client.hostInterfaceName, hostIf.HardwareAddr, ebtables.Append); err != nil {
		return err
	}

	// Add ARP reply rule for host primary IP address.
	// ARP requests for all IP addresses are forwarded to the SDN fabric, but fabric
	// doesn't respond to ARP requests from the VM for its own primary IP address.
	primary := extIf.IPAddresses[0].IP
	log.Printf("[net] Adding ARP reply rule for primary IP address %v.", primary)
	if err := ebtables.SetArpReply(primary, hostIf.HardwareAddr, ebtables.Append); err != nil {
		return err
	}

	// Add DNAT rule to forward ARP replies to container interfaces.
	log.Printf("[net] Adding DNAT rule for ingress ARP traffic on interface %v.", client.hostInterfaceName)
	if err := ebtables.SetDnatForArpReplies(client.hostInterfaceName, ebtables.Append); err != nil {
		return err
	}

	// Enable VEPA for host policy enforcement if necessary.
	if client.mode == opModeTunnel {
		log.Printf("[net] Enabling VEPA mode for %v.", client.hostInterfaceName)
		if err := ebtables.SetVepaMode(client.bridgeName, commonInterfacePrefix, virtualMacAddress, ebtables.Append); err != nil {
			return err
		}
	}

	return nil
}

func (client *LinuxBridgeClient) DeleteBridgeRules(extIf *externalInterface) {
	ebtables.SetVepaMode(client.bridgeName, commonInterfacePrefix, virtualMacAddress, ebtables.Delete)
	ebtables.SetDnatForArpReplies(extIf.Name, ebtables.Delete)
	ebtables.SetArpReply(extIf.IPAddresses[0].IP, extIf.MacAddress, ebtables.Delete)
	ebtables.SetSnatForInterface(extIf.Name, extIf.MacAddress, ebtables.Delete)
}

func (client *LinuxBridgeClient) SetBridgeMasterToHostInterface() error {
	return netlink.SetLinkMaster(client.hostInterfaceName, client.bridgeName)
}

func (client *LinuxBridgeClient) SetHairpinOnHostInterface(enable bool) error {
	return netlink.SetLinkHairpin(client.hostInterfaceName, enable)
}
