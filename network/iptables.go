package network

type ipTablesClient interface {
	InsertIptableRule(version, tableName, chainName, match, target string) error
	AppendIptableRule(version, tableName, chainName, match, target string) error
	DeleteIptableRule(version, tableName, chainName, match, target string) error
	RuleExists(version, tableName, chainName, match, target string) bool
	CreateChain(version, tableName, chainName string) error
	RunCmd(version, params string) error
	// DeleteIptableRuleIfExists deletes the rule and swallows ONLY the
	// kernel "rule does not exist" error. Prefer this over the
	// RuleExists(...)+DeleteIptableRule(...) pattern in cleanup paths so
	// that transient check errors (xtables lock, permissions) don't get
	// misread as "rule absent" and silently skip the delete.
	DeleteIptableRuleIfExists(version, tableName, chainName, match, target string) error
}
