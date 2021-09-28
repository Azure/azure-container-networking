// Copyright 2017 Microsoft. All rights reserved.
// MIT License

package ovssnat

import (
	"errors"
	"fmt"
	"net"
	"strings"

	"github.com/Azure/azure-container-networking/ebtables"
	"github.com/Azure/azure-container-networking/iptables"
	"github.com/Azure/azure-container-networking/log"
	"github.com/Azure/azure-container-networking/netlink"
	"github.com/Azure/azure-container-networking/network/epcommon"
	"github.com/Azure/azure-container-networking/ovsctl"
	"github.com/Azure/azure-container-networking/platform"
)

const (
	azureSnatVeth0      = "azSnatveth0"
	azureSnatVeth1      = "azSnatveth1"
	azureSnatIfName     = "eth1"
	SnatBridgeName      = "azSnatbr"
	ImdsIP              = "169.254.169.254/32"
	vlanDropDeleteRule  = "ebtables -t nat -D PREROUTING -p 802_1Q -j DROP"
	vlanDropAddRule     = "ebtables -t nat -A PREROUTING -p 802_1Q -j DROP"
	vlanDropMatch       = "-p 802_1Q -j DROP"
	l2PreroutingEntries = "ebtables -t nat -L PREROUTING"
)

var errorOVSSnatClient = errors.New("OVSSnatClient Error")

func newErrorOVSSnatClient(errStr string) error {
	return fmt.Errorf("%w : %s", errorOVSSnatClient, errStr)
}

type OVSSnatClient struct {
	hostSnatVethName       string
	hostPrimaryMac         string
	containerSnatVethName  string
	localIP                string
	snatBridgeIP           string
	SkipAddressesFromBlock []string
	netlink                netlink.NetlinkInterface
	ovsctlClient           ovsctl.OvsInterface
}

func NewSnatClient(hostIfName string,
	contIfName string,
	localIP string,
	snatBridgeIP string,
	hostPrimaryMac string,
	skipAddressesFromBlock []string,
	nl netlink.NetlinkInterface,
	ovsctlClient ovsctl.OvsInterface,
) OVSSnatClient {
	log.Printf("Initialize new snat client")
	snatClient := OVSSnatClient{
		hostSnatVethName:      hostIfName,
		containerSnatVethName: contIfName,
		localIP:               localIP,
		snatBridgeIP:          snatBridgeIP,
		hostPrimaryMac:        hostPrimaryMac,
		netlink:               nl,
		ovsctlClient:          ovsctlClient,
	}

	snatClient.SkipAddressesFromBlock = append(snatClient.SkipAddressesFromBlock, skipAddressesFromBlock...)

	log.Printf("Initialize new snat client %+v", snatClient)

	return snatClient
}

func (client *OVSSnatClient) CreateSnatEndpoint(bridgeName string) error {
	// Create linux Bridge for outbound connectivity
	if err := client.createSnatBridge(client.snatBridgeIP, client.hostPrimaryMac, bridgeName); err != nil {
		log.Printf("creating snat bridge failed with error %v", err)
		return err
	}

	// SNAT Rule to masquerade packets destined to non-vnet ip
	if err := client.addMasqueradeRule(client.snatBridgeIP); err != nil {
		log.Printf("Adding snat rule failed with error %v", err)
		return err
	}

	// Drop all vlan packets coming via linux bridge.
	if err := client.addVlanDropRule(); err != nil {
		log.Printf("Adding vlan drop rule failed with error %v", err)
		return err
	}

	epc := epcommon.NewEPCommon(client.netlink)
	// Create veth pair to tie one end to container and other end to linux bridge
	if err := epc.CreateEndpoint(client.hostSnatVethName, client.containerSnatVethName); err != nil {
		log.Printf("Creating Snat Endpoint failed with error %v", err)
		return newErrorOVSSnatClient(err.Error())
	}

	err := client.netlink.SetLinkMaster(client.hostSnatVethName, SnatBridgeName)
	if err != nil {
		return newErrorOVSSnatClient(err.Error())
	}
	return nil
}

