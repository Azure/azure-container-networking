package ipsets

import testutils "github.com/Azure/azure-container-networking/test/utils"

var fakeRestoreSuccessCommand = testutils.TestCmd{
	Cmd:      []string{"ipset", "restore"},
	Stdout:   "success",
	ExitCode: 0,
}

func getMockIOShim(calls []testutils.TestCmd) *common.IOShim {
	return common.NewMockIOShim([]testutils.TestCmd{})
}

func GetApplyIPSetsTestCalls(toAddOrUpdateIPSets, toDeleteIPSets []*IPSetMetadata) []testutils.TestCmd {
	// TODO eventually call ipset save if there are toAddOrUpdateIPSets
	return []testutils.TestCmd{fakeRestoreSuccessCommand}
}

func GetResetTestCalls() []testutils.TestCmd {
	return []testutils.TestCmd{}
}
