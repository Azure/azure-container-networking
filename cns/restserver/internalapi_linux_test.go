// Copyright 2020 Microsoft. All rights reserved.
// MIT License

package restserver

import (
	"errors"
	"fmt"
	"strconv"
	"strings"
	"testing"

	"github.com/Azure/azure-container-networking/cns"
	"github.com/Azure/azure-container-networking/cns/types"
	"github.com/Azure/azure-container-networking/iptables"
	"github.com/Azure/azure-container-networking/network/networkutils"
)

var (
	errChainExists   = errors.New("chain already exists")
	errChainNotFound = errors.New("chain not found")
	errRuleExists    = errors.New("rule already exists")
	errRuleNotFound  = errors.New("rule not found")
	errIndexBounds   = errors.New("index out of bounds")
)

type ipTablesLegacyMock struct {
	deleteCallCount int
}

func (c *ipTablesLegacyMock) Delete(_, _ string, _ ...string) error {
	c.deleteCallCount++
	return nil
}

func (c *ipTablesLegacyMock) DeleteCallCount() int {
	return c.deleteCallCount
}

type ipTablesMock struct {
	state               map[string]map[string][]string
	clearChainCallCount int
}

func newIPTablesMock() *ipTablesMock {
	return &ipTablesMock{
		state: make(map[string]map[string][]string),
	}
}

func (c *ipTablesMock) ensureTableExists(table string) {
	_, exists := c.state[table]
	if !exists {
		c.state[table] = make(map[string][]string)
	}
}

func (c *ipTablesMock) ChainExists(table, chain string) (bool, error) {
	c.ensureTableExists(table)

	builtins := []string{iptables.Input, iptables.Output, iptables.Prerouting, iptables.Postrouting, iptables.Forward}

	_, exists := c.state[table][chain]

	// these chains always exist
	for _, val := range builtins {
		if chain == val && !exists {
			c.state[table][chain] = []string{}
			return true, nil
		}
	}

	return exists, nil
}

func (c *ipTablesMock) NewChain(table, chain string) error {
	c.ensureTableExists(table)

	exists, _ := c.ChainExists(table, chain)

	if exists {
		return errChainExists
	}

	c.state[table][chain] = []string{}
	return nil
}

func (c *ipTablesMock) Exists(table, chain string, rulespec ...string) (bool, error) {
	c.ensureTableExists(table)

	chainExists, _ := c.ChainExists(table, chain)
	if !chainExists {
		return false, nil
	}

	targetRule := strings.Join(rulespec, " ")
	chainRules := c.state[table][chain]

	for _, chainRule := range chainRules {
		if targetRule == chainRule {
			return true, nil
		}
	}
	return false, nil
}

func (c *ipTablesMock) Append(table, chain string, rulespec ...string) error {
	c.ensureTableExists(table)

	chainRules := c.state[table][chain]
	return c.Insert(table, chain, len(chainRules)+1, rulespec...)
}

func (c *ipTablesMock) Insert(table, chain string, pos int, rulespec ...string) error {
	c.ensureTableExists(table)

	chainExists, _ := c.ChainExists(table, chain)
	if !chainExists {
		return errChainNotFound
	}

	targetRule := strings.Join(rulespec, " ")
	chainRules := c.state[table][chain]

	// convert 1-based position to 0-based index
	index := pos - 1
	if index < 0 {
		index = 0
	}

	switch {
	case index == len(chainRules):
		c.state[table][chain] = append(chainRules, targetRule)
	case index > len(chainRules):
		return errIndexBounds
	default:
		c.state[table][chain] = append(chainRules[:index], append([]string{targetRule}, chainRules[index:]...)...)
	}

	return nil
}

func (c *ipTablesMock) List(table, chain string) ([]string, error) {
	c.ensureTableExists(table)

	chainExists, _ := c.ChainExists(table, chain)
	if !chainExists {
		return nil, errChainNotFound
	}

	chainRules := c.state[table][chain]
	// preallocate: 1 for chain header + number of rules
	result := make([]string, 0, 1+len(chainRules))

	// for built-in chains, start with policy -P, otherwise start with definition -N
	builtins := []string{iptables.Input, iptables.Output, iptables.Prerouting, iptables.Postrouting, iptables.Forward}
	isBuiltIn := false
	for _, builtin := range builtins {
		if chain == builtin {
			isBuiltIn = true
			break
		}
	}

	if isBuiltIn {
		result = append(result, fmt.Sprintf("-P %s ACCEPT", chain))
	} else {
		result = append(result, "-N "+chain)
	}

	// iptables with -S always outputs the rules in -A format
	for _, rule := range chainRules {
		result = append(result, fmt.Sprintf("-A %s %s", chain, rule))
	}

	return result, nil
}

