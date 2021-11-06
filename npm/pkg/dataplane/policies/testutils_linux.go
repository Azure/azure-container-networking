package policies

import (
	"strings"

	"github.com/Azure/azure-container-networking/npm/util"
	testutils "github.com/Azure/azure-container-networking/test/utils"
)

var (
	fakeIPTablesRestoreCommand        = testutils.TestCmd{Cmd: []string{"iptables-restore", "-T", "filter", "--noflush"}}
	fakeIPTablesRestoreFailureCommand = testutils.TestCmd{Cmd: []string{"iptables-restore", "-T", "filter", "--noflush"}, ExitCode: 1}

	listLineNumbersCommandStrings      = []string{"iptables", "-w", "60", "-t", "filter", "-n", "-L", "FORWARD", "--line-numbers"}
	listPolicyChainNamesCommandStrings = []string{"iptables", "-w", "60", "-t", "filter", "-n", "-L"}
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

func GetInitializeTestCalls() []testutils.TestCmd {
	return []testutils.TestCmd{
		fakeIPTablesRestoreCommand, // gives correct exit code
		{
			Cmd:      listLineNumbersCommandStrings,
			ExitCode: 1, // grep call gets this exit code (exit code 1 means grep found nothing)
		},
		// NOTE: after the StdOut pipe used for grep, MockIOShim gets confused and each command's ExitCode and Stdout are applied to the ensuing command
		{
			Cmd:      []string{"grep", "KUBE-SERVICES"},
			Stdout:   "iptables: No chain/target/match by that name.", // this Stdout and ExitCode are for the iptables check command below
			ExitCode: 1,
		},
		{Cmd: []string{"iptables", "-w", "60", "-C", "FORWARD", "-j", "AZURE-NPM", "-m", "conntrack", "--ctstate", "NEW"}},
		{Cmd: []string{"iptables", "-w", "60", "-I", "FORWARD", "1", "-j", "AZURE-NPM", "-m", "conntrack", "--ctstate", "NEW"}},
	}
}

func GetResetTestCalls() []testutils.TestCmd {
	return []testutils.TestCmd{
		{Cmd: []string{"iptables", "-w", "60", "-D", "FORWARD", "-j", "AZURE-NPM", "-m", "conntrack", "--ctstate", "NEW"}},
		{
			Cmd:    listPolicyChainNamesCommandStrings,
			Stdout: "Chain AZURE-NPM-INGRESS-123456\nChain AZURE-NPM-EGRESS-123456",
		},
		// NOTE: after the StdOut pipe used for grep, MockIOShim gets confused and each command's ExitCode and Stdout are applied to the ensuing command
		{Cmd: []string{"grep", ingressOrEgressPolicyChainPattern}}, // ExitCode 0 for the iptables restore command
		fakeIPTablesRestoreCommand,
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
