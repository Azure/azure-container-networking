//go:build linux
// +build linux

package networkutils

import (
	"fmt"
	"net"

	"github.com/Azure/azure-container-networking/cni/log"
	"github.com/Azure/azure-container-networking/iptables"
	"github.com/Azure/azure-container-networking/netlink"
	"github.com/Azure/azure-container-networking/platform"
	"github.com/pkg/errors"
	"go.uber.org/zap"
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
	toggleIPV6Cmd        = "sysctl -w net.ipv6.conf.all.disable_ipv6=%d"
	enableIPV6ForwardCmd = "sysctl -w net.ipv6.conf.all.forwarding=1"
	enableIPV4ForwardCmd = "sysctl -w net.ipv4.conf.all.forwarding=1"
	disableRACmd         = "sysctl -w net.ipv6.conf.%s.accept_ra=0"
	acceptRAV6File       = "/proc/sys/net/ipv6/conf/%s/accept_ra"
)

var logger = log.CNILogger.With(zap.String("component", "net-utils"))

type ipTablesClient interface {
	InsertIptableRule(version, tableName, chainName, match, target string) error
	AppendIptableRule(version, tableName, chainName, match, target string) error
	DeleteIptableRule(version, tableName, chainName, match, target string) error
}

var errorNetworkUtils = errors.New("NetworkUtils Error")

func newErrorNetworkUtils(errStr string) error {
	return fmt.Errorf("%w : %s", errorNetworkUtils, errStr)
}

type NetworkUtils struct {
	netlink  netlink.NetlinkInterface
	plClient platform.ExecClient
}

func NewNetworkUtils(nl netlink.NetlinkInterface, plClient platform.ExecClient) NetworkUtils {
	return NetworkUtils{
		netlink:  nl,
		plClient: plClient,
	}
}

func (nu NetworkUtils) CreateEndpoint(hostVethName, containerVethName string, macAddress net.HardwareAddr) error {
	logger.Info("Creating veth pair", zap.String("hostVethName", hostVethName), zap.String("containerVethName", containerVethName))

	link := netlink.VEthLink{
		LinkInfo: netlink.LinkInfo{
			Type:       netlink.LINK_TYPE_VETH,
			Name:       hostVethName,
			MacAddress: macAddress,
		},
		PeerName: containerVethName,
	}

	err := nu.netlink.AddLink(&link)
	if err != nil {
		logger.Error("Failed to create veth pair with", zap.Error(err))
		return newErrorNetworkUtils(err.Error())
	}

	err = nu.netlink.SetLinkState(hostVethName, true)
	if err != nil {
		return newErrorNetworkUtils(err.Error())
	}

	if err := nu.DisableRAForInterface(hostVethName); err != nil {
		return newErrorNetworkUtils(err.Error())
	}

	return nil
}

func (nu NetworkUtils) SetupContainerInterface(containerVethName, targetIfName string) error {
	// Interface needs to be down before renaming.
	if err := nu.netlink.SetLinkState(containerVethName, false); err != nil {
		return newErrorNetworkUtils(err.Error())
	}

	// Rename the container interface.
	logger.Info("Setting link", zap.String("containerVethName", containerVethName), zap.String("targetIfName", targetIfName))
	if err := nu.netlink.SetLinkName(containerVethName, targetIfName); err != nil {
		return newErrorNetworkUtils(err.Error())
	}

	if err := nu.DisableRAForInterface(targetIfName); err != nil {
		return newErrorNetworkUtils(err.Error())
	}

	// Bring the interface back up.
	err := nu.netlink.SetLinkState(targetIfName, true)
	if err != nil {
		return newErrorNetworkUtils(err.Error())
	}
	return nil
}

func (nu NetworkUtils) AssignIPToInterface(interfaceName string, ipAddresses []net.IPNet) error {
	var err error
	// Assign IP address to container network interface.
	for i, ipAddr := range ipAddresses {
		logger.Info("Adding IP", zap.String("address", ipAddr.String()), zap.String("interfaceName", interfaceName))
		err = nu.netlink.AddIPAddress(interfaceName, ipAddr.IP, &ipAddresses[i])
		if err != nil {
			return newErrorNetworkUtils(err.Error())
		}
	}

	return nil
}

func (nu NetworkUtils) addOrDeleteFilterRule(iptablesClient ipTablesClient, bridgeName, action, ipAddress, chainName, target string) error {
	var err error
	option := "i"

	if chainName == iptables.Output {
		option = "o"
	}

	matchCondition := fmt.Sprintf("-%s %s -d %s", option, bridgeName, ipAddress)

	switch action {
	case iptables.Insert:
		err = iptablesClient.InsertIptableRule(iptables.V4, iptables.Filter, chainName, matchCondition, target)
	case iptables.Append:
		err = iptablesClient.AppendIptableRule(iptables.V4, iptables.Filter, chainName, matchCondition, target)
	case iptables.Delete:
		err = iptablesClient.DeleteIptableRule(iptables.V4, iptables.Filter, chainName, matchCondition, target)
	}

	return err
}

