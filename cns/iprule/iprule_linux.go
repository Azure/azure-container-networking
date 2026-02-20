// Copyright Microsoft. All rights reserved.
// MIT License

package iprule

import (
	"net"

	"github.com/pkg/errors"
	vishnetlink "github.com/vishvananda/netlink"
)

// IPRule is a simple representation of an IP routing rule,
// decoupled from the underlying netlink implementation.
type IPRule struct {
	Dst      *net.IPNet
	Table    int
	Priority int
}

// ListIPRules returns all IPv4 ip rules on the host.
func ListIPRules() ([]IPRule, error) {
	rules, err := vishnetlink.RuleList(vishnetlink.FAMILY_V4)
	if err != nil {
		return nil, errors.Wrap(err, "failed to list ip rules")
	}
	result := make([]IPRule, len(rules))
	for i := range rules {
		result[i] = IPRule{
			Dst:      rules[i].Dst,
			Table:    rules[i].Table,
			Priority: rules[i].Priority,
		}
	}
	return result, nil
}

// AddIPRule programs a single ip rule via netlink.
func AddIPRule(rule IPRule) error {
	nlRule := vishnetlink.NewRule()
	nlRule.Dst = rule.Dst
	nlRule.Table = rule.Table
	nlRule.Priority = rule.Priority
	return errors.Wrap(vishnetlink.RuleAdd(nlRule), "failed to add ip rule")
}
