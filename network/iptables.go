package network

type ipTablesClient interface {
	InsertIptableRule(version, tableName, chainName, match, target string) error
	AppendIptableRule(version, tableName, chainName, match, target string) error
	DeleteIptableRule(version, tableName, chainName, match, target string) error
	RuleExists(version, tableName, chainName, match, target string) bool
	CreateChain(version, tableName, chainName string) error
	RunCmd(version, params string) error
}