// AllowIPAddressesOnSnatBridge adds iptables rules  that allows only specific Private IPs via linux bridge
func (client *OVSSnatClient) AllowIPAddressesOnSnatBridge() error {
	if err := epcommon.AllowIPAddresses(SnatBridgeName, client.SkipAddressesFromBlock, iptables.Insert); err != nil {
		log.Printf("AllowIPAddresses failed with error %v", err)
		return newErrorOVSSnatClient(err.Error())
	}

	return nil
}

// BlockIPAddressesOnSnatBridge adds iptables rules  that blocks all private IPs flowing via linux bridge
func (client *OVSSnatClient) BlockIPAddressesOnSnatBridge() error {
	if err := epcommon.BlockIPAddresses(SnatBridgeName, iptables.Append); err != nil {
		log.Printf("AllowIPAddresses failed with error %v", err)
		return newErrorOVSSnatClient(err.Error())
	}

	return nil
}

/**
	Move container veth inside container network namespace
**/
func (client *OVSSnatClient) MoveSnatEndpointToContainerNS(netnsPath string, nsID uintptr) error {
	log.Printf("[ovs] Setting link %v netns %v.", client.containerSnatVethName, netnsPath)
	err := client.netlink.SetLinkNetNs(client.containerSnatVethName, nsID)
	if err != nil {
		return newErrorOVSSnatClient(err.Error())
	}
	return nil
}

/**
	Configure Routes and setup name for container veth
**/
func (client *OVSSnatClient) SetupSnatContainerInterface() error {
	epc := epcommon.NewEPCommon(client.netlink)
	if err := epc.SetupContainerInterface(client.containerSnatVethName, azureSnatIfName); err != nil {
		return newErrorOVSSnatClient(err.Error())
	}

	client.containerSnatVethName = azureSnatIfName

	return nil
}

func getNCLocalAndGatewayIP(client *OVSSnatClient) (net.IP, net.IP) {
	bridgeIP, _, _ := net.ParseCIDR(client.snatBridgeIP)
	containerIP, _, _ := net.ParseCIDR(client.localIP)
	return bridgeIP, containerIP
}

/**
	This function adds iptables rules that allows only host to NC communication and not the other way
**/
func (client *OVSSnatClient) AllowInboundFromHostToNC() error {
	bridgeIP, containerIP := getNCLocalAndGatewayIP(client)

	// Create CNI Ouptut chain
	if err := iptables.CreateChain(iptables.V4, iptables.Filter, iptables.CNIOutputChain); err != nil {
		log.Printf("AllowInboundFromHostToNC: Creating %v failed with error: %v", iptables.CNIOutputChain, err)
		return newErrorOVSSnatClient(err.Error())
	}

	// Forward traffic from Ouptut chain to CNI Output chain
	if err := iptables.InsertIptableRule(iptables.V4, iptables.Filter, iptables.Output, "", iptables.CNIOutputChain); err != nil {
		log.Printf("AllowInboundFromHostToNC: Inserting forward rule to %v failed with error: %v", iptables.CNIOutputChain, err)
		return newErrorOVSSnatClient(err.Error())
	}

	// Allow connection from Host to NC
	matchCondition := fmt.Sprintf("-s %s -d %s", bridgeIP.String(), containerIP.String())
	err := iptables.InsertIptableRule(iptables.V4, iptables.Filter, iptables.CNIOutputChain, matchCondition, iptables.Accept)
	if err != nil {
		log.Printf("AllowInboundFromHostToNC: Inserting output rule failed: %v", err)
		return newErrorOVSSnatClient(err.Error())
	}

	// Create cniinput chain
	if err := iptables.CreateChain(iptables.V4, iptables.Filter, iptables.CNIInputChain); err != nil {
		log.Printf("AllowInboundFromHostToNC: Creating %v failed with error: %v", iptables.CNIInputChain, err)
		return newErrorOVSSnatClient(err.Error())
	}

	// Forward from Input to cniinput chain
	if err := iptables.InsertIptableRule(iptables.V4, iptables.Filter, iptables.Input, "", iptables.CNIInputChain); err != nil {
		log.Printf("AllowInboundFromHostToNC: Inserting forward rule to %v failed with error: %v", iptables.CNIInputChain, err)
		return newErrorOVSSnatClient(err.Error())
	}

	// Accept packets from NC only if established connection
	matchCondition = fmt.Sprintf(" -i %s -m state --state %s,%s", SnatBridgeName, iptables.Established, iptables.Related)
	err = iptables.InsertIptableRule(iptables.V4, iptables.Filter, iptables.CNIInputChain, matchCondition, iptables.Accept)
	if err != nil {
		log.Printf("AllowInboundFromHostToNC: Inserting input rule failed: %v", err)
		return newErrorOVSSnatClient(err.Error())
	}

	snatContainerVeth, _ := net.InterfaceByName(client.containerSnatVethName)

	// Add static arp entry for localIP to prevent arp going out of VM
	log.Printf("Adding static arp entry for ip %s mac %s", containerIP, snatContainerVeth.HardwareAddr.String())
	err = client.netlink.AddOrRemoveStaticArp(netlink.ADD, SnatBridgeName, containerIP, snatContainerVeth.HardwareAddr, false)
	if err != nil {
		log.Printf("AllowInboundFromHostToNC: Error adding static arp entry for ip %s mac %s: %v", containerIP, snatContainerVeth.HardwareAddr.String(), err)
		return newErrorOVSSnatClient(err.Error())
	}

	return nil
}

