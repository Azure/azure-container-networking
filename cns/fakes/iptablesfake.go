package fakes

import (
	"errors"
	"fmt"
	"strings"

	"github.com/Azure/azure-container-networking/iptables"
)

var (
	errChainExists   = errors.New("chain already exists")
	errChainNotFound = errors.New("chain not found")
	errRuleExists    = errors.New("rule already exists")
	errRuleNotFound  = errors.New("rule not found")
	errIndexBounds   = errors.New("index out of bounds")
)

type IPTablesLegacyMock struct {
	deleteCallCount int
}

func (c *IPTablesLegacyMock) Delete(_, _ string, _ ...string) error {
	c.deleteCallCount++
	return nil
}

func (c *IPTablesLegacyMock) DeleteCallCount() int {
	return c.deleteCallCount
}

type IPTablesMock struct {
	state               map[string]map[string][]string
	clearChainCallCount int
}

func NewIPTablesMock() *IPTablesMock {
	return &IPTablesMock{
		state: make(map[string]map[string][]string),
	}
}

func (c *IPTablesMock) ensureTableExists(table string) {
	_, exists := c.state[table]
	if !exists {
		c.state[table] = make(map[string][]string)
	}
}

func (c *IPTablesMock) ChainExists(table, chain string) (bool, error) {
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

func (c *IPTablesMock) NewChain(table, chain string) error {
	c.ensureTableExists(table)

	exists, _ := c.ChainExists(table, chain)

	if exists {
		return errChainExists
	}

	c.state[table][chain] = []string{}
	return nil
}

func (c *IPTablesMock) Exists(table, chain string, rulespec ...string) (bool, error) {
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

func (c *IPTablesMock) Append(table, chain string, rulespec ...string) error {
	c.ensureTableExists(table)

	chainRules := c.state[table][chain]
	return c.Insert(table, chain, len(chainRules)+1, rulespec...)
}

func (c *IPTablesMock) Insert(table, chain string, pos int, rulespec ...string) error {
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

func (c *IPTablesMock) List(table, chain string) ([]string, error) {
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

func (c *IPTablesMock) ClearChain(table, chain string) error {
	c.clearChainCallCount++
	c.ensureTableExists(table)

	chainExists, _ := c.ChainExists(table, chain)
	if !chainExists {
		return errChainNotFound
	}

	c.state[table][chain] = []string{}
	return nil
}

func (c *IPTablesMock) Delete(table, chain string, rulespec ...string) error {
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

func (c *IPTablesMock) ClearChainCallCount() int {
	return c.clearChainCallCount
}
