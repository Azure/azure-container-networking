package policies

import testutils "github.com/Azure/azure-container-networking/test/utils"

func GetAddPolicyTestCalls(_ *NPMNetworkPolicy) []testutils.TestCmd {
	return []testutils.TestCmd{}
}

func GetRemovePolicyTestCalls(_ *NPMNetworkPolicy) []testutils.TestCmd {
	return []testutils.TestCmd{}
}

func GetBootupTestCalls(_ bool) []testutils.TestCmd {
	return []testutils.TestCmd{}
}