func (client *OVSSnatClient) DeleteInboundFromHostToNC() error {
	bridgeIP, containerIP := getNCLocalAndGatewayIP(client)

	// Delete allow connection from Host to NC
	matchCondition := fmt.Sprintf("-s %s -d %s", bridgeIP.String(), containerIP.String())
	err := iptables.DeleteIptableRule(iptables.V4, iptables.Filter, iptables.CNIOutputChain, matchCondition, iptables.Accept)
	if err != nil {
		log.Printf("DeleteInboundFromHostToNC: Error removing output rule %v", err)
	}

	// Remove static arp entry added for container local IP
	log.Printf("Removing static arp entry for ip %s ", containerIP)
	err = client.netlink.AddOrRemoveStaticArp(netlink.REMOVE, SnatBridgeName, containerIP, nil, false)
	if err != nil {
		log.Printf("AllowInboundFromHostToNC: Error removing static arp entry for ip %s: %v", containerIP, err)
	}

	return err
}

/**
	This function adds iptables rules that allows only NC to Host communication and not the other way
**/
func (client *OVSSnatClient) AllowInboundFromNCToHost() error {
	bridgeIP, containerIP := getNCLocalAndGatewayIP(client)

	// Create CNI Input chain
	if err := iptables.CreateChain(iptables.V4, iptables.Filter, iptables.CNIInputChain); err != nil {
		log.Printf("AllowInboundFromHostToNC: Creating %v failed with error: %v", iptables.CNIInputChain, err)
		return err
	}

	// Forward traffic from Input to cniinput chain
	if err := iptables.InsertIptableRule(iptables.V4, iptables.Filter, iptables.Input, "", iptables.CNIInputChain); err != nil {
		log.Printf("AllowInboundFromHostToNC: Inserting forward rule to %v failed with error: %v", iptables.CNIInputChain, err)
		return err
	}

	// Allow NC to Host connection
	matchCondition := fmt.Sprintf("-s %s -d %s", containerIP.String(), bridgeIP.String())
	err := iptables.InsertIptableRule(iptables.V4, iptables.Filter, iptables.CNIInputChain, matchCondition, iptables.Accept)
	if err != nil {
		log.Printf("AllowInboundFromHostToNC: Inserting output rule failed: %v", err)
		return err
	}

	// Create CNI output chain
	if err := iptables.CreateChain(iptables.V4, iptables.Filter, iptables.CNIOutputChain); err != nil {
		log.Printf("AllowInboundFromHostToNC: Creating %v failed with error: %v", iptables.CNIOutputChain, err)
		return err
	}

	// Forward traffic from Output to CNI Output chain
	if err := iptables.InsertIptableRule(iptables.V4, iptables.Filter, iptables.Output, "", iptables.CNIOutputChain); err != nil {
		log.Printf("AllowInboundFromHostToNC: Inserting forward rule to %v failed with error: %v", iptables.CNIOutputChain, err)
		return err
	}

	// Accept packets from Host only if established connection
	matchCondition = fmt.Sprintf(" -o %s -m state --state %s,%s", SnatBridgeName, iptables.Established, iptables.Related)
	err = iptables.InsertIptableRule(iptables.V4, iptables.Filter, iptables.CNIOutputChain, matchCondition, iptables.Accept)
	if err != nil {
		log.Printf("AllowInboundFromHostToNC: Inserting input rule failed: %v", err)
		return err
	}

	snatContainerVeth, _ := net.InterfaceByName(client.containerSnatVethName)

	// Add static arp entry for localIP to prevent arp going out of VM
	log.Printf("Adding static arp entry for ip %s mac %s", containerIP, snatContainerVeth.HardwareAddr.String())
	err = client.netlink.AddOrRemoveStaticArp(netlink.ADD, SnatBridgeName, containerIP, snatContainerVeth.HardwareAddr, false)
	if err != nil {
		log.Printf("AllowInboundFromNCToHost: Error adding static arp entry for ip %s mac %s: %v", containerIP, snatContainerVeth.HardwareAddr.String(), err)
	}

	return err
}

