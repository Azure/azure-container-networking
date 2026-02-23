// Copyright Microsoft. All rights reserved.
// MIT License

package nodesetup

import (
	"net"

	"github.com/Azure/azure-container-networking/cns/iprule"
	"github.com/Azure/azure-container-networking/network/networkutils"
	"github.com/pkg/errors"
	"go.uber.org/zap"
	"golang.org/x/sys/unix"
)

const (
	// highestIPRulePriority is the priority for the ip rules that route infrastructure
	// traffic (wireserver, IMDS) through the main routing table.
	highestIPRulePriority = 0
)

// listIPRules and addIPRule are package-level variables to allow test injection.
var (
	listIPRules = iprule.ListIPRules
	addIPRule   = iprule.AddIPRule
)

// Run performs one-time node-level setup.
func Run(z *zap.Logger) error {
	return programWireserverIPRules(z)
}

// programWireserverIPRules programs ip rules for infrastructure traffic.
func programWireserverIPRules(z *zap.Logger) error {
	// For scenarios like Prefix on NIC v6 with Cilium CNI based on SwiftV2, pod traffic may be routed
	// through eth1 (delegated NIC). These rules ensure critical traffic (e.g. wireserver,
	// IMDS) is routed through eth0 (infra NIC) via the main routing table.
	dstIPs := []string{networkutils.AzureDNS, networkutils.AzureIMDS}

	z.Info("ensuring ip rules for critical infrastructure traffic", zap.Strings("dstIPs", dstIPs))

	var rules []iprule.IPRule
	for _, ip := range dstIPs {
		r, err := ipRulesForDst(ip, highestIPRulePriority)
		if err != nil {
			return err
		}
		rules = append(rules, r...)
	}

	if len(rules) == 0 {
		return nil
	}

	existing, err := listIPRules()

	z.Info("fetched existing ip rules", zap.Int("count", len(existing)))
	if err != nil {
		return errors.Wrap(err, "failed to list existing ip rules")
	}

	for i := range rules {
		if err := ensureIPRule(rules[i], existing, z); err != nil {
			return err
		}

		z.Info("ensured ip rule exists", zap.String("dst", rules[i].Dst.String()), zap.Int("table", rules[i].Table), zap.Int("priority", rules[i].Priority))
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
