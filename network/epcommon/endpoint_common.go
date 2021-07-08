// +build linux

package epcommon

import (
	"fmt"
	"net"

	"github.com/Azure/azure-container-networking/iptables"
	"github.com/Azure/azure-container-networking/log"
	"github.com/Azure/azure-container-networking/netlink"
	"github.com/Azure/azure-container-networking/platform"
)

/*RFC For Private Address Space: https://tools.ietf.org/html/rfc1918
   The Internet Assigned Numbers Authority (IANA) has reserved the
   following three blocks of the IP address space for private internets:

     10.0.0.0        -   10.255.255.255  (10/8 prefix)
     172.16.0.0      -   172.31.255.255  (172.16/12 prefix)
     192.168.0.0     -   192.168.255.255 (192.168/16 prefix)

RFC for Link Local Addresses: https://tools.ietf.org/html/rfc3927
   This document describes how a host may
   automatically configure an interface with an IPv4 address within the
   169.254/16 prefix that is valid for communication with other devices
   connected to the same physical (or logical) link.
*/

const (
	enableIPForwardCmd   = "sysctl -w net.ipv4.ip_forward=1"
	toggleIPV6Cmd        = "sysctl -w net.ipv6.conf.all.disable_ipv6=%d"
	enableIPV6ForwardCmd = "sysctl -w net.ipv6.conf.all.forwarding=1"
	disableRACmd         = "sysctl -w net.ipv6.conf.%s.accept_ra=0"
	acceptRAV6File       = "/proc/sys/net/ipv6/conf/%s/accept_ra"
)

func getPrivateIPSpace() []string {
	privateIPAddresses := []string{"10.0.0.0/8", "172.16.0.0/12", "192.168.0.0/16", "169.254.0.0/16"}
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

	if err := DisableRAForInterface(hostVethName); err != nil {
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

	if err := DisableRAForInterface(targetIfName); err != nil {
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
	var err error
	option := "i"

	if chainName == iptables.Output {
		option = "o"
	}

	matchCondition := fmt.Sprintf("-%s %s -d %s", option, bridgeName, ipAddress)

	switch action {
	case iptables.Insert:
		err = iptables.InsertIptableRule(iptables.V4, iptables.Filter, chainName, matchCondition, target)
	case iptables.Append:
		err = iptables.AppendIptableRule(iptables.V4, iptables.Filter, chainName, matchCondition, target)
	case iptables.Delete:
		err = iptables.DeleteIptableRule(iptables.V4, iptables.Filter, chainName, matchCondition, target)
	}

	return err
}

func AllowIPAddresses(bridgeName string, skipAddresses []string, action string) error {
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

	return nil
}

func BlockIPAddresses(bridgeName string, action string) error {
	privateIPAddresses := getPrivateIPSpace()
	chains := getFilterChains()
	target := getFilterchainTarget()

	log.Printf("[net] Addresses to block %v", privateIPAddresses)

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

// This fucntion enables ip forwarding in VM and allow forwarding packets from the interface
func EnableIPForwarding(ifName string) error {
	// Enable ip forwading on linux vm.
	// sysctl -w net.ipv4.ip_forward=1
	cmd := fmt.Sprint(enableIPForwardCmd)
	_, err := platform.ExecuteCommand(cmd)
	if err != nil {
		log.Printf("[net] Enable ipforwarding failed with: %v", err)
		return err
	}

	// Append a rule in forward chain to allow forwarding from bridge
	if err := iptables.AppendIptableRule(iptables.V4, iptables.Filter, iptables.Forward, "", iptables.Accept); err != nil {
		log.Printf("[net] Appending forward chain rule: allow traffic coming from snatbridge failed with: %v", err)
		return err
	}

	return nil
}

func EnableIPV6Forwarding() error {
	cmd := fmt.Sprint(enableIPV6ForwardCmd)
	_, err := platform.ExecuteCommand(cmd)
	if err != nil {
		log.Printf("[net] Enable ipv6 forwarding failed with: %v", err)
		return err
	}

	return nil
}

// This functions enables/disables ipv6 setting based on enable parameter passed.
func UpdateIPV6Setting(disable int) error {
	// sysctl -w net.ipv6.conf.all.disable_ipv6=0/1
	cmd := fmt.Sprintf(toggleIPV6Cmd, disable)
	_, err := platform.ExecuteCommand(cmd)
	if err != nil {
		log.Printf("[net] Update IPV6 Setting failed with: %v", err)
	}

	return err
}

// This fucntion adds rule which snat to ip passed filtered by match string.
func AddSnatRule(match string, ip net.IP) error {
	version := iptables.V4
	if ip.To4() == nil {
		version = iptables.V6
	}

	target := fmt.Sprintf("SNAT --to %s", ip.String())
	return iptables.InsertIptableRule(version, iptables.Nat, iptables.Postrouting, match, target)
}

func DisableRAForInterface(ifName string) error {
	raFilePath := fmt.Sprintf(acceptRAV6File, ifName)
	exist, err := platform.CheckIfFileExists(raFilePath)
	if !exist {
		log.Printf("[net] accept_ra file doesn't exist:err:%v", err)
		return nil
	}

	cmd := fmt.Sprintf(disableRACmd, ifName)
	out, err := platform.ExecuteCommand(cmd)
	if err != nil {
		log.Errorf("[net] Diabling ra failed with err: %v out: %v", err, out)
	}

	return err
}
