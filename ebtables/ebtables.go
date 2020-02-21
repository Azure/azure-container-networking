// Copyright 2017 Microsoft. All rights reserved.
// MIT License

package ebtables

import (
	"fmt"
	"io/ioutil"
	"net"
	"os/exec"
	"strings"

	"github.com/Azure/azure-container-networking/log"
	"github.com/Azure/azure-container-networking/platform"
)

const (
	// Ebtables actions.
	Append = "-A"
	Delete = "-D"
)

// InstallEbtables installs the ebtables package.
func installEbtables() {
	version, _ := ioutil.ReadFile("/proc/version")
	os := strings.ToLower(string(version))

	if strings.Contains(os, "ubuntu") {
		executeShellCommand("apt-get install ebtables")
	} else if strings.Contains(os, "redhat") {
		executeShellCommand("yum install ebtables")
	} else {
		log.Printf("Unable to detect OS platform. Please make sure the ebtables package is installed.")
	}
}

// SetSnatForInterface sets a MAC SNAT rule for an interface.
func SetSnatForInterface(interfaceName string, macAddress net.HardwareAddr, action string) error {
	command := fmt.Sprintf(
		"ebtables -t nat %s POSTROUTING -s unicast -o %s -j snat --to-src %s --snat-arp --snat-target ACCEPT",
		action, interfaceName, macAddress.String())

	return executeShellCommand(command)
}

// SetArpReply sets an ARP reply rule for the given target IP address and MAC address.
func SetArpReply(ipAddress net.IP, macAddress net.HardwareAddr, action string) error {
	command := fmt.Sprintf(
		"ebtables -t nat %s PREROUTING -p ARP --arp-op Request --arp-ip-dst %s -j arpreply --arpreply-mac %s --arpreply-target DROP",
		action, ipAddress, macAddress.String())

	return executeShellCommand(command)
}

// SetBrouteAccept sets an EB rule.
func SetBrouteAccept(ipAddress, action string) error {
	command := fmt.Sprintf(
		"ebtables -t broute %s BROUTING --ip-dst %s -p IPv4 -j redirect --redirect-target ACCEPT",
		action, ipAddress)
	_, err := platform.ExecuteCommand(command)

	return err
}

// GetEbtableRules gets EB rules for a table and chain.
func GetEbtableRules(tableName, chainName string) ([]string, error) {
	var (
		inChain bool
		rules   []string
	)

	command := fmt.Sprintf(
		"ebtables -t %s -L %s --Lmac2",
		tableName, chainName)
	out, err := platform.ExecuteCommand(command)
	if err != nil {
		return nil, err
	}

	// Splits lines and finds rules.
	lines := strings.Split(out, "\n")
	chainTitle := fmt.Sprintf("Bridge chain: %s", chainName)

	for _, line := range lines {
		if strings.HasPrefix(line, chainTitle) {
			inChain = true
			continue
		}
		if inChain {
			if strings.HasPrefix(line, "-") {
				rules = append(rules, strings.TrimSpace(line))
			} else {
				break
			}
		}
	}

	return rules, nil
}

// EbTableRuleExists checks if eb rule exists in table and chain.
func EbTableRuleExists(tableName, chainName, matchSet string) (bool, error) {
	rules, err := GetEbtableRules(tableName, chainName)
	if err != nil {
		return false, err
	}

	for _, rule := range rules {
		if rule == matchSet {
			return true, nil
		}
	}

	return false, nil
}

// SetDnatForArpReplies sets a MAC DNAT rule for ARP replies received on an interface.
func SetDnatForArpReplies(interfaceName string, action string) error {
	command := fmt.Sprintf(
		"ebtables -t nat %s PREROUTING -p ARP -i %s --arp-op Reply -j dnat --to-dst ff:ff:ff:ff:ff:ff --dnat-target ACCEPT",
		action, interfaceName)

	return executeShellCommand(command)
}

// SetVepaMode sets the VEPA mode for a bridge and its ports.
func SetVepaMode(bridgeName string, downstreamIfNamePrefix string, upstreamMacAddress string, action string) error {
	if !strings.HasPrefix(bridgeName, downstreamIfNamePrefix) {
		command := fmt.Sprintf(
			"ebtables -t nat %s PREROUTING -i %s -j dnat --to-dst %s --dnat-target ACCEPT",
			action, bridgeName, upstreamMacAddress)

		err := executeShellCommand(command)
		if err != nil {
			return err
		}
	}

	command := fmt.Sprintf(
		"ebtables -t nat %s PREROUTING -i %s+ -j dnat --to-dst %s --dnat-target ACCEPT",
		action, downstreamIfNamePrefix, upstreamMacAddress)

	return executeShellCommand(command)
}

// SetDnatForIPAddress sets a MAC DNAT rule for an IP address.
func SetDnatForIPAddress(interfaceName string, ipAddress net.IP, macAddress net.HardwareAddr, action string) error {
	command := fmt.Sprintf(
		"ebtables -t nat %s PREROUTING -p IPv4 -i %s --ip-dst %s -j dnat --to-dst %s --dnat-target ACCEPT",
		action, interfaceName, ipAddress.String(), macAddress.String())

	return executeShellCommand(command)
}

func executeShellCommand(command string) error {
	log.Debugf("[ebtables] %s", command)
	cmd := exec.Command("sh", "-c", command)
	err := cmd.Start()
	if err != nil {
		return err
	}
	return cmd.Wait()
}
