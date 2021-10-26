package policies

import (
	testutils "github.com/Azure/azure-container-networking/test/utils"
)

func getAddPolicyTestCalls(_ *NPMNetworkPolicy) []testutils.TestCmd {
	return []testutils.TestCmd{fakeIPTablesRestoreCommand}
}

func getRemovePolicyTestCalls(policy *NPMNetworkPolicy) []testutils.TestCmd {
	deleteIngressJumpSpecs := []string{"iptables", "-w", "60", "-X"}
	deleteIngressJumpSpecs = append(deleteIngressJumpSpecs, getIngressJumpSpecs(policy)...)
	deleteEgressJumpSpecs := []string{"iptables", "-w", "60", "-X"}
	deleteEgressJumpSpecs = append(deleteEgressJumpSpecs, getEgressJumpSpecs(policy)...)

	return []testutils.TestCmd{
		fakeIPTablesRestoreCommand,
		{Cmd: deleteIngressJumpSpecs},
		{Cmd: deleteEgressJumpSpecs},
		fakeIPTablesRestoreCommand,
	}
}
