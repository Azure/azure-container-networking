package ipsets

import testutils "github.com/Azure/azure-container-networking/test/utils"

var (
	ipsetRestoreStringSlice = []string{"ipset", "restore"}
	ipsetSaveStringSlice    = []string{"ipset", "save"}

	fakeRestoreSuccessCommand = testutils.TestCmd{
		Cmd:      ipsetRestoreStringSlice,
		Stdout:   "success",
		ExitCode: 0,
	}
)

func GetApplyIPSetsTestCalls(toAddOrUpdateIPSets, toDeleteIPSets []*IPSetMetadata) []testutils.TestCmd {
	if len(toAddOrUpdateIPSets) > 0 {
		return []testutils.TestCmd{
			{Cmd: ipsetSaveStringSlice, PipedToCommand: true},
			{Cmd: []string{"grep", "azure-npm-"}, ExitCode: 1}, // grep didn't find anything
			{Cmd: ipsetRestoreStringSlice},
		}
	}
	return []testutils.TestCmd{fakeRestoreSuccessCommand}
}

func GetResetTestCalls() []testutils.TestCmd {
	return []testutils.TestCmd{
		{Cmd: []string{"ipset", "list", "--name"}, PipedToCommand: true},
		{Cmd: []string{"grep", "azure-npm-"}, ExitCode: 1}, // grep didn't find anything
	}
}
