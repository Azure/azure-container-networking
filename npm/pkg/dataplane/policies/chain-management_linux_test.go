package policies

import (
	"fmt"
	"sort"
	"strings"
	"testing"

	"github.com/Azure/azure-container-networking/common"
	dptestutils "github.com/Azure/azure-container-networking/npm/pkg/dataplane/testutils"
	testutils "github.com/Azure/azure-container-networking/test/utils"
	"github.com/stretchr/testify/require"
)

const (
	testChain1 = "chain1"
	testChain2 = "chain2"
	testChain3 = "chain3"
)

func TestEmptyAndGetAll(t *testing.T) {
	pMgr := NewPolicyManager(common.NewMockIOShim(nil))
	pMgr.staleChains.add(testChain1)
	pMgr.staleChains.add(testChain2)
	chainsToCleanup := pMgr.staleChains.emptyAndGetAll()
	require.Equal(t, 2, len(chainsToCleanup))
	require.True(t, chainsToCleanup[0] == testChain1 || chainsToCleanup[1] == testChain1)
	require.True(t, chainsToCleanup[0] == testChain2 || chainsToCleanup[1] == testChain2)
	assertStaleChainsContain(t, pMgr.staleChains)
}

func assertStaleChainsContain(t *testing.T, s *staleChains, expectedChains ...string) {
	require.Equal(t, len(expectedChains), len(s.chainsToCleanup), "incorrectly tracking chains for cleanup")
	for _, chain := range expectedChains {
		_, exists := s.chainsToCleanup[chain]
		require.True(t, exists, "incorrectly tracking chains for cleanup")
	}
}

func TestCleanupChainsSuccess(t *testing.T) {
	calls := []testutils.TestCmd{
		getFakeDestroyCommand(testChain1),
		getFakeDestroyCommandWithExitCode(testChain2, 1), // exit code 1 means the chain d.n.e.
	}
	ioshim := common.NewMockIOShim(calls)
	// TODO defer ioshim.VerifyCalls(t, ioshim, calls)
	pMgr := NewPolicyManager(ioshim)

	pMgr.staleChains.add(testChain1)
	pMgr.staleChains.add(testChain2)
	chainsToCleanup := pMgr.staleChains.emptyAndGetAll()
	sort.Strings(chainsToCleanup)
	require.NoError(t, pMgr.cleanupChains(chainsToCleanup))
	assertStaleChainsContain(t, pMgr.staleChains)
}

func TestCleanupChainsFailure(t *testing.T) {
	calls := []testutils.TestCmd{
		getFakeDestroyCommandWithExitCode(testChain1, 2),
		getFakeDestroyCommand(testChain2),
		getFakeDestroyCommandWithExitCode(testChain3, 2),
	}
	ioshim := common.NewMockIOShim(calls)
	// TODO defer ioshim.VerifyCalls(t, ioshim, calls)
	pMgr := NewPolicyManager(ioshim)

	pMgr.staleChains.add(testChain1)
	pMgr.staleChains.add(testChain2)
	pMgr.staleChains.add(testChain3)
	chainsToCleanup := pMgr.staleChains.emptyAndGetAll()
	sort.Strings(chainsToCleanup)
	require.Error(t, pMgr.cleanupChains(chainsToCleanup))
	assertStaleChainsContain(t, pMgr.staleChains, testChain1, testChain3)
}

