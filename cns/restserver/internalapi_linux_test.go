// Copyright 2020 Microsoft. All rights reserved.
// MIT License

package restserver

import (
	"strconv"
	"testing"

	"github.com/Azure/azure-container-networking/cns"
	"github.com/Azure/azure-container-networking/cns/fakes"
	"github.com/Azure/azure-container-networking/cns/types"
	"github.com/Azure/azure-container-networking/iptables"
	"github.com/Azure/azure-container-networking/network/networkutils"
)

type FakeIPTablesProvider struct {
	iptables *fakes.IPTablesMock
}

func (c *FakeIPTablesProvider) GetIPTables() (iptablesClient, error) {
	// persist iptables in testing
	if c.iptables == nil {
		c.iptables = fakes.NewIPTablesMock()
	}
	return c.iptables, nil
}

func TestAddSNATRules(t *testing.T) {
	type expectedScenario struct {
		table string
		chain string
		rule  []string
	}

	tests := []struct {
		name     string
		input    *cns.CreateNetworkContainerRequest
		expected []expectedScenario
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
			expected: []expectedScenario{
				{
					table: iptables.Nat,
					chain: SWIFT,
					rule: []string{
						"-m", "addrtype", "!", "--dst-type", "local", "-s", "240.1.2.0/24", "-d",
						networkutils.AzureDNS, "-p", iptables.UDP, "--dport", strconv.Itoa(iptables.DNSPort), "-j", iptables.Snat, "--to", "240.1.2.1",
					},
				},
				{
					table: iptables.Nat,
					chain: SWIFT,
					rule: []string{
						"-m", "addrtype", "!", "--dst-type", "local", "-s", "240.1.2.0/24", "-d",
						networkutils.AzureDNS, "-p", iptables.TCP, "--dport", strconv.Itoa(iptables.DNSPort), "-j", iptables.Snat, "--to", "240.1.2.1",
					},
				},
				{
					table: iptables.Nat,
					chain: SWIFT,
					rule: []string{
						"-m", "addrtype", "!", "--dst-type", "local", "-s", "240.1.2.0/24", "-d",
						networkutils.AzureIMDS, "-p", iptables.TCP, "--dport", strconv.Itoa(iptables.HTTPPort), "-j", iptables.Snat, "--to", "10.0.0.4",
					},
				},
			},
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
			expected: []expectedScenario{
				{
					table: iptables.Nat,
					chain: SWIFT,
					rule: []string{
						"-m", "addrtype", "!", "--dst-type", "local", "-s", "240.1.2.0/28", "-d",
						networkutils.AzureDNS, "-p", iptables.UDP, "--dport", strconv.Itoa(iptables.DNSPort), "-j", iptables.Snat, "--to", "10.0.0.4",
					},
				},
				{
					table: iptables.Nat,
					chain: SWIFT,
					rule: []string{
						"-m", "addrtype", "!", "--dst-type", "local", "-s", "240.1.2.0/28", "-d",
						networkutils.AzureDNS, "-p", iptables.TCP, "--dport", strconv.Itoa(iptables.DNSPort), "-j", iptables.Snat, "--to", "10.0.0.4",
					},
				},
				{
					table: iptables.Nat,
					chain: SWIFT,
					rule: []string{
						"-m", "addrtype", "!", "--dst-type", "local", "-s", "240.1.2.0/28", "-d",
						networkutils.AzureIMDS, "-p", iptables.TCP, "--dport", strconv.Itoa(iptables.HTTPPort), "-j", iptables.Snat, "--to", "10.0.0.4",
					},
				},
			},
		},
	}

	for _, tt := range tests {
		service := getTestService()
		service.iptables = &FakeIPTablesProvider{}
		resp, msg := service.programSNATRules(tt.input)
		if resp != types.Success {
			t.Fatal("failed to program snat rules", msg, " case: ", tt.name)
		}
		finalState, _ := service.iptables.GetIPTables()
		for _, ex := range tt.expected {
			exists, err := finalState.Exists(ex.table, ex.chain, ex.rule...)
			if err != nil || !exists {
				t.Fatal("rule not found", ex.rule, " case: ", tt.name)
			}
		}
	}
}
