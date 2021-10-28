package policies

import (
	"github.com/Azure/azure-container-networking/npm/util"
	testutils "github.com/Azure/azure-container-networking/test/utils"
)

func getAddPolicyTestCalls(_ *NPMNetworkPolicy) []testutils.TestCmd {
	return []testutils.TestCmd{fakeIPTablesRestoreCommand}
}

func getRemovePolicyTestCalls(policy *NPMNetworkPolicy) []testutils.TestCmd {
	calls := []testutils.TestCmd{}
	hasIngress, hasEgress := policy.hasIngressAndEgress()
	if hasIngress {
		deleteIngressJumpSpecs := []string{"iptables", "-w", "60", "-D", util.IptablesAzureIngressChain}
		deleteIngressJumpSpecs = append(deleteIngressJumpSpecs, getIngressJumpSpecs(policy)...)
		calls = append(calls, testutils.TestCmd{Cmd: deleteIngressJumpSpecs})
	}
	if hasEgress {
		deleteEgressJumpSpecs := []string{"iptables", "-w", "60", "-D", util.IptablesAzureEgressChain}
		deleteEgressJumpSpecs = append(deleteEgressJumpSpecs, getEgressJumpSpecs(policy)...)
		calls = append(calls, testutils.TestCmd{Cmd: deleteEgressJumpSpecs})
	}

	calls = append(calls, fakeIPTablesRestoreCommand)
	return calls
}
