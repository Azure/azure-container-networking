package epcommon

import (
	"fmt"
	"net"

	"github.com/Azure/azure-container-networking/log"
	"github.com/Azure/azure-container-networking/netlink"
	"github.com/Azure/azure-container-networking/platform"
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

func CreateEndpoint(hostVethName string, containerVethName string) error {
	log.Printf("[net] Creating veth pair %v %v.", hostVethName, containerVethName)

	link := netlink.VEthLink{
		LinkInfo: netlink.LinkInfo{
			Type: netlink.LINK_TYPE_VETH,
			Name: hostVethName,
		},
		PeerName: containerVethName,
	}

	err := netlink.AddLink(&link)
	if err != nil {
		log.Printf("[net] Failed to create veth pair, err:%v.", err)
		return err
	}

	log.Printf("[net] Setting link %v state up.", hostVethName)
	err = netlink.SetLinkState(hostVethName, true)
	if err != nil {
		return err
	}

	return nil
}

func SetupContainerInterface(containerVethName string, targetIfName string) error {
	// Interface needs to be down before renaming.
	log.Printf("[net] Setting link %v state down.", containerVethName)
	if err := netlink.SetLinkState(containerVethName, false); err != nil {
		return err
	}

	// Rename the container interface.
	log.Printf("[net] Setting link %v name %v.", containerVethName, targetIfName)
	if err := netlink.SetLinkName(containerVethName, targetIfName); err != nil {
		return err
	}

	// Bring the interface back up.
	log.Printf("[net] Setting link %v state up.", targetIfName)
	return netlink.SetLinkState(targetIfName, true)
}

func AssignIPToInterface(interfaceName string, ipAddresses []net.IPNet) error {
	// Assign IP address to container network interface.
	for _, ipAddr := range ipAddresses {
		log.Printf("[net] Adding IP address %v to link %v.", ipAddr.String(), interfaceName)
		err := netlink.AddIpAddress(interfaceName, ipAddr.IP, &ipAddr)
		if err != nil {
			return err
		}
	}

	return nil
}

func addOrDeleteFilterRule(bridgeName string, action string, ipAddress string, chainName string, target string) error {
	option := "i"

	if chainName == "OUTPUT" {
		option = "o"
	}

	if action != "D" {
		cmd := fmt.Sprintf("iptables -t filter -C %v -%v %v -d %v -j %v", chainName, option, bridgeName, ipAddress, target)
		_, err := platform.ExecuteCommand(cmd)
		if err == nil {
			log.Printf("Iptable filter for private ipaddr %v on %v chain %v target rule already exists", ipAddress, chainName, target)
			return nil
		}
	}

	cmd := fmt.Sprintf("iptables -t filter -%v %v -%v %v -d %v -j %v", action, chainName, option, bridgeName, ipAddress, target)
	_, err := platform.ExecuteCommand(cmd)
	if err != nil {
		log.Printf("Iptable filter %v action for private ipaddr %v on %v chain %v target failed with %v", action, ipAddress, chainName, target, err)
		return err
	}

	return nil
}

func AddOrDeletePrivateIPBlockRule(bridgeName string, skipAddresses []string, action string) error {
	privateIPAddresses := getPrivateIPSpace()
	chains := getFilterChains()
	target := getFilterchainTarget()

	log.Printf("[net] Addresses to allow %v", skipAddresses)

	for _, address := range skipAddresses {
		if err := addOrDeleteFilterRule(bridgeName, action, address, chains[0], target[0]); err != nil {
			return err
		}

		if err := addOrDeleteFilterRule(bridgeName, action, address, chains[1], target[0]); err != nil {
			return err
		}

		if err := addOrDeleteFilterRule(bridgeName, action, address, chains[2], target[0]); err != nil {
			return err
		}

	}

	for _, ipAddress := range privateIPAddresses {
		if err := addOrDeleteFilterRule(bridgeName, action, ipAddress, chains[0], target[1]); err != nil {
			return err
		}

		if err := addOrDeleteFilterRule(bridgeName, action, ipAddress, chains[1], target[1]); err != nil {
			return err
		}

		if err := addOrDeleteFilterRule(bridgeName, action, ipAddress, chains[2], target[1]); err != nil {
			return err
		}
	}

	return nil
}