func (client *OVSSnatClient) DeleteInboundFromNCToHost() error {
	bridgeIP, containerIP := getNCLocalAndGatewayIP(client)

	// Delete allow NC to Host connection
	matchCondition := fmt.Sprintf("-s %s -d %s", containerIP.String(), bridgeIP.String())
	err := iptables.DeleteIptableRule(iptables.V4, iptables.Filter, iptables.CNIInputChain, matchCondition, iptables.Accept)
	if err != nil {
		log.Printf("DeleteInboundFromNCToHost: Error removing output rule %v", err)
	}

	// Remove static arp entry added for container local IP
	log.Printf("Removing static arp entry for ip %s ", containerIP)
	err = client.netlink.AddOrRemoveStaticArp(netlink.REMOVE, SnatBridgeName, containerIP, nil, false)
	if err != nil {
		log.Printf("DeleteInboundFromNCToHost: Error removing static arp entry for ip %s: %v", containerIP, err)
	}

	return err
}

/**
	Configures Local IP Address for container Veth
**/

func (client *OVSSnatClient) ConfigureSnatContainerInterface() error {
	log.Printf("[ovs] Adding IP address %v to link %v.", client.localIP, client.containerSnatVethName)
	ip, intIpAddr, _ := net.ParseCIDR(client.localIP)
	err := client.netlink.AddIPAddress(client.containerSnatVethName, ip, intIpAddr)
	if err != nil {
		return newErrorOVSSnatClient(err.Error())
	}
	return nil
}

func (client *OVSSnatClient) DeleteSnatEndpoint() error {
	log.Printf("[ovs] Deleting snat veth pair %v.", client.hostSnatVethName)
	err := client.netlink.DeleteLink(client.hostSnatVethName)
	if err != nil {
		log.Printf("[ovs] Failed to delete veth pair %v: %v.", client.hostSnatVethName, err)
		return newErrorOVSSnatClient(err.Error())
	}

	return nil
}

func (client *OVSSnatClient) setBridgeMac(hostPrimaryMac string) error {
	hwAddr, err := net.ParseMAC(hostPrimaryMac)
	if err != nil {
		log.Errorf("Error while parsing host primary mac: %s error:%+v", hostPrimaryMac, err)
		return err
	}

	if err = client.netlink.SetLinkAddress(SnatBridgeName, hwAddr); err != nil {
		log.Errorf("Error while setting macaddr on bridge: %s error:%+v", hwAddr.String(), err)
	}
	return err
}

func (client *OVSSnatClient) dropArpForSnatBridgeApipaRange(snatBridgeIP, azSnatVethIfName string) error {
	var err error
	_, ipCidr, _ := net.ParseCIDR(snatBridgeIP)
	if err = ebtables.SetArpDropRuleForIpCidr(ipCidr.String(), azSnatVethIfName); err != nil {
		log.Errorf("Error setting arp drop rule for snatbridge ip :%s", snatBridgeIP)
	}

	return err
}

