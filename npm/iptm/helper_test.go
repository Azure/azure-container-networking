package iptm

import (
	"testing"

	"github.com/Azure/azure-container-networking/npm/util"
)

func TestGetAllChainsAndRules(t *testing.T) {
	allChainsandRules := getAllDefaultRules()

	parentNpmRulesCount := 6

	if len(allChainsandRules[0]) > 3 {
		t.Errorf("TestGetAllChainsAndRules failed @ INGRESS target check")
	}

	if len(allChainsandRules[1]) > 3 {
		t.Errorf("TestGetAllChainsAndRules failed @ EGRESS target check")
	}

	for i, rule := range allChainsandRules {
		if i == parentNpmRulesCount {
			break
		}
		// make sure the ordering is correct
		// first 7 rules should be parent chain rules
		if rule[0] != util.IptablesAzureChain {
			t.Errorf("TestGetAllChainsAndRules failed @ AzureNpmChain rule count check")
		}
	}
}
