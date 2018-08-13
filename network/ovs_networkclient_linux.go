package network

import (
	"bytes"
	"fmt"
	"net"
	"os"
	"strings"

	"github.com/Azure/azure-container-networking/log"
	"github.com/Azure/azure-container-networking/netlink"
	"github.com/Azure/azure-container-networking/ovsctl"
	"github.com/Azure/azure-container-networking/platform"
)

type OVSNetworkClient struct {
	bridgeName        string
	hostInterfaceName string
	snatBridgeIP      string
	enableSnatOnHost  bool
}

const (
	azureSnatVeth0 = "azSnatveth0"
	azureSnatVeth1 = "azSnatveth1"
	snatBridgeName = "azSnatbr"
	imdsIP         = "169.254.169.254/32"
	ovsConfigFile  = "/etc/default/openvswitch-switch"
	ovsOpt         = "OVS_CTL_OPTS='--delete-bridges'"
)

func getPrivateIPSpace() []string {
	privateIPAddresses := []string{"10.0.0.0/8", "172.16.0.0/12", "192.168.0.0/16"}
	return privateIPAddresses
}

func getFilterChains() []string {
	chains := []string{"FORWARD", "INPUT", "OUTPUT"}
	return chains
}

func getFilterchainTarget() []string {
	actions := []string{"ACCEPT", "DROP"}
	return actions
}

func updateOVSConfig(option string) error {
	f, err := os.OpenFile(ovsConfigFile, os.O_APPEND|os.O_RDWR, 0666)
	if err != nil {
		log.Printf("Error while opening ovs config %v", err)
		return err
	}

	defer f.Close()

	buf := new(bytes.Buffer)
	buf.ReadFrom(f)
	contents := buf.String()

	conSplit := strings.Split(contents, "\n")
	for _, existingOption := range conSplit {
		if option == existingOption {
			log.Printf("Not updating ovs config. Found option already written")
			return nil
		}
	}

	log.Printf("writing ovsconfig option %v", option)

	if _, err = f.WriteString(option); err != nil {
		log.Printf("Error while writing ovs config %v", err)
		return err
	}

	return nil
}

func NewOVSClient(bridgeName, hostInterfaceName, snatBridgeIP string, enableSnatOnHost bool) *OVSNetworkClient {
	ovsClient := &OVSNetworkClient{
		bridgeName:        bridgeName,
		hostInterfaceName: hostInterfaceName,
		snatBridgeIP:      snatBridgeIP,
		enableSnatOnHost:  enableSnatOnHost,
	}

	return ovsClient
}

func (client *OVSNetworkClient) CreateBridge() error {
	if err := ovsctl.CreateOVSBridge(client.bridgeName); err != nil {
		return err
	}

	if err := updateOVSConfig(ovsOpt); err != nil {
		return err
	}

	if client.enableSnatOnHost {
		if err := createSnatBridge(client.snatBridgeIP, client.bridgeName); err != nil {
			log.Printf("[net] Creating snat bridge failed with erro %v", err)
			return err
		}

		if err := addOrDeletePrivateIPBlockRule("A"); err != nil {
			log.Printf("addPrivateIPBlockRule failed with error %v", err)
			return err
		}

		if err := addMasqueradeRule(client.snatBridgeIP); err != nil {
			return err
		}

		return addVlanDropRule()
	}

	return nil
}

