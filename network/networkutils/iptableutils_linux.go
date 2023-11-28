//go:build linux
// +build linux

package networkutils

import (
	"fmt"
	"net"

	"github.com/Azure/azure-container-networking/iptables"
	"go.uber.org/zap"
)

func addOrDeleteFilterRule(iptablesClient IPTablesClientInterface, bridgeName, action, ipAddress, chainName, target string) error {
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

func AllowIPAddresses(iptablesClient IPTablesClientInterface, bridgeName string, skipAddresses []string, action string) error {
	chains := getFilterChains()
	target := getFilterchainTarget()

	logger.Info("Addresses to allow", zap.Any("skipAddresses", skipAddresses))

	for _, address := range skipAddresses {
		if err := addOrDeleteFilterRule(iptablesClient, bridgeName, action, address, chains[0], target[0]); err != nil {
			return err
		}

		if err := addOrDeleteFilterRule(iptablesClient, bridgeName, action, address, chains[1], target[0]); err != nil {
			return err
		}

		if err := addOrDeleteFilterRule(iptablesClient, bridgeName, action, address, chains[2], target[0]); err != nil {
			return err
		}

	}

	return nil
}

func BlockIPAddresses(iptablesClient IPTablesClientInterface, bridgeName, action string) error {
	privateIPAddresses := getPrivateIPSpace()
	chains := getFilterChains()
	target := getFilterchainTarget()

	logger.Info("Addresses to block", zap.Any("privateIPAddresses", privateIPAddresses))

	for _, ipAddress := range privateIPAddresses {
		if err := addOrDeleteFilterRule(iptablesClient, bridgeName, action, ipAddress, chains[0], target[1]); err != nil {
			return err
		}

		if err := addOrDeleteFilterRule(iptablesClient, bridgeName, action, ipAddress, chains[1], target[1]); err != nil {
			return err
		}

		if err := addOrDeleteFilterRule(iptablesClient, bridgeName, action, ipAddress, chains[2], target[1]); err != nil {
			return err
		}
	}

	return nil
}

// This fucntion adds rule which snat to ip passed filtered by match string.
func AddSnatRule(iptablesClient IPTablesClientInterface, match string, ip net.IP) error {
	version := iptables.V4
	if ip.To4() == nil {
		version = iptables.V6
	}

	target := fmt.Sprintf("SNAT --to %s", ip.String())
	return iptablesClient.InsertIptableRule(version, iptables.Nat, iptables.Postrouting, match, target)
}
