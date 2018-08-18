package ovssnat

import (
	"fmt"
	"net"
	"strings"

	"github.com/Azure/azure-container-networking/log"
	"github.com/Azure/azure-container-networking/netlink"
	"github.com/Azure/azure-container-networking/network/epcommon"
	"github.com/Azure/azure-container-networking/ovsctl"
	"github.com/Azure/azure-container-networking/platform"
)

const (
	azureSnatVeth0  = "azSnatveth0"
	azureSnatVeth1  = "azSnatveth1"
	azureSnatIfName = "eth1"
	SnatBridgeName  = "azSnatbr"
	ImdsIP          = "169.254.169.254/32"
)

type OVSSnatClient struct {
	hostSnatVethName       string
	containerSnatVethName  string
	localIP                string
	snatBridgeIP           string
	SkipAddressesFromBlock []string
}

func NewSnatClient(hostIfName string, contIfName string, localIP string, snatBridgeIP string, skipAddressesFromBlock []string) OVSSnatClient {
	log.Printf("Initialize new snat client")
	snatClient := OVSSnatClient{}
	snatClient.hostSnatVethName = hostIfName
	snatClient.containerSnatVethName = contIfName
	snatClient.localIP = localIP
	snatClient.snatBridgeIP = snatBridgeIP

	for _, address := range skipAddressesFromBlock {
		snatClient.SkipAddressesFromBlock = append(snatClient.SkipAddressesFromBlock, address)
	}

	log.Printf("Initialize new snat client %+v", snatClient)

	return snatClient
}

func (client *OVSSnatClient) CreateSnatEndpoint(bridgeName string) error {
	if err := CreateSnatBridge(client.snatBridgeIP, bridgeName); err != nil {
		log.Printf("creating snat bridge failed with error %v", err)
		return err
	}

	if err := AddMasqueradeRule(client.snatBridgeIP); err != nil {
		log.Printf("Adding snat rule failed with error %v", err)
		return err
	}

	if err := AddVlanDropRule(); err != nil {
		log.Printf("Adding vlan drop rule failed with error %v", err)
		return err
	}

	if err := epcommon.CreateEndpoint(client.hostSnatVethName, client.containerSnatVethName); err != nil {
		log.Printf("Creating Snat Endpoint failed with error %v", err)
		return err
	}

	return netlink.SetLinkMaster(client.hostSnatVethName, SnatBridgeName)
}

func (client *OVSSnatClient) AddPrivateIPBlockRule() error {
	if err := epcommon.AddOrDeletePrivateIPBlockRule(SnatBridgeName, client.SkipAddressesFromBlock, "A"); err != nil {
		log.Printf("AddPrivateIPBlockRule failed with error %v", err)
		return err
	}

	return nil
}

func (client *OVSSnatClient) MoveSnatEndpointToContainerNS(netnsPath string, nsID uintptr) error {
	log.Printf("[ovs] Setting link %v netns %v.", client.containerSnatVethName, netnsPath)
	return netlink.SetLinkNetNs(client.containerSnatVethName, nsID)
}

func (client *OVSSnatClient) SetupSnatContainerInterface() error {
	if err := epcommon.SetupContainerInterface(client.containerSnatVethName, azureSnatIfName); err != nil {
		return err
	}

	client.containerSnatVethName = azureSnatIfName

	return nil
}

func (client *OVSSnatClient) ConfigureSnatContainerInterface() error {
	log.Printf("[ovs] Adding IP address %v to link %v.", client.localIP, client.containerSnatVethName)
	ip, intIpAddr, _ := net.ParseCIDR(client.localIP)
	return netlink.AddIpAddress(client.containerSnatVethName, ip, intIpAddr)
}

func (client *OVSSnatClient) DeleteSnatEndpoint() error {
	log.Printf("[ovs] Deleting snat veth pair %v.", client.hostSnatVethName)
	err := netlink.DeleteLink(client.hostSnatVethName)
	if err != nil {
		log.Printf("[ovs] Failed to delete veth pair %v: %v.", client.hostSnatVethName, err)
		return err
	}

	return nil
}

