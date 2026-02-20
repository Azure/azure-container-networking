// Copyright Microsoft. All rights reserved.
// MIT License

package nodesetup

import (
	"net"

	"github.com/Azure/azure-container-networking/cns/iprule"
	"github.com/pkg/errors"
	"go.uber.org/zap"
	"golang.org/x/sys/unix"
)

const (
	// wireserverRulePriority is the priority for the ip rule that routes wireserver traffic.
	// This ensures wireserver traffic goes through eth0 (infra NIC) even when other rules are added.
	wireserverRulePriority = 0
)

// listIPRules and addIPRule are package-level variables to allow test injection.
var (
	listIPRules = iprule.ListIPRules
	addIPRule   = iprule.AddIPRule
)

// Run performs one-time node-level setup.
func (nc *NodeConfiguration) Run() error {
	// For scenarios like Prefix on NIC v6 with Cilium CNI, pod traffic may be routed
	// through eth1 (delegated NIC). These rules ensure critical traffic (e.g. wireserver)
	// is routed through eth0 (infra NIC) via the main routing table.
	rules, err := ipRulesForDst(nc.config.WireserverIP, wireserverRulePriority)
	if err != nil {
		return err
	}

	if len(rules) == 0 {
		return nil
	}

	existing, err := listIPRules()
	if err != nil {
		return errors.Wrap(err, "failed to list existing ip rules")
	}

	for i := range rules {
		if err := ensureIPRule(rules[i], existing, nc.logger); err != nil {
			return err
		}
	}
	return nil
}

// ipRulesForDst builds ip rules to route traffic for a destination IP through the main routing table.
func ipRulesForDst(ip string, priority int) ([]iprule.IPRule, error) {
	_, dstNet, err := net.ParseCIDR(ip + "/32")
	if err != nil {
		return nil, errors.Wrapf(err, "failed to parse IP %s", ip)
	}
	return []iprule.IPRule{
		{Dst: dstNet, Table: unix.RT_TABLE_MAIN, Priority: priority},
	}, nil
}

// ensureIPRule programs a single ip rule if it does not already exist in the provided set.
func ensureIPRule(rule iprule.IPRule, existing []iprule.IPRule, z *zap.Logger) error {
	for _, r := range existing {
		if r.Dst != nil && rule.Dst != nil && r.Dst.String() == rule.Dst.String() &&
			r.Table == rule.Table && r.Priority == rule.Priority {
			z.Info("ip rule already exists", zap.String("dst", rule.Dst.String()), zap.Int("table", rule.Table), zap.Int("priority", rule.Priority))
			return nil
		}
	}

	if err := addIPRule(rule); err != nil {
		return errors.Wrapf(err, "failed to add ip rule to %s table %d priority %d", rule.Dst, rule.Table, rule.Priority)
	}

	z.Info("added ip rule", zap.String("dst", rule.Dst.String()), zap.Int("table", rule.Table), zap.Int("priority", rule.Priority))
	return nil
}
