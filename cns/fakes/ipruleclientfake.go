package fakes

import (
	"net"

	"github.com/Azure/azure-container-networking/cns/restserver"
)

// IPRuleClientFake is a mock implementation of restserver.IPRuleClient for testing.
type IPRuleClientFake struct {
	Rules       []restserver.IPRule
	RuleListErr error
	RuleAddErr  error
	AddedRules  []restserver.IPRule
}

// NewIPRuleClientFake creates a new IPRuleClientFake.
func NewIPRuleClientFake() *IPRuleClientFake {
	return &IPRuleClientFake{
		Rules:      []restserver.IPRule{},
		AddedRules: []restserver.IPRule{},
	}
}

// RuleList returns the configured rules or error.
func (f *IPRuleClientFake) RuleList(_ int) ([]restserver.IPRule, error) {
	if f.RuleListErr != nil {
		return nil, f.RuleListErr
	}
	return f.Rules, nil
}

// RuleAdd adds a rule to the AddedRules slice or returns error.
func (f *IPRuleClientFake) RuleAdd(rule *restserver.IPRule) error {
	if f.RuleAddErr != nil {
		return f.RuleAddErr
	}
	f.AddedRules = append(f.AddedRules, *rule)
	return nil
}

// AddExistingRule adds a rule to the existing rules list for testing.
func (f *IPRuleClientFake) AddExistingRule(dst *net.IPNet, table, priority int) {
	f.Rules = append(f.Rules, restserver.IPRule{
		Dst:      dst,
		Table:    table,
		Priority: priority,
	})
}