func CreateSnatBridge(snatBridgeIP string, mainInterface string) error {
	_, err := net.InterfaceByName(SnatBridgeName)
	if err == nil {
		log.Printf("Snat Bridge already exists")
		return nil
	}

	log.Printf("[net] Creating Snat bridge %v.", SnatBridgeName)

	link := netlink.BridgeLink{
		LinkInfo: netlink.LinkInfo{
			Type: netlink.LINK_TYPE_BRIDGE,
			Name: SnatBridgeName,
		},
	}

	if err := netlink.AddLink(&link); err != nil {
		return err
	}

	_, err = net.InterfaceByName(azureSnatVeth0)
	if err == nil {
		log.Printf("Azure snat veth already exists")
		return nil
	}

	vethLink := netlink.VEthLink{
		LinkInfo: netlink.LinkInfo{
			Type: netlink.LINK_TYPE_VETH,
			Name: azureSnatVeth0,
		},
		PeerName: azureSnatVeth1,
	}

	err = netlink.AddLink(&vethLink)
	if err != nil {
		log.Printf("[net] Failed to create veth pair, err:%v.", err)
		return err
	}

	log.Printf("Assigning %v on snat bridge", snatBridgeIP)

	ip, addr, _ := net.ParseCIDR(snatBridgeIP)
	err = netlink.AddIpAddress(SnatBridgeName, ip, addr)
	if err != nil && !strings.Contains(strings.ToLower(err.Error()), "file exists") {
		log.Printf("[net] Failed to add IP address %v: %v.", addr, err)
		return err
	}

	if err := netlink.SetLinkState(SnatBridgeName, true); err != nil {
		return err
	}

	if err := netlink.SetLinkState(azureSnatVeth0, true); err != nil {
		return err
	}

	if err := netlink.SetLinkMaster(azureSnatVeth0, SnatBridgeName); err != nil {
		return err
	}

	if err := netlink.SetLinkState(azureSnatVeth1, true); err != nil {
		return err
	}

	if err := ovsctl.AddPortOnOVSBridge(azureSnatVeth1, mainInterface, 0); err != nil {
		return err
	}

	return nil
}

func DeleteSnatBridge(bridgeName string) error {
	cmd := "ebtables -t nat -D PREROUTING -p 802_1Q -j DROP"
	_, err := platform.ExecuteCommand(cmd)
	if err != nil {
		log.Printf("Deleting ebtable vlan drop rule failed with error %v", err)
	}

	if err = ovsctl.DeletePortFromOVS(bridgeName, azureSnatVeth1); err != nil {
		log.Printf("Deleting snatveth from ovs failed with error %v", err)
	}

	if err = netlink.DeleteLink(azureSnatVeth0); err != nil {
		log.Printf("Deleting host snatveth failed with error %v", err)
	}

	// Delete the bridge.
	err = netlink.DeleteLink(SnatBridgeName)
	if err != nil {
		log.Printf("[net] Failed to delete bridge %v, err:%v.", SnatBridgeName, err)
	}

	return err
}

func AddMasqueradeRule(snatBridgeIPWithPrefix string) error {
	_, ipNet, _ := net.ParseCIDR(snatBridgeIPWithPrefix)
	cmd := fmt.Sprintf("iptables -t nat -C POSTROUTING -s %v -j MASQUERADE", ipNet.String())
	_, err := platform.ExecuteCommand(cmd)
	if err == nil {
		log.Printf("iptable snat rule already exists")
		return nil
	}

	cmd = fmt.Sprintf("iptables -t nat -A POSTROUTING -s %v -j MASQUERADE", ipNet.String())
	log.Printf("Adding iptable snat rule %v", cmd)
	_, err = platform.ExecuteCommand(cmd)
	return err
}

func DeleteMasqueradeRule() error {
	snatIf, err := net.InterfaceByName(SnatBridgeName)
	if err != nil {
		return err
	}

	addrs, _ := snatIf.Addrs()
	for _, addr := range addrs {
		ipAddr, ipNet, err := net.ParseCIDR(addr.String())
		if err != nil {
			log.Printf("error %v", err)
			continue
		}

		if ipAddr.To4() != nil {
			cmd := fmt.Sprintf("iptables -t nat -D POSTROUTING -s %v -j MASQUERADE", ipNet.String())
			log.Printf("Deleting iptable snat rule %v", cmd)
			_, err = platform.ExecuteCommand(cmd)
			return err
		}
	}

	return nil
}

func AddVlanDropRule() error {
	cmd := "ebtables -t nat -L PREROUTING"
	out, err := platform.ExecuteCommand(cmd)
	if err != nil {
		log.Printf("Error while listing ebtable rules %v", err)
		return err
	}

	out = strings.TrimSpace(out)
	if strings.Contains(out, "-p 802_1Q -j DROP") {
		log.Printf("vlan drop rule already exists")
		return nil
	}

	cmd = "ebtables -t nat -A PREROUTING -p 802_1Q -j DROP"
	log.Printf("Adding ebtable rule to drop vlan traffic on snat bridge %v", cmd)
	_, err = platform.ExecuteCommand(cmd)
	return err
}
