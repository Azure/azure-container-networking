package network

// mockIPTablesClient is a mock for the ipTablesClient interface that tracks calls.
type mockIPTablesClient struct {
	insertCalls []iptablesCall
}

type iptablesCall struct {
	version   string
	tableName string
	chainName string
	match     string
	target    string
}

func (c *mockIPTablesClient) InsertIptableRule(version, tableName, chainName, match, target string) error {
	c.insertCalls = append(c.insertCalls, iptablesCall{version, tableName, chainName, match, target})
	return nil
}

func (c *mockIPTablesClient) AppendIptableRule(_, _, _, _, _ string) error { return nil }
func (c *mockIPTablesClient) DeleteIptableRule(_, _, _, _, _ string) error { return nil }
func (c *mockIPTablesClient) CreateChain(_, _, _ string) error             { return nil }
func (c *mockIPTablesClient) RunCmd(_, _ string) error                     { return nil }