func TestInitChainsCreator(t *testing.T) {
	pMgr := NewPolicyManager(common.NewMockIOShim(nil))
	creator := pMgr.creatorForInitChains() // doesn't make any exec calls
	actualLines := strings.Split(creator.ToString(), "\n")
	expectedLines := []string{"*filter"}
	for _, chain := range iptablesAzureChains {
		expectedLines = append(expectedLines, fmt.Sprintf(":%s - -", chain))
	}
	expectedLines = append(expectedLines, []string{
		"-A AZURE-NPM -j AZURE-NPM-INGRESS",
		"-A AZURE-NPM -j AZURE-NPM-EGRESS",
		"-A AZURE-NPM -j AZURE-NPM-ACCEPT",
		"-A AZURE-NPM-INGRESS -j DROP -m mark --mark 0x4000 -m comment --comment DROP-ON-INGRESS-DROP-MARK-0x4000",
		"-A AZURE-NPM-INGRESS-ALLOW-MARK -j MARK --set-mark 0x2000 -m comment --comment SET-INGRESS-ALLOW-MARK-0x2000",
		"-A AZURE-NPM-INGRESS-ALLOW-MARK -j AZURE-NPM-EGRESS",
		"-A AZURE-NPM-EGRESS -j DROP -m mark --mark 0x5000 -m comment --comment DROP-ON-EGRESS-DROP-MARK-0x5000",
		"-A AZURE-NPM-EGRESS -j AZURE-NPM-ACCEPT -m mark --mark 0x2000 -m comment --comment ACCEPT-ON-INGRESS-ALLOW-MARK-0x2000",
		"-A AZURE-NPM-ACCEPT -j MARK --set-mark 0x0 -m comment --comment Clear-AZURE-NPM-MARKS",
		"-A AZURE-NPM-ACCEPT -j ACCEPT",
		"COMMIT\n",
	}...)
	dptestutils.AssertEqualLines(t, expectedLines, actualLines)
}

func TestInitChainsSuccess(t *testing.T) {
	calls := GetInitializeTestCalls()
	pMgr := NewPolicyManager(common.NewMockIOShim(calls))
	require.NoError(t, pMgr.initializeNPMChains())
}

func TestInitChainsFailureOnRestore(t *testing.T) {
	calls := []testutils.TestCmd{fakeIPTablesRestoreFailureCommand}
	pMgr := NewPolicyManager(common.NewMockIOShim(calls))
	require.Error(t, pMgr.initializeNPMChains())
}

func TestInitChainsFailureOnPosition(t *testing.T) {
	calls := []testutils.TestCmd{
		fakeIPTablesRestoreCommand, // gives correct exit code
		{
			Cmd:      listLineNumbersCommandStrings,
			ExitCode: 1, // grep call gets this exit code (exit code 1 means grep found nothing)
		},
		// NOTE: after the StdOut pipe used for grep, MockIOShim gets confused and each command's ExitCode and Stdout are applied to the ensuing command
		{
			Cmd:      []string{"grep", "KUBE-SERVICES"},
			Stdout:   "iptables: No chain/target/match by that name.", // this Stdout and ExitCode are for the iptables check command below
			ExitCode: 2,                                               // Check failed for unknown reason
		},
		{Cmd: []string{"iptables", "-w", "60", "-C", "FORWARD", "-j", "AZURE-NPM", "-m", "conntrack", "--ctstate", "NEW"}},
	}
	pMgr := NewPolicyManager(common.NewMockIOShim(calls))
	require.Error(t, pMgr.initializeNPMChains())
}

func TestRemoveChainsCreator(t *testing.T) {
	creatorCalls := []testutils.TestCmd{
		{
			Cmd:    listPolicyChainNamesCommandStrings,
			Stdout: "Chain AZURE-NPM-INGRESS-123456\nChain AZURE-NPM-EGRESS-123456",
		},
		// NOTE: after the StdOut pipe used for grep, MockIOShim gets confused and each command's ExitCode and Stdout are applied to the ensuing command
		{Cmd: []string{"grep", ingressOrEgressPolicyChainPattern}},
	}

	pMgr := NewPolicyManager(common.NewMockIOShim(creatorCalls))
	creator, chainsToFlush := pMgr.creatorAndChainsForReset()
	expectedChainsToFlush := []string{
		"AZURE-NPM",
		"AZURE-NPM-INGRESS",
		"AZURE-NPM-INGRESS-ALLOW-MARK",
		"AZURE-NPM-EGRESS",
		"AZURE-NPM-ACCEPT",
		// deprecated
		"AZURE-NPM-INGRESS-FROM",
		"AZURE-NPM-INGRESS-PORT",
		"AZURE-NPM-INGRESS-DROPS",
		"AZURE-NPM-EGRESS-TO",
		"AZURE-NPM-EGRESS-PORT",
		"AZURE-NPM-EGRESS-DROPS",
		"AZURE-NPM-TARGET-SETS",
		"AZURE-NPM-INRGESS-DROPS",
		// policy chains
		"AZURE-NPM-INGRESS-123456",
		"AZURE-NPM-EGRESS-123456",
	}
	require.Equal(t, expectedChainsToFlush, chainsToFlush)
	actualLines := strings.Split(creator.ToString(), "\n")
	expectedLines := []string{"*filter"}
	for _, chain := range expectedChainsToFlush {
		expectedLines = append(expectedLines, fmt.Sprintf(":%s - -", chain))
	}
	expectedLines = append(expectedLines, "COMMIT\n")
	dptestutils.AssertEqualLines(t, expectedLines, actualLines)
}

