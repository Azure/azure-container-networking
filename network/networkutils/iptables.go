package networkutils

import "github.com/Azure/azure-container-networking/iptables"

type IPTablesClientInterface interface {
	RunCmd(version, params string) error
	ChainExists(version, tableName, chainName string) bool
	GetCreateChainCmd(version, tableName, chainName string) iptables.IPTableEntry
	CreateChain(version, tableName, chainName string) error
	RuleExists(version, tableName, chainName, match, target string) bool
	GetInsertIptableRuleCmd(version, tableName, chainName, match, target string) iptables.IPTableEntry
	InsertIptableRule(version, tableName, chainName, match, target string) error
	GetAppendIptableRuleCmd(version, tableName, chainName, match, target string) iptables.IPTableEntry
	AppendIptableRule(version, tableName, chainName, match, target string) error
	DeleteIptableRule(version, tableName, chainName, match, target string) error
}
