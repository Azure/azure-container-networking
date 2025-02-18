package fakes

import (
	"errors"
	"strings"

	"github.com/Azure/azure-container-networking/iptables"
)

var (
	errChainExists   = errors.New("chain already exists")
	errChainNotFound = errors.New("chain not found")
	errRuleExists    = errors.New("rule already exists")
)

type IPTablesMock struct {
	state map[string]map[string][]string
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

	chainExists, _ := c.ChainExists(table, chain)
	if !chainExists {
		return errChainNotFound
	}

	ruleExists, _ := c.Exists(table, chain, rulespec...)
	if ruleExists {
		return errRuleExists
	}

	targetRule := strings.Join(rulespec, " ")
	c.state[table][chain] = append(c.state[table][chain], targetRule)
	return nil
}

func (c *IPTablesMock) Insert(table, chain string, _ int, rulespec ...string) error {
	return c.Append(table, chain, rulespec...)
}
