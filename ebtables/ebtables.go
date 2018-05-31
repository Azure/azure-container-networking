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

func CheckIfDnatForArpReplyRuleExists(rulesList string, interfaceName string) bool {
	arpReplyMatch := fmt.Sprintf("-p ARP -i %s --arp-op Reply -j dnat --to-dst ff:ff:ff:ff:ff:ff --dnat-target ACCEPT", interfaceName)

	for _, line := range strings.Split(rulesList, "\n") {
		if strings.TrimSpace(line) == arpReplyMatch {
			return true
		}
	}

	return false
}

func CheckIfSnatRuleExists(rulesList string, interfaceName string, macAddress net.HardwareAddr) bool {
	snatRuleMatch := fmt.Sprintf("-s Unicast -o %s -j snat --to-src %s --snat-arp --snat-target ACCEPT", interfaceName, macAddress.String())

	for _, line := range strings.Split(rulesList, "\n") {
		if strings.TrimSpace(line) == snatRuleMatch {
			return true
		}
	}
	return false
}

func CheckIfArpReplyRuleExists(rulesList string, ipAddress net.IP, macAddress net.HardwareAddr) bool {
	arpReplyRuleMatch := fmt.Sprintf("-p ARP --arp-op Request --arp-ip-dst %s -j arpreply --arpreply-mac %s", ipAddress.String(), macAddress.String())

	for _, line := range strings.Split(rulesList, "\n") {
		if strings.TrimSpace(line) == arpReplyRuleMatch {
			return true
		}
	}
	return false
}

func CheckIfDnatForIPRuleExists(rulesList string, interfaceName string, ipAddress net.IP, macAddress net.HardwareAddr) bool {
	dnatForIPRuleMatch := fmt.Sprintf("-p IPv4 -i %s --ip-dst %s -j dnat --to-dst %s --dnat-target ACCEPT", interfaceName, ipAddress.String(), macAddress.String())

	for _, line := range strings.Split(rulesList, "\n") {
		if strings.TrimSpace(line) == dnatForIPRuleMatch {
			return true
		}
	}
	return false
}