/**
	This function creates linux bridge which will be used for outbound connectivity by NCs
**/
func (client *OVSSnatClient) createSnatBridge(snatBridgeIP string, hostPrimaryMac string, mainInterface string) error {
	_, err := net.InterfaceByName(SnatBridgeName)
	if err == nil {
		log.Printf("Snat Bridge already exists")
	} else {
		log.Printf("[net] Creating Snat bridge %v.", SnatBridgeName)

		link := netlink.BridgeLink{
			LinkInfo: netlink.LinkInfo{
				Type: netlink.LINK_TYPE_BRIDGE,
				Name: SnatBridgeName,
			},
		}

		if err := client.netlink.AddLink(&link); err != nil {
			return newErrorOVSSnatClient(err.Error())
		}
	}

	log.Printf("Setting snat bridge mac: %s", hostPrimaryMac)
	if err := client.setBridgeMac(hostPrimaryMac); err != nil {
		return err
	}

	log.Printf("Drop ARP for snat bridge ip: %s", snatBridgeIP)
	if err := client.dropArpForSnatBridgeApipaRange(snatBridgeIP, azureSnatVeth0); err != nil {
		return err
	}

	// Create a veth pair. One end of veth will be attached to ovs bridge and other end
	// of veth will be attached to linux bridge
	_, err = net.InterfaceByName(azureSnatVeth0)
	if err == nil {
		log.Printf("Azure snat veth already exists")
		return nil
	}

	if err := epcommon.DisableRAForInterface(SnatBridgeName); err != nil {
		return err
	}

	vethLink := netlink.VEthLink{
		LinkInfo: netlink.LinkInfo{
			Type: netlink.LINK_TYPE_VETH,
			Name: azureSnatVeth0,
		},
		PeerName: azureSnatVeth1,
	}

	err = client.netlink.AddLink(&vethLink)
	if err != nil {
		log.Printf("[net] Failed to create veth pair, err:%v.", err)
		return err
	}

	if err := epcommon.DisableRAForInterface(azureSnatVeth0); err != nil {
		return err
	}

	if err := epcommon.DisableRAForInterface(azureSnatVeth1); err != nil {
		return err
	}

	log.Printf("Assigning %v on snat bridge", snatBridgeIP)

	ip, addr, _ := net.ParseCIDR(snatBridgeIP)
	err = client.netlink.AddIPAddress(SnatBridgeName, ip, addr)
	if err != nil && !strings.Contains(strings.ToLower(err.Error()), "file exists") {
		log.Printf("[net] Failed to add IP address %v: %v.", addr, err)
		return newErrorOVSSnatClient(err.Error())
	}

	if err := client.netlink.SetLinkState(SnatBridgeName, true); err != nil {
		return newErrorOVSSnatClient(err.Error())
	}

	if err := client.netlink.SetLinkState(azureSnatVeth0, true); err != nil {
		return newErrorOVSSnatClient(err.Error())
	}

	if err := client.netlink.SetLinkMaster(azureSnatVeth0, SnatBridgeName); err != nil {
		return newErrorOVSSnatClient(err.Error())
	}

	if err := client.netlink.SetLinkState(azureSnatVeth1, true); err != nil {
		return newErrorOVSSnatClient(err.Error())
	}

	if err := client.ovsctlClient.AddPortOnOVSBridge(azureSnatVeth1, mainInterface, 0); err != nil {
		return newErrorOVSSnatClient(err.Error())
	}

	return nil
}

/**
	This function adds iptable rules that will snat all traffic that has source ip in apipa range and coming via linux bridge
**/
func (client *OVSSnatClient) addMasqueradeRule(snatBridgeIPWithPrefix string) error {
	_, ipNet, _ := net.ParseCIDR(snatBridgeIPWithPrefix)
	matchCondition := fmt.Sprintf("-s %s", ipNet.String())
	return iptables.InsertIptableRule(iptables.V4, iptables.Nat, iptables.Postrouting, matchCondition, iptables.Masquerade)
}

/**
	Drop all vlan traffic on linux bridge
**/
func (client *OVSSnatClient) addVlanDropRule() error {
	out, err := platform.ExecuteCommand(l2PreroutingEntries)
	if err != nil {
		log.Printf("Error while listing ebtable rules %v", err)
		return err
	}

	out = strings.TrimSpace(out)
	if strings.Contains(out, vlanDropMatch) {
		log.Printf("vlan drop rule already exists")
		return nil
	}

	log.Printf("Adding ebtable rule to drop vlan traffic on snat bridge %v", vlanDropAddRule)
	_, err = platform.ExecuteCommand(vlanDropAddRule)
	return err
}