func (c *ipTablesMock) ClearChain(table, chain string) error {
	c.clearChainCallCount++
	c.ensureTableExists(table)

	chainExists, _ := c.ChainExists(table, chain)
	if !chainExists {
		return errChainNotFound
	}

	c.state[table][chain] = []string{}
	return nil
}

func (c *ipTablesMock) Delete(table, chain string, rulespec ...string) error {
	c.ensureTableExists(table)

	chainExists, _ := c.ChainExists(table, chain)
	if !chainExists {
		return errChainNotFound
	}

	targetRule := strings.Join(rulespec, " ")
	chainRules := c.state[table][chain]

	// delete first match
	for i, rule := range chainRules {
		if rule == targetRule {
			c.state[table][chain] = append(chainRules[:i], chainRules[i+1:]...)
			return nil
		}
	}

	return errRuleNotFound
}

func (c *ipTablesMock) ClearChainCallCount() int {
	return c.clearChainCallCount
}

type fakeIPTablesProvider struct {
	iptables       *ipTablesMock
	iptablesLegacy *ipTablesLegacyMock
}

func (c *fakeIPTablesProvider) GetIPTables() (iptablesClient, error) {
	// persist iptables in testing
	if c.iptables == nil {
		c.iptables = newIPTablesMock()
	}
	return c.iptables, nil
}

func (c *fakeIPTablesProvider) GetIPTablesLegacy() (iptablesLegacyClient, error) {
	if c.iptablesLegacy == nil {
		c.iptablesLegacy = &ipTablesLegacyMock{}
	}
	return c.iptablesLegacy, nil
}