func (nu NetworkUtils) AllowIPAddresses(iptablesClient ipTablesClient, bridgeName string, skipAddresses []string, action string) error {
	chains := getFilterChains()
	target := getFilterchainTarget()

	logger.Info("Addresses to allow", zap.Any("skipAddresses", skipAddresses))

	for _, address := range skipAddresses {
		if err := nu.addOrDeleteFilterRule(iptablesClient, bridgeName, action, address, chains[0], target[0]); err != nil {
			return err
		}

		if err := nu.addOrDeleteFilterRule(iptablesClient, bridgeName, action, address, chains[1], target[0]); err != nil {
			return err
		}

		if err := nu.addOrDeleteFilterRule(iptablesClient, bridgeName, action, address, chains[2], target[0]); err != nil {
			return err
		}

	}

	return nil
}

func (nu NetworkUtils) BlockEgressTrafficFromContainer(iptablesClient ipTablesClient, version, ipAddress, protocol string, port int) error {
	// iptables -t filter -I FORWARD -j DROP -d <ip> -p <protocol> -m <protocol> --dport <port>
	dropTraffic := fmt.Sprintf("-d %s -p %s -m %s --dport %d", ipAddress, protocol, protocol, port)
	return errors.Wrap(iptablesClient.InsertIptableRule(version, iptables.Filter, iptables.Forward, dropTraffic, iptables.Drop), "iptables block traffic failed")
}

func (nu NetworkUtils) BlockIPAddresses(iptablesClient ipTablesClient, bridgeName, action string) error {
	privateIPAddresses := getPrivateIPSpace()
	chains := getFilterChains()
	target := getFilterchainTarget()

	logger.Info("Addresses to block", zap.Any("privateIPAddresses", privateIPAddresses))

	for _, ipAddress := range privateIPAddresses {
		if err := nu.addOrDeleteFilterRule(iptablesClient, bridgeName, action, ipAddress, chains[0], target[1]); err != nil {
			return err
		}

		if err := nu.addOrDeleteFilterRule(iptablesClient, bridgeName, action, ipAddress, chains[1], target[1]); err != nil {
			return err
		}

		if err := nu.addOrDeleteFilterRule(iptablesClient, bridgeName, action, ipAddress, chains[2], target[1]); err != nil {
			return err
		}
	}

	return nil
}

func (nu NetworkUtils) EnableIPV4Forwarding() error {
	_, err := nu.plClient.ExecuteRawCommand(enableIPV4ForwardCmd)
	if err != nil {
		logger.Error("Enable ipv4 forwarding failed with", zap.Error(err))
		return errors.Wrap(err, "enable ipv4 forwarding failed")
	}

	return nil
}

func (nu NetworkUtils) EnableIPV6Forwarding() error {
	cmd := fmt.Sprint(enableIPV6ForwardCmd)
	_, err := nu.plClient.ExecuteRawCommand(cmd)
	if err != nil {
		logger.Error("Enable ipv6 forwarding failed with", zap.Error(err))
		return err
	}

	return nil
}

// This functions enables/disables ipv6 setting based on enable parameter passed.
func (nu NetworkUtils) UpdateIPV6Setting(disable int) error {
	// sysctl -w net.ipv6.conf.all.disable_ipv6=0/1
	cmd := fmt.Sprintf(toggleIPV6Cmd, disable)
	_, err := nu.plClient.ExecuteRawCommand(cmd)
	if err != nil {
		logger.Error("Update IPV6 Setting failed with", zap.Error(err))
	}

	return err
}

// This function adds rule which snat to ip passed filtered by match string.
func (nu NetworkUtils) AddSnatRule(iptablesClient ipTablesClient, match string, ip net.IP) error {
	version := iptables.V4
	if ip.To4() == nil {
		version = iptables.V6
	}

	target := fmt.Sprintf("SNAT --to %s", ip.String())
	return errors.Wrap(iptablesClient.InsertIptableRule(version, iptables.Nat, iptables.Postrouting, match, target), "failed to add snat rule")
}

func (nu NetworkUtils) DisableRAForInterface(ifName string) error {
	raFilePath := fmt.Sprintf(acceptRAV6File, ifName)
	exist, err := platform.CheckIfFileExists(raFilePath)
	if !exist {
		logger.Error("accept_ra file doesn't exist with", zap.Error(err))
		return nil
	}

	cmd := fmt.Sprintf(disableRACmd, ifName)
	out, err := nu.plClient.ExecuteRawCommand(cmd)
	if err != nil {
		logger.Error("Diabling ra failed with", zap.Error(err), zap.Any("out", out))
	}

	return err
}

func (nu NetworkUtils) SetProxyArp(ifName string) error {
	cmd := fmt.Sprintf("echo 1 > /proc/sys/net/ipv4/conf/%v/proxy_arp", ifName)
	_, err := nu.plClient.ExecuteRawCommand(cmd)
	return errors.Wrapf(err, "failed to set proxy arp for interface %v", ifName)
}

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