func TestRemoveChainsSuccess(t *testing.T) {
	calls := GetResetTestCalls()
	for _, chain := range iptablesOldAndNewChains { // TODO write these out, don't use variable
		calls = append(calls, getFakeDestroyCommand(chain))
	}
	calls = append(
		calls,
		getFakeDestroyCommand("AZURE-NPM-INGRESS-123456"),
		getFakeDestroyCommand("AZURE-NPM-EGRESS-123456"),
	)

	pMgr := NewPolicyManager(common.NewMockIOShim(calls))
	require.NoError(t, pMgr.removeNPMChains())
}

func TestRemoveChainsFailureOnDelete(t *testing.T) {
	calls := []testutils.TestCmd{
		{
			Cmd:      []string{"iptables", "-w", "60", "-D", "FORWARD", "-j", "AZURE-NPM", "-m", "conntrack", "--ctstate", "NEW"},
			ExitCode: 1, // delete failure
		},
	}
	pMgr := NewPolicyManager(common.NewMockIOShim(calls))
	require.Error(t, pMgr.removeNPMChains())
}

func TestRemoveChainsFailureOnRestore(t *testing.T) {
	calls := []testutils.TestCmd{
		{Cmd: []string{"iptables", "-w", "60", "-D", "FORWARD", "-j", "AZURE-NPM", "-m", "conntrack", "--ctstate", "NEW"}},
		{
			Cmd:    listPolicyChainNamesCommandStrings,
			Stdout: "Chain AZURE-NPM-INGRESS-123456\nChain AZURE-NPM-EGRESS-123456",
		},
		// NOTE: after the StdOut pipe used for grep, MockIOShim gets confused and each command's ExitCode and Stdout are applied to the ensuing command
		{
			Cmd:      []string{"grep", ingressOrEgressPolicyChainPattern},
			ExitCode: 1, // ExitCode 1 for the iptables restore command
		},
		fakeIPTablesRestoreFailureCommand, // the exit code doesn't matter for this command since it receives the exit code of the command above
	}
	pMgr := NewPolicyManager(common.NewMockIOShim(calls))
	require.Error(t, pMgr.removeNPMChains())
}

func TestRemoveChainsFailureOnDestroy(t *testing.T) {
	calls := []testutils.TestCmd{
		{Cmd: []string{"iptables", "-w", "60", "-D", "FORWARD", "-j", "AZURE-NPM", "-m", "conntrack", "--ctstate", "NEW"}},
		{
			Cmd:    listPolicyChainNamesCommandStrings,
			Stdout: "Chain AZURE-NPM-INGRESS-123456\nChain AZURE-NPM-EGRESS-123456",
		},
		// NOTE: after the StdOut pipe used for grep, MockIOShim gets confused and each command's ExitCode and Stdout are applied to the ensuing command
		{Cmd: []string{"grep", ingressOrEgressPolicyChainPattern}}, // ExitCode 0 for the iptables restore command
		fakeIPTablesRestoreCommand,
	}
	calls = append(calls, getFakeDestroyCommandWithExitCode(iptablesOldAndNewChains[0], 2)) // this ExitCode here will actually impact the next below
	for _, chain := range iptablesOldAndNewChains[1:] {
		calls = append(calls, getFakeDestroyCommand(chain))
	}
	calls = append(
		calls,
		getFakeDestroyCommand("AZURE-NPM-INGRESS-123456"),
		getFakeDestroyCommand("AZURE-NPM-EGRESS-123456"),
	)
	pMgr := NewPolicyManager(common.NewMockIOShim(calls))
	require.Error(t, pMgr.removeNPMChains())
}

