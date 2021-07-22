package iptable

import (
	"fmt"
)

type Iptables struct {
	name   string
	chains map[string]*IptablesChain
}

// Constructor
func NewIptables(name string, chains map[string]*IptablesChain) *Iptables {
	iptable := &Iptables{}
	iptable.name = name
	iptable.chains = chains
	return iptable
}

func (t *Iptables) Name() string {
	return t.name
}

func (t *Iptables) SetName(name string) {
	t.name = name
}

func (t *Iptables) Chains() map[string]*IptablesChain {
	return t.chains
}

func (t *Iptables) SetChains(chain_m map[string]*IptablesChain) {
	t.chains = chain_m
}

type IptablesChain struct {
	name  string
	data  []byte
	rules []*IptablesRule
}

func NewIptablesChain(name string, data []byte, rules []*IptablesRule) *IptablesChain {
	iptableChain := &IptablesChain{}
	iptableChain.name = name
	iptableChain.data = data
	iptableChain.rules = rules
	return iptableChain
}

func (c *IptablesChain) Name() string {
	return c.name
}

func (c *IptablesChain) SetName(name string) {
	c.name = name
}

func (c *IptablesChain) Data() []byte {
	return c.data
}

func (c *IptablesChain) SetData(d []byte) {
	c.data = d
}

func (c *IptablesChain) Rules() []*IptablesRule {
	return c.rules
}

func (c *IptablesChain) SetRules(rules []*IptablesRule) {
	c.rules = rules
}

type IptablesRule struct {
	protocol string
	target   *Target
	modules  []*Module
}

func NewIptablesRule(protocol string, target *Target, modules []*Module) *IptablesRule {
	iptablerule := &IptablesRule{}
	iptablerule.protocol = protocol
	iptablerule.target = target
	iptablerule.modules = modules
	return iptablerule
}

func (r *IptablesRule) Protocol() string {
	return r.protocol
}

func (r *IptablesRule) SetProtocol(p string) {
	r.protocol = p
}

func (r *IptablesRule) Target() *Target {
	return r.target
}

func (r *IptablesRule) SetTarget(t *Target) {
	r.target = t
}

func (r *IptablesRule) Modules() []*Module {
	return r.modules
}

func (r *IptablesRule) SetModules(m_list []*Module) {
	r.modules = m_list
}

type Module struct {
	verb           string
	optionValueMap map[string][]string
}

func NewModule(verb string, optionValueMap map[string][]string) *Module {
	module := &Module{}
	module.verb = verb
	module.optionValueMap = optionValueMap
	return module
}

func (m *Module) Verb() string {
	return m.verb
}

func (m *Module) SetVerb(v string) {
	m.verb = v
}

func (m *Module) OptionValueMap() map[string][]string {
	return m.optionValueMap
}

func (m *Module) SetOptionValueMap(optionvaluemap map[string][]string) {
	m.optionValueMap = optionvaluemap
}

type Target struct {
	name           string
	optionValueMap map[string][]string
}

func NewTarget(name string, optionValueMap map[string][]string) *Target {
	target := &Target{}
	target.name = name
	target.optionValueMap = optionValueMap
	return target
}

func (t *Target) Name() string {
	return t.name
}
func (t *Target) SetName(name string) {
	t.name = name
}

func (t *Target) OptionValueMap() map[string][]string {
	return t.optionValueMap
}
func (t *Target) SetOptionValueMap(optionvaluemap map[string][]string) {
	t.optionValueMap = optionvaluemap
}

// for debugging
func (t *Iptables) PrintIptable() {
	fmt.Printf("IPTABLE NAME - %v\n", t.Name())
	t.printIptableChains()
}

func (t *Iptables) printIptableChains() {
	for k, v := range t.Chains() {
		fmt.Printf("	IPTABLE CHAIN NAME - %v\n", k)
		t.printIptableChainRules(v)
	}
}

func (t *Iptables) printIptableChainRules(chain *IptablesChain) {
	for k, v := range chain.Rules() {
		fmt.Printf("		RULE %v\n", k)
		fmt.Printf("			RULE'S PROTOCOL - %v\n", v.Protocol())
		t.printIptableRuleModules(v.Modules())
		t.printIptableRuleTarget(v.Target())

	}
}

func (t *Iptables) printIptableRuleModules(m_list []*Module) {
	fmt.Printf("			RULE'S MODULES\n")
	for i, v := range m_list {
		fmt.Printf("				Module %v\n", i)
		fmt.Printf("					Verb - %v\n", v.Verb())
		fmt.Printf("					OptionValueMap - %+v\n", v.OptionValueMap())
	}
}

func (t *Iptables) printIptableRuleTarget(target *Target) {
	fmt.Printf("			RULE'S TARGET\n")
	fmt.Printf("					NAME - %v\n", target.Name())
	fmt.Printf("					OptionValueMap - %+v\n", target.OptionValueMap())
}