func addVlanDropRule() error {
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

func addMasqueradeRule(snatBridgeIPWithPrefix string) error {
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

func deleteMasqueradeRule(interfaceName string) error {
	snatIf, err := net.InterfaceByName(interfaceName)
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

func (client *OVSNetworkClient) DeleteBridge() error {
	if err := ovsctl.DeleteOVSBridge(client.bridgeName); err != nil {
		log.Printf("Deleting ovs bridge failed with error %v", err)
		return err
	}

	if client.enableSnatOnHost {
		deleteMasqueradeRule(snatBridgeName)

		cmd := "ebtables -t nat -D PREROUTING -p 802_1Q -j DROP"
		_, err := platform.ExecuteCommand(cmd)
		if err != nil {
			log.Printf("Deleting ebtable vlan drop rule failed with error %v", err)
		}

		if err := addOrDeletePrivateIPBlockRule("D"); err != nil {
			log.Printf("Deleting PrivateIP Block rules failed with error %v", err)
		}

		if err := ovsctl.DeletePortFromOVS(client.bridgeName, azureSnatVeth1); err != nil {
			return err
		}

		if err := DeleteSnatBridge(); err != nil {
			log.Printf("Deleting snat bridge failed with error %v", err)
			return err
		}

		return netlink.DeleteLink(azureSnatVeth0)
	}

	return nil
}

func createSnatBridge(snatBridgeIP string, mainInterface string) error {
	_, err := net.InterfaceByName(snatBridgeName)
	if err == nil {
		log.Printf("Snat Bridge already exists")
		return nil
	}

	log.Printf("[net] Creating Snat bridge %v.", snatBridgeName)

	link := netlink.BridgeLink{
		LinkInfo: netlink.LinkInfo{
			Type: netlink.LINK_TYPE_BRIDGE,
			Name: snatBridgeName,
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
	err = netlink.AddIpAddress(snatBridgeName, ip, addr)
	if err != nil && !strings.Contains(strings.ToLower(err.Error()), "file exists") {
		log.Printf("[net] Failed to add IP address %v: %v.", addr, err)
		return err
	}

	if err := netlink.SetLinkState(snatBridgeName, true); err != nil {
		return err
	}

	if err := netlink.SetLinkState(azureSnatVeth0, true); err != nil {
		return err
	}

	if err := netlink.SetLinkMaster(azureSnatVeth0, snatBridgeName); err != nil {
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

func addOrDeleteFilterRule(action string, ipAddress string, chainName string, target string) error {
	option := "i"

	if chainName == "OUTPUT" {
		option = "o"
	}

	if action != "D" {
		cmd := fmt.Sprintf("iptables -t filter -C %v -%v %v -d %v -j %v", chainName, option, snatBridgeName, ipAddress, target)
		_, err := platform.ExecuteCommand(cmd)
		if err == nil {
			log.Printf("Iptable filter for private ipaddr %v on %v chain %v target rule already exists", ipAddress, chainName, target)
			return nil
		}
	}

	cmd := fmt.Sprintf("iptables -t filter -%v %v -%v %v -d %v -j %v", action, chainName, option, snatBridgeName, ipAddress, target)
	_, err := platform.ExecuteCommand(cmd)
	if err != nil {
		log.Printf("Iptable filter %v action for private ipaddr %v on %v chain %v target failed with %v", action, ipAddress, chainName, target, err)
		return err
	}

	return nil
}

func addOrDeletePrivateIPBlockRule(action string) error {
	privateIPAddresses := getPrivateIPSpace()
	chains := getFilterChains()
	target := getFilterchainTarget()

	for _, chain := range chains {
		if err := addOrDeleteFilterRule(action, "10.0.0.10", chain, target[0]); err != nil {
			return err
		}
	}

	for _, ipAddress := range privateIPAddresses {
		if err := addOrDeleteFilterRule(action, ipAddress, chains[0], target[1]); err != nil {
			return err
		}

		if err := addOrDeleteFilterRule(action, ipAddress, chains[1], target[1]); err != nil {
			return err
		}

		if err := addOrDeleteFilterRule(action, ipAddress, chains[2], target[1]); err != nil {
			return err
		}
	}

	return nil
}

func addStaticRoute(ip string, interfaceName string) error {
	log.Printf("[ovs] Adding %v static route", ip)
	var routes []RouteInfo
	_, ipNet, _ := net.ParseCIDR(imdsIP)
	gwIP := net.ParseIP("0.0.0.0")
	route := RouteInfo{Dst: *ipNet, Gw: gwIP}
	routes = append(routes, route)
	if err := addRoutes(interfaceName, routes); err != nil {
		if err != nil && !strings.Contains(strings.ToLower(err.Error()), "file exists") {
			log.Printf("addroutes failed with error %v", err)
			return err
		}
	}

	return nil
}

func DeleteSnatBridge() error {
	// Delete the bridge.
	err := netlink.DeleteLink(snatBridgeName)
	if err != nil {
		log.Printf("[net] Failed to delete bridge %v, err:%v.", snatBridgeName, err)
	}

	return err
}

func (client *OVSNetworkClient) AddL2Rules(extIf *externalInterface) error {
	//primary := extIf.IPAddresses[0].IP.String()
	mac := extIf.MacAddress.String()
	macHex := strings.Replace(mac, ":", "", -1)

	ofport, err := ovsctl.GetOVSPortNumber(client.hostInterfaceName)
	if err != nil {
		return err
	}

	// Arp SNAT Rule
	log.Printf("[ovs] Adding ARP SNAT rule for egress traffic on interface %v", client.hostInterfaceName)
	if err := ovsctl.AddArpSnatRule(client.bridgeName, mac, macHex, ofport); err != nil {
		return err
	}

	log.Printf("[ovs] Adding DNAT rule for ingress ARP traffic on interface %v.", client.hostInterfaceName)
	if err := ovsctl.AddArpDnatRule(client.bridgeName, ofport, macHex); err != nil {
		return err
	}

	if client.enableSnatOnHost {
		addStaticRoute(imdsIP, client.bridgeName)
	}

	return nil
}

func (client *OVSNetworkClient) DeleteL2Rules(extIf *externalInterface) {
	ovsctl.DeletePortFromOVS(client.bridgeName, client.hostInterfaceName)
}

func (client *OVSNetworkClient) SetBridgeMasterToHostInterface() error {
	return ovsctl.AddPortOnOVSBridge(client.hostInterfaceName, client.bridgeName, 0)
}

func (client *OVSNetworkClient) SetHairpinOnHostInterface(enable bool) error {
	return nil
}