func TestPositionJumpWhenNoChainsExist(t *testing.T) {
	calls := []testutils.TestCmd{
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
	pMgr := NewPolicyManager(common.NewMockIOShim(calls))
	require.NoError(t, pMgr.positionAzureChainJumpRule())
}

func TestPositionJumpWhenOnlyAzureExists(t *testing.T) {
	calls := []testutils.TestCmd{
		{
			Cmd:      listLineNumbersCommandStrings,
			ExitCode: 1, // grep call gets this exit code (exit code 1 means grep found nothing)
		},
		// NOTE: after the StdOut pipe used for grep, MockIOShim gets confused and each command's ExitCode and Stdout are applied to the ensuing command
		{Cmd: []string{"grep", "KUBE-SERVICES"}}, // ExitCode 0 for the iptables check command below
		{Cmd: []string{"iptables", "-w", "60", "-C", "FORWARD", "-j", "AZURE-NPM", "-m", "conntrack", "--ctstate", "NEW"}},
	}
	pMgr := NewPolicyManager(common.NewMockIOShim(calls))
	require.NoError(t, pMgr.positionAzureChainJumpRule())
}

func TestPositionJumpWhenOnlyKubeServicesExists(t *testing.T) {
	calls := []testutils.TestCmd{
		{
			Cmd:    listLineNumbersCommandStrings,
			Stdout: "3    KUBE-SERVICES  all  --  0.0.0.0/0            0.0.0.0/0 ", // grep call gets this Stdout
		},
		// NOTE: after the StdOut pipe used for grep, MockIOShim gets confused and each command's ExitCode and Stdout are applied to the ensuing command
		{
			Cmd:      []string{"grep", "KUBE-SERVICES"},
			Stdout:   "iptables: No chain/target/match by that name.", // this Stdout and ExitCode are for the iptables check command below
			ExitCode: 1,
		},
		{Cmd: []string{"iptables", "-w", "60", "-C", "FORWARD", "-j", "AZURE-NPM", "-m", "conntrack", "--ctstate", "NEW"}},
		{Cmd: []string{"iptables", "-w", "60", "-I", "FORWARD", "4", "-j", "AZURE-NPM", "-m", "conntrack", "--ctstate", "NEW"}},
	}
	pMgr := NewPolicyManager(common.NewMockIOShim(calls))
	require.NoError(t, pMgr.positionAzureChainJumpRule())
}

func TestPositionJumpWhenOnlyKubeServicesExistsAndInsertFails(t *testing.T) {
	calls := []testutils.TestCmd{
		{
			Cmd:    listLineNumbersCommandStrings,
			Stdout: "3    KUBE-SERVICES  all  --  0.0.0.0/0            0.0.0.0/0 ", // grep call gets this Stdout
		},
		// NOTE: after the StdOut pipe used for grep, MockIOShim gets confused and each command's ExitCode and Stdout are applied to the ensuing command
		{
			Cmd:      []string{"grep", "KUBE-SERVICES"},
			Stdout:   "iptables: No chain/target/match by that name.", // this Stdout and ExitCode are for the iptables check command below
			ExitCode: 1,
		},
		{
			Cmd:      []string{"iptables", "-w", "60", "-C", "FORWARD", "-j", "AZURE-NPM", "-m", "conntrack", "--ctstate", "NEW"},
			ExitCode: 1, // ExitCode 1 for insert below
		},
		{Cmd: []string{"iptables", "-w", "60", "-I", "FORWARD", "4", "-j", "AZURE-NPM", "-m", "conntrack", "--ctstate", "NEW"}},
	}
	pMgr := NewPolicyManager(common.NewMockIOShim(calls))
	require.Error(t, pMgr.positionAzureChainJumpRule())
}

func TestPositionJumpWhenAzureAfterKubeServices(t *testing.T) {
	// don't move the rule for AZURE-NPM
	calls := []testutils.TestCmd{
		{
			Cmd:    listLineNumbersCommandStrings,
			Stdout: "3    KUBE-SERVICES  all  --  0.0.0.0/0            0.0.0.0/0 ", // grep call gets this Stdout
		},
		// NOTE: after the StdOut pipe used for grep, MockIOShim gets confused and each command's ExitCode and Stdout are applied to the ensuing command
		{Cmd: []string{"grep", "KUBE-SERVICES"}}, // ExitCode 0 for the iptables check command below
		{
			Cmd:    []string{"iptables", "-w", "60", "-C", "FORWARD", "-j", "AZURE-NPM", "-m", "conntrack", "--ctstate", "NEW"},
			Stdout: "4    AZURE-NPM  all  --  0.0.0.0/0            0.0.0.0/0 ", // grep call below gets this Stdout
		},
		{Cmd: listLineNumbersCommandStrings},
		{Cmd: []string{"grep", "AZURE-NPM"}},
	}
	pMgr := NewPolicyManager(common.NewMockIOShim(calls))
	require.NoError(t, pMgr.positionAzureChainJumpRule())
}

func TestPositionJumpWhenAzureBeforeKubeServices(t *testing.T) {
	// move the rule for AZURE-NPM
	calls := []testutils.TestCmd{
		{
			Cmd:    listLineNumbersCommandStrings,
			Stdout: "3    KUBE-SERVICES  all  --  0.0.0.0/0            0.0.0.0/0 ", // grep call gets this Stdout
		},
		// NOTE: after the StdOut pipe used for grep, MockIOShim gets confused and each command's ExitCode and Stdout are applied to the ensuing command
		{Cmd: []string{"grep", "KUBE-SERVICES"}}, // ExitCode 0 for the iptables check command below
		{
			Cmd:    []string{"iptables", "-w", "60", "-C", "FORWARD", "-j", "AZURE-NPM", "-m", "conntrack", "--ctstate", "NEW"},
			Stdout: "2    AZURE-NPM  all  --  0.0.0.0/0            0.0.0.0/0 ", // grep call below gets this Stdout
		},
		{Cmd: listLineNumbersCommandStrings},
		{Cmd: []string{"grep", "AZURE-NPM"}},
		{Cmd: []string{"iptables", "-w", "60", "-D", "FORWARD", "-j", "AZURE-NPM", "-m", "conntrack", "--ctstate", "NEW"}},
		{Cmd: []string{"iptables", "-w", "60", "-I", "FORWARD", "3", "-j", "AZURE-NPM", "-m", "conntrack", "--ctstate", "NEW"}},
	}
	pMgr := NewPolicyManager(common.NewMockIOShim(calls))
	require.NoError(t, pMgr.positionAzureChainJumpRule())
}

func TestPositionJumpWhenAzureBeforeKubeServicesAndDeleteFails(t *testing.T) {
	calls := []testutils.TestCmd{
		{
			Cmd:    listLineNumbersCommandStrings,
			Stdout: "3    KUBE-SERVICES  all  --  0.0.0.0/0            0.0.0.0/0 ", // grep call gets this Stdout
		},
		// NOTE: after the StdOut pipe used for grep, MockIOShim gets confused and each command's ExitCode and Stdout are applied to the ensuing command
		{Cmd: []string{"grep", "KUBE-SERVICES"}}, // ExitCode 0 for the iptables check command below
		{
			Cmd:    []string{"iptables", "-w", "60", "-C", "FORWARD", "-j", "AZURE-NPM", "-m", "conntrack", "--ctstate", "NEW"},
			Stdout: "2    AZURE-NPM  all  --  0.0.0.0/0            0.0.0.0/0 ", // grep call below gets this Stdout
		},
		{
			Cmd:      listLineNumbersCommandStrings,
			ExitCode: 1,
			// NOTE: now MockIOShim is off by 2 for ExitCodes and Stdout
			// ExitCode 1 for delete command below
		},
		{Cmd: []string{"grep", "AZURE-NPM"}},
		{Cmd: []string{"iptables", "-w", "60", "-D", "FORWARD", "-j", "AZURE-NPM", "-m", "conntrack", "--ctstate", "NEW"}},
	}
	pMgr := NewPolicyManager(common.NewMockIOShim(calls))
	require.Error(t, pMgr.positionAzureChainJumpRule())
}

func TestPositionJumpWhenAzureBeforeKubeServicesAndInsertFails(t *testing.T) {
	calls := []testutils.TestCmd{
		{
			Cmd:    listLineNumbersCommandStrings,
			Stdout: "3    KUBE-SERVICES  all  --  0.0.0.0/0            0.0.0.0/0 ", // grep call gets this Stdout
		},
		// NOTE: after the StdOut pipe used for grep, MockIOShim gets confused and each command's ExitCode and Stdout are applied to the ensuing command
		{Cmd: []string{"grep", "KUBE-SERVICES"}}, // ExitCode 0 for the iptables check command below
		{
			Cmd:    []string{"iptables", "-w", "60", "-C", "FORWARD", "-j", "AZURE-NPM", "-m", "conntrack", "--ctstate", "NEW"},
			Stdout: "2    AZURE-NPM  all  --  0.0.0.0/0            0.0.0.0/0 ", // grep call below gets this Stdout
		},
		{Cmd: listLineNumbersCommandStrings}, // NOTE: now MockIOShim is off by 2 for ExitCodes and Stdout
		// ExitCode 0 for delete command below
		{
			Cmd:      []string{"grep", "AZURE-NPM"},
			ExitCode: 1, // ExitCode 1 for insert command below
		},
		{Cmd: []string{"iptables", "-w", "60", "-D", "FORWARD", "-j", "AZURE-NPM", "-m", "conntrack", "--ctstate", "NEW"}},
		{Cmd: []string{"iptables", "-w", "60", "-I", "FORWARD", "3", "-j", "AZURE-NPM", "-m", "conntrack", "--ctstate", "NEW"}},
	}
	pMgr := NewPolicyManager(common.NewMockIOShim(calls))
	require.Error(t, pMgr.positionAzureChainJumpRule())
}

func TestGetChainLineNumber(t *testing.T) {
	testChainName := "TEST-CHAIN-NAME"
	grepCommand := testutils.TestCmd{Cmd: []string{"grep", testChainName}}

	// chain exists at line 3
	calls := []testutils.TestCmd{
		{
			Cmd:      listLineNumbersCommandStrings,
			Stdout:   fmt.Sprintf("3    %s  all  --  0.0.0.0/0            0.0.0.0/0 ", testChainName),
			ExitCode: 0,
		},
		grepCommand,
	}
	pMgr := NewPolicyManager(common.NewMockIOShim(calls))
	lineNum, err := pMgr.chainLineNumber(testChainName)
	require.Equal(t, 3, lineNum)
	require.NoError(t, err)

	// chain doesn't exist
	calls = []testutils.TestCmd{
		{
			Cmd:      listLineNumbersCommandStrings,
			ExitCode: 1, // grep found nothing
		},
		grepCommand,
	}
	pMgr = NewPolicyManager(common.NewMockIOShim(calls))
	lineNum, err = pMgr.chainLineNumber(testChainName)
	require.Equal(t, 0, lineNum)
	require.NoError(t, err)
}

func TestGetPolicyChainNames(t *testing.T) {
	// grep that finds results
	grepCommand := testutils.TestCmd{Cmd: []string{"grep", ingressOrEgressPolicyChainPattern}}
	calls := []testutils.TestCmd{
		{
			Cmd:    listPolicyChainNamesCommandStrings,
			Stdout: "Chain AZURE-NPM-INGRESS-123456\nChain AZURE-NPM-EGRESS-123456",
		},
		grepCommand,
	}
	pMgr := NewPolicyManager(common.NewMockIOShim(calls))
	chainNames, err := pMgr.policyChainNames()
	expectedChainNames := []string{
		"AZURE-NPM-INGRESS-123456",
		"AZURE-NPM-EGRESS-123456",
	}
	require.Equal(t, expectedChainNames, chainNames)
	require.NoError(t, err)

	// grep with no results
	calls = []testutils.TestCmd{
		{
			Cmd:      listPolicyChainNamesCommandStrings,
			ExitCode: 1, // grep found nothing
		},
		grepCommand,
	}
	pMgr = NewPolicyManager(common.NewMockIOShim(calls))
	chainNames, err = pMgr.policyChainNames()
	expectedChainNames = nil
	require.Equal(t, expectedChainNames, chainNames)
	require.NoError(t, err)
}

func getFakeDestroyCommand(chain string) testutils.TestCmd {
	return testutils.TestCmd{Cmd: []string{"iptables", "-w", "60", "-X", chain}}
}

func getFakeDestroyCommandWithExitCode(chain string, exitCode int) testutils.TestCmd {
	command := getFakeDestroyCommand(chain)
	command.ExitCode = exitCode
	return command
}
