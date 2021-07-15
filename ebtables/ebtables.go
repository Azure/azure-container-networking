// Copyright 2017 Microsoft. All rights reserved.
// MIT License

package ebtables

import (
	"fmt"
	"net"
	"strings"

	"github.com/Azure/azure-container-networking/platform"
)

const (
	// Ebtable actions.
	Append = "-A"
	Delete = "-D"
	// Ebtable tables.
	Nat    = "nat"
	Broute = "broute"
	Filter = "filter"
	// Ebtable chains.
	PreRouting  = "PREROUTING"
	PostRouting = "POSTROUTING"
	Brouting    = "BROUTING"
	Forward     = "FORWARD"
	// Ebtable Protocols
	IPV4 = "IPv4"
	IPV6 = "IPv6"
	// Ebtable Targets
	Accept         = "ACCEPT"
	RedirectAccept = "redirect --redirect-target ACCEPT"
)

// SetSnatForInterface sets a MAC SNAT rule for an interface.
func SetSnatForInterface(interfaceName string, macAddress net.HardwareAddr, action string) error {
	table := Nat
	chain := PostRouting
	rule := fmt.Sprintf("-s unicast -o %s -j snat --to-src %s --snat-arp --snat-target ACCEPT",
		interfaceName, macAddress.String())

	return runEbCmd(table, action, chain, rule)
}

// SetArpReply sets an ARP reply rule for the given target IP address and MAC address.
func SetArpReply(ipAddress net.IP, macAddress net.HardwareAddr, action string) error {
	table := Nat
	chain := PreRouting
	rule := fmt.Sprintf("-p ARP --arp-op Request --arp-ip-dst %s -j arpreply --arpreply-mac %s --arpreply-target DROP",
		ipAddress, macAddress.String())

	return runEbCmd(table, action, chain, rule)
}

// SetBrouteAccept sets an EB rule.
func SetBrouteAccept(ipAddress, action string) error {
	table := Broute
	chain := Brouting
	rule := fmt.Sprintf("--ip-dst %s -p IPv4 -j redirect --redirect-target ACCEPT", ipAddress)

	return runEbCmd(table, action, chain, rule)
}

// SetDnatForArpReplies sets a MAC DNAT rule for ARP replies received on an interface.
func SetDnatForArpReplies(interfaceName string, action string) error {
	table := Nat
	chain := PreRouting
	rule := fmt.Sprintf("-p ARP -i %s --arp-op Reply -j dnat --to-dst ff:ff:ff:ff:ff:ff --dnat-target ACCEPT",
		interfaceName)

	return runEbCmd(table, action, chain, rule)
}

// SetVepaMode sets the VEPA mode for a bridge and its ports.
func SetVepaMode(bridgeName string, downstreamIfNamePrefix string, upstreamMacAddress string, action string) error {
	table := Nat
	chain := PreRouting

	if !strings.HasPrefix(bridgeName, downstreamIfNamePrefix) {
		rule := fmt.Sprintf("-i %s -j dnat --to-dst %s --dnat-target ACCEPT", bridgeName, upstreamMacAddress)

		if err := runEbCmd(table, action, chain, rule); err != nil {
			return err
		}
	}

	rule2 := fmt.Sprintf("-i %s+ -j dnat --to-dst %s --dnat-target ACCEPT",
		downstreamIfNamePrefix, upstreamMacAddress)

	return runEbCmd(table, action, chain, rule2)
}

// SetDnatForIPAddress sets a MAC DNAT rule for an IP address.
func SetDnatForIPAddress(interfaceName string, ipAddress net.IP, macAddress net.HardwareAddr, action string) error {
	protocol := "IPv4"
	dst := "--ip-dst"
	if ipAddress.To4() == nil {
		protocol = "IPv6"
		dst = "--ip6-dst"
	}

	table := Nat
	chain := PreRouting
	rule := fmt.Sprintf("-p %s -i %s %s %s -j dnat --to-dst %s --dnat-target ACCEPT",
		protocol, interfaceName, dst, ipAddress.String(), macAddress.String())

	return runEbCmd(table, action, chain, rule)
}

// Drop Icmpv6 discovery messages going out of interface
func DropICMPv6Solicitation(interfaceName string, action string) error {
	table := Filter
	chain := Forward

	rule := fmt.Sprintf("-p IPv6 --ip6-proto ipv6-icmp --ip6-icmp-type neighbour-solicitation -o %s -j DROP",
		interfaceName)

	return runEbCmd(table, action, chain, rule)
}

// SetEbRule sets any given eb rule
func SetEbRule(table, action, chain, rule string) error {
	return runEbCmd(table, action, chain, rule)
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

// SetBrouteAcceptCidr - broute chain MAC redirect rule. Will change mac target address to bridge port
// that receives the frame.
func SetBrouteAcceptByCidr(ipNet *net.IPNet, protocol, action, target string) error {
	dst := "--ip-dst"
	if protocol == IPV6 {
		dst = "--ip6-dst"
	}

	var rule string
	table := Broute
	chain := Brouting
	if ipNet != nil {
		rule = fmt.Sprintf("-p %s %s %s -j %s",
			protocol, dst, ipNet.String(), target)
	} else {
		rule = fmt.Sprintf("-p %s -j %s",
			protocol, target)
	}

	return runEbCmd(table, action, chain, rule)
}

func SetBrouteAcceptByInterface(ifName string, protocol, action, target string) error {
	var rule string
	table := Broute
	chain := Brouting
	if protocol == "" {
		rule = fmt.Sprintf("-i %s -j %s", ifName, target)
	} else {
		rule = fmt.Sprintf("-p %s -i %s -j %s", protocol, ifName, target)
	}

	return runEbCmd(table, action, chain, rule)
}

func SetArpDropRuleForIpCidr(ipCidr string, ifName string) error {
	rule := fmt.Sprintf("-p ARP -o %s --arp-op Request --arp-ip-dst %s -j DROP", ifName, ipCidr)
	exists, err := EbTableRuleExists(Nat, PostRouting, rule)
	if err != nil {
		return err
	}

	if exists {
		return nil
	}

	return runEbCmd(Nat, Append, PostRouting, rule)
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

// runEbCmd runs an EB rule command.
func runEbCmd(table, action, chain, rule string) error {
	command := fmt.Sprintf("ebtables -t %s %s %s %s", table, action, chain, rule)
	_, err := platform.ExecuteCommand(command)

	return err
}
