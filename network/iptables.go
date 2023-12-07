package network

type ipTablesClient interface {
	InsertIptableRule(version, tableName, chainName, match, target string) error
	AppendIptableRule(version, tableName, chainName, match, target string) error
	DeleteIptableRule(version, tableName, chainName, match, target string) error
	CreateChain(version, tableName, chainName string) error
	RunCmd(version, params string) error
}