func TestAddSNATRules(t *testing.T) {
	type chainExpectation struct {
		table    string
		chain    string
		expected []string
	}

	type preExistingRule struct {
		table string
		chain string
		rule  []string
	}

	tests := []struct {
		name                    string
		input                   *cns.CreateNetworkContainerRequest
		preExistingRules        []preExistingRule
		expectedChains          []chainExpectation
		expectedClearChainCalls int
	}{
		{
			// in pod subnet, the primary nic ip is in the same address space as the pod subnet
			name: "podsubnet",
			input: &cns.CreateNetworkContainerRequest{
				NetworkContainerid: ncID,
				IPConfiguration: cns.IPConfiguration{
					IPSubnet: cns.IPSubnet{
						IPAddress:    "240.1.2.1",
						PrefixLength: 24,
					},
				},
				SecondaryIPConfigs: map[string]cns.SecondaryIPConfig{
					"abc": {
						IPAddress: "240.1.2.7",
					},
				},
				HostPrimaryIP: "10.0.0.4",
			},
			expectedChains: []chainExpectation{
				{
					table: iptables.Nat,
					chain: SWIFTPOSTROUTING,
					expected: []string{
						"-N SWIFT-POSTROUTING",
						"-A SWIFT-POSTROUTING -m addrtype ! --dst-type local -s 240.1.2.0/24 -d " + networkutils.AzureDNS + " -p udp --dport " + strconv.Itoa(iptables.DNSPort) + " -j SNAT --to 10.0.0.4",
						"-A SWIFT-POSTROUTING -m addrtype ! --dst-type local -s 240.1.2.0/24 -d " + networkutils.AzureDNS + " -p tcp --dport " + strconv.Itoa(iptables.DNSPort) + " -j SNAT --to 10.0.0.4",
						"-A SWIFT-POSTROUTING -m addrtype ! --dst-type local -s 240.1.2.0/24 -d " + networkutils.AzureIMDS + " -p tcp --dport " + strconv.Itoa(iptables.HTTPPort) + " -j SNAT --to 10.0.0.4",
					},
				},
				{
					table: iptables.Nat,
					chain: iptables.Postrouting,
					expected: []string{
						"-P POSTROUTING ACCEPT",
						"-A POSTROUTING -j SWIFT-POSTROUTING",
					},
				},
			},
			expectedClearChainCalls: 1,
		},
		{
			// test with pre-existing SWIFT rule that should be migrated
			name: "migration from old SWIFT",
			input: &cns.CreateNetworkContainerRequest{
				NetworkContainerid: ncID,
				IPConfiguration: cns.IPConfiguration{
					IPSubnet: cns.IPSubnet{
						IPAddress:    "240.1.2.1",
						PrefixLength: 24,
					},
				},
				SecondaryIPConfigs: map[string]cns.SecondaryIPConfig{
					"abc": {
						IPAddress: "240.1.2.7",
					},
				},
				HostPrimaryIP: "10.0.0.4",
			},
			preExistingRules: []preExistingRule{
				{
					table: iptables.Nat,
					chain: iptables.Postrouting,
					rule:  []string{"-j", "SWIFT"},
				},
				{
					// stale rule at lower priority should be cleaned up
					table: iptables.Nat,
					chain: iptables.Postrouting,
					rule:  []string{"-j", "SWIFT-POSTROUTING"},
				},
				{
					// should be cleaned up
					table: iptables.Nat,
					chain: SWIFTPOSTROUTING,
					rule: []string{
						"-m", "addrtype", "!", "--dst-type", "local", "-s", "240.1.2.0/24", "-d", networkutils.AzureDNS,
						"-p", "udp", "--dport", strconv.Itoa(iptables.DNSPort), "-j", "SNAT", "--to", "99.1.2.1",
					},
				},
				{
					table: iptables.Nat,
					chain: "SWIFT",
					rule: []string{
						"-m", "addrtype", "!", "--dst-type", "local", "-s", "240.1.2.0/24", "-d", networkutils.AzureDNS,
						"-p", "udp", "--dport", strconv.Itoa(iptables.DNSPort), "-j", "SNAT", "--to", "192.1.2.1",
					},
				},
			},
			expectedChains: []chainExpectation{
				{
					table: iptables.Nat,
					chain: SWIFTPOSTROUTING,
					expected: []string{
						"-N SWIFT-POSTROUTING",
						"-A SWIFT-POSTROUTING -m addrtype ! --dst-type local -s 240.1.2.0/24 -d " + networkutils.AzureDNS + " -p udp --dport " + strconv.Itoa(iptables.DNSPort) + " -j SNAT --to 10.0.0.4",
						"-A SWIFT-POSTROUTING -m addrtype ! --dst-type local -s 240.1.2.0/24 -d " + networkutils.AzureDNS + " -p tcp --dport " + strconv.Itoa(iptables.DNSPort) + " -j SNAT --to 10.0.0.4",
						"-A SWIFT-POSTROUTING -m addrtype ! --dst-type local -s 240.1.2.0/24 -d " + networkutils.AzureIMDS + " -p tcp --dport " + strconv.Itoa(iptables.HTTPPort) + " -j SNAT --to 10.0.0.4",
					},
				},
				{
					table: iptables.Nat,
					chain: iptables.Postrouting,
					expected: []string{
						"-P POSTROUTING ACCEPT",
						"-A POSTROUTING -j SWIFT-POSTROUTING",
						"-A POSTROUTING -j SWIFT",
					},
				},
				{
					// stale old rule can remain
					table: iptables.Nat,
					chain: "SWIFT",
					expected: []string{
						"-N SWIFT",
						"-A SWIFT -m addrtype ! --dst-type local -s 240.1.2.0/24 -d " + networkutils.AzureDNS + " -p udp --dport " + strconv.Itoa(iptables.DNSPort) + " -j SNAT --to 192.1.2.1",
					},
				},
			},
			expectedClearChainCalls: 1,
		},
		{
			// test after migration has already completed
			name: "after migration from old SWIFT",
			input: &cns.CreateNetworkContainerRequest{
				NetworkContainerid: ncID,
				IPConfiguration: cns.IPConfiguration{
					IPSubnet: cns.IPSubnet{
						IPAddress:    "240.1.2.1",
						PrefixLength: 24,
					},
				},
				SecondaryIPConfigs: map[string]cns.SecondaryIPConfig{
					"abc": {
						IPAddress: "240.1.2.7",
					},
				},
				HostPrimaryIP: "10.0.0.4",
			},
			preExistingRules: []preExistingRule{
				{
					// rule at higher priority means nothing happens
					table: iptables.Nat,
					chain: iptables.Postrouting,
					rule:  []string{"-j", "SWIFT-POSTROUTING"},
				},
				{
					table: iptables.Nat,
					chain: iptables.Postrouting,
					rule:  []string{"-j", "SWIFT"},
				},
				{
					table: iptables.Nat,
					chain: SWIFTPOSTROUTING,
					rule: []string{
						"-m", "addrtype", "!", "--dst-type", "local", "-s", "240.1.2.0/24", "-d", networkutils.AzureDNS,
						"-p", "udp", "--dport", strconv.Itoa(iptables.DNSPort), "-j", "SNAT", "--to", "10.0.0.4",
					},
				},
				{
					table: iptables.Nat,
					chain: SWIFTPOSTROUTING,
					rule: []string{
						"-m", "addrtype", "!", "--dst-type", "local", "-s", "240.1.2.0/24", "-d", networkutils.AzureDNS,
						"-p", "tcp", "--dport", strconv.Itoa(iptables.DNSPort), "-j", "SNAT", "--to", "10.0.0.4",
					},
				},
				{
					table: iptables.Nat,
					chain: SWIFTPOSTROUTING,
					rule: []string{
						"-m", "addrtype", "!", "--dst-type", "local", "-s", "240.1.2.0/24", "-d", networkutils.AzureIMDS,
						"-p", "tcp", "--dport", strconv.Itoa(iptables.HTTPPort), "-j", "SNAT", "--to", "10.0.0.4",
					},
				},
				{
					table: iptables.Nat,
					chain: "SWIFT",
					rule: []string{
						"-m", "addrtype", "!", "--dst-type", "local", "-s", "240.1.2.0/24", "-d", networkutils.AzureDNS,
						"-p", "udp", "--dport", strconv.Itoa(iptables.DNSPort), "-j", "SNAT", "--to", "192.1.2.1",
					},
				},
			},
			expectedChains: []chainExpectation{
				{
					table: iptables.Nat,
					chain: SWIFTPOSTROUTING,
					expected: []string{
						"-N SWIFT-POSTROUTING",
						"-A SWIFT-POSTROUTING -m addrtype ! --dst-type local -s 240.1.2.0/24 -d " + networkutils.AzureDNS + " -p udp --dport " + strconv.Itoa(iptables.DNSPort) + " -j SNAT --to 10.0.0.4",
						"-A SWIFT-POSTROUTING -m addrtype ! --dst-type local -s 240.1.2.0/24 -d " + networkutils.AzureDNS + " -p tcp --dport " + strconv.Itoa(iptables.DNSPort) + " -j SNAT --to 10.0.0.4",
						"-A SWIFT-POSTROUTING -m addrtype ! --dst-type local -s 240.1.2.0/24 -d " + networkutils.AzureIMDS + " -p tcp --dport " + strconv.Itoa(iptables.HTTPPort) + " -j SNAT --to 10.0.0.4",
					},
				},
				{
					table: iptables.Nat,
					chain: iptables.Postrouting,
					expected: []string{
						"-P POSTROUTING ACCEPT",
						"-A POSTROUTING -j SWIFT-POSTROUTING",
						"-A POSTROUTING -j SWIFT",
					},
				},
				{
					// stale old rule can remain
					table: iptables.Nat,
					chain: "SWIFT",
					expected: []string{
						"-N SWIFT",
						"-A SWIFT -m addrtype ! --dst-type local -s 240.1.2.0/24 -d " + networkutils.AzureDNS + " -p udp --dport " + strconv.Itoa(iptables.DNSPort) + " -j SNAT --to 192.1.2.1",
					},
				},
			},
			expectedClearChainCalls: 0,
		},
		{
			// in vnet scale, the primary nic ip becomes the node ip (diff address space from pod subnet)
			name: "vnet scale",
			input: &cns.CreateNetworkContainerRequest{
				NetworkContainerid: ncID,
				IPConfiguration: cns.IPConfiguration{
					IPSubnet: cns.IPSubnet{
						IPAddress:    "10.0.0.4",
						PrefixLength: 28,
					},
				},
				SecondaryIPConfigs: map[string]cns.SecondaryIPConfig{
					"abc": {
						IPAddress: "240.1.2.15",
					},
				},
				HostPrimaryIP: "10.0.0.4",
			},
			expectedChains: []chainExpectation{
				{
					table: iptables.Nat,
					chain: SWIFTPOSTROUTING,
					expected: []string{
						"-N SWIFT-POSTROUTING",
						"-A SWIFT-POSTROUTING -m addrtype ! --dst-type local -s 240.1.2.0/28 -d " + networkutils.AzureDNS + " -p udp --dport " + strconv.Itoa(iptables.DNSPort) + " -j SNAT --to 10.0.0.4",
						"-A SWIFT-POSTROUTING -m addrtype ! --dst-type local -s 240.1.2.0/28 -d " + networkutils.AzureDNS + " -p tcp --dport " + strconv.Itoa(iptables.DNSPort) + " -j SNAT --to 10.0.0.4",
						"-A SWIFT-POSTROUTING -m addrtype ! --dst-type local -s 240.1.2.0/28 -d " + networkutils.AzureIMDS + " -p tcp --dport " + strconv.Itoa(iptables.HTTPPort) + " -j SNAT --to 10.0.0.4",
					},
				},
				{
					table: iptables.Nat,
					chain: iptables.Postrouting,
					expected: []string{
						"-P POSTROUTING ACCEPT",
						"-A POSTROUTING -j SWIFT-POSTROUTING",
					},
				},
			},
			expectedClearChainCalls: 1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			service := getTestService(cns.KubernetesCRD)
			ipt := newIPTablesMock()
			iptl := &ipTablesLegacyMock{}
			service.iptables = &fakeIPTablesProvider{
				iptables:       ipt,
				iptablesLegacy: iptl,
			}

			// setup pre-existing rules
			if len(tt.preExistingRules) > 0 {
				for _, preRule := range tt.preExistingRules {
					chainExists, _ := ipt.ChainExists(preRule.table, preRule.chain)

					if !chainExists {
						err := ipt.NewChain(preRule.table, preRule.chain)
						if err != nil {
							t.Fatal("failed to setup pre-existing rule chain:", err)
						}
					}

					err := ipt.Append(preRule.table, preRule.chain, preRule.rule...)
					if err != nil {
						t.Fatal("failed to setup pre-existing rule:", err)
					}
				}
			}

			resp, msg := service.programSNATRules(tt.input)
			if resp != types.Success {
				t.Fatal("failed to program snat rules", msg)
			}

			// verify chain contents using List
			for _, chainExp := range tt.expectedChains {
				actualRules, err := ipt.List(chainExp.table, chainExp.chain)
				if err != nil {
					t.Fatal("failed to list rules for chain", chainExp.chain, ":", err)
				}

				if len(actualRules) != len(chainExp.expected) {
					t.Fatalf("chain %s rule count mismatch: got %d, expected %d\nActual: %v\nExpected: %v",
						chainExp.chain, len(actualRules), len(chainExp.expected), actualRules, chainExp.expected)
				}

				for i, expectedRule := range chainExp.expected {
					if actualRules[i] != expectedRule {
						t.Fatalf("chain %s rule %d mismatch:\nActual:   %s\nExpected: %s",
							chainExp.chain, i, actualRules[i], expectedRule)
					}
				}
			}

			// verify ClearChain was called the expected number of times
			actualClearChainCalls := ipt.ClearChainCallCount()
			if actualClearChainCalls != tt.expectedClearChainCalls {
				t.Fatalf("ClearChain call count mismatch: got %d, expected %d", actualClearChainCalls, tt.expectedClearChainCalls)
			}

			// verify we delete legacy swift postrouting jump
			actualLegacyDeleteCalls := iptl.DeleteCallCount()
			if actualLegacyDeleteCalls != 1 {
				t.Fatalf("Delete call count mismatch: got %d, expected 1", actualLegacyDeleteCalls)
			}
		})
	}
}
