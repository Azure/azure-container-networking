package ipsets

import testutils "github.com/Azure/azure-container-networking/test/utils"

func GetApplyIPSetsTestCalls(_, _ []*IPSetMetadata) []testutils.TestCmd {
	return []testutils.TestCmd{}
}

func GetResetTestCalls() []testutils.TestCmd {
	return []testutils.TestCmd{}
}
