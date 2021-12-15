package policies

import (
	"strings"

	"github.com/Azure/azure-container-networking/npm/util"
	testutils "github.com/Azure/azure-container-networking/test/utils"
)

var (
	fakeIPTablesRestoreCommand        = testutils.TestCmd{Cmd: []string{"iptables-restore", "-w", "60", "-T", "filter", "--noflush"}}
	fakeIPTablesRestoreFailureCommand = testutils.TestCmd{Cmd: []string{"iptables-restore", "-w", "60", "-T", "filter", "--noflush"}, ExitCode: 1}

	listLineNumbersCommandStrings = []string{"iptables", "-w", "60", "-t", "filter", "-n", "-L", "FORWARD", "--line-numbers"}
	listAllCommandStrings         = []string{"iptables", "-w", "60", "-t", "filter", "-n", "-L"}
)

func GetAddPolicyTestCalls(_ *NPMNetworkPolicy) []testutils.TestCmd {
	return []testutils.TestCmd{fakeIPTablesRestoreCommand}
}

func GetAddPolicyFailureTestCalls(_ *NPMNetworkPolicy) []testutils.TestCmd {
	return []testutils.TestCmd{fakeIPTablesRestoreFailureCommand}
}

func GetRemovePolicyTestCalls(policy *NPMNetworkPolicy) []testutils.TestCmd {
	calls := []testutils.TestCmd{}
	hasIngress, hasEgress := policy.hasIngressAndEgress()
	if hasIngress {
		deleteIngressJumpSpecs := []string{"iptables", "-w", "60", "-D", util.IptablesAzureIngressChain}
		deleteIngressJumpSpecs = append(deleteIngressJumpSpecs, ingressJumpSpecs(policy)...)
		calls = append(calls, testutils.TestCmd{Cmd: deleteIngressJumpSpecs})
	}
	if hasEgress {
		deleteEgressJumpSpecs := []string{"iptables", "-w", "60", "-D", util.IptablesAzureEgressChain}
		deleteEgressJumpSpecs = append(deleteEgressJumpSpecs, egressJumpSpecs(policy)...)
		calls = append(calls, testutils.TestCmd{Cmd: deleteEgressJumpSpecs})
	}

	calls = append(calls, fakeIPTablesRestoreCommand)
	return calls
}

// GetRemovePolicyFailureTestCalls fails on the restore
func GetRemovePolicyFailureTestCalls(policy *NPMNetworkPolicy) []testutils.TestCmd {
	calls := GetRemovePolicyTestCalls(policy)
	calls[len(calls)-1] = fakeIPTablesRestoreFailureCommand // replace the restore success with a failure
	return calls
}

func GetBootupTestCalls() []testutils.TestCmd {
	return []testutils.TestCmd{
		{Cmd: []string{"iptables", "-w", "60", "-D", "FORWARD", "-j", "AZURE-NPM"}, ExitCode: 2}, //nolint // AZURE-NPM chain didn't exist
		{Cmd: listAllCommandStrings, PipedToCommand: true},
		{
			Cmd:      []string{"grep", "Chain AZURE-NPM"},
			ExitCode: 1,
		},
		fakeIPTablesRestoreCommand,
		{Cmd: listLineNumbersCommandStrings, PipedToCommand: true},
		{Cmd: []string{"grep", "AZURE-NPM"}, ExitCode: 1},
		{Cmd: []string{"iptables", "-w", "60", "-I", "FORWARD", "-j", "AZURE-NPM", "-m", "conntrack", "--ctstate", "NEW"}},
	}
}

func getFakeDeleteJumpCommand(chainName, jumpRule string) testutils.TestCmd {
	args := []string{"iptables", "-w", "60", "-D", chainName}
	args = append(args, strings.Split(jumpRule, " ")...)
	return testutils.TestCmd{Cmd: args}
}

func getFakeDeleteJumpCommandWithCode(chainName, jumpRule string, exitCode int) testutils.TestCmd {
	command := getFakeDeleteJumpCommand(chainName, jumpRule)
	command.ExitCode = exitCode
	return command
}
