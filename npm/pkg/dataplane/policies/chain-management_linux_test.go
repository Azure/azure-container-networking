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

	grepOutputAzureChainsWithoutPolicies = `Chain AZURE-NPM (1 references)
Chain AZURE-NPM-ACCEPT (1 references)
Chain AZURE-NPM-EGRESS (1 references)
Chain AZURE-NPM-INGRESS (1 references)
Chain AZURE-NPM-INGRESS-ALLOW-MARK (1 references)
`

	grepOutputAzureChainsWithPolicies = `Chain AZURE-NPM (1 references)
Chain AZURE-NPM-ACCEPT (1 references)
Chain AZURE-NPM-EGRESS (1 references)
Chain AZURE-NPM-EGRESS-123456 (1 references)
Chain AZURE-NPM-INGRESS (1 references)
Chain AZURE-NPM-INGRESS-123456 (1 references)
Chain AZURE-NPM-INGRESS-ALLOW-MARK (1 references)
`
)

func TestStaleChainsAddAndRemove(t *testing.T) {
	ioshim := common.NewMockIOShim(nil)
	defer ioshim.VerifyCalls(t, nil)
	pMgr := NewPolicyManager(ioshim, IPSetAndNoRebootConfig)

	pMgr.staleChains.add(testChain1)
	assertStaleChainsContain(t, pMgr.staleChains, testChain1)

	pMgr.staleChains.remove(testChain1)
	assertStaleChainsContain(t, pMgr.staleChains)

	// don't add our core NPM chains when we try to
	coreAzureChains := []string{
		"AZURE-NPM",
		"AZURE-NPM-INGRESS",
		"AZURE-NPM-INGRESS-ALLOW-MARK",
		"AZURE-NPM-EGRESS",
		"AZURE-NPM-ACCEPT",
	}
	for _, chain := range coreAzureChains {
		pMgr.staleChains.add(chain)
		assertStaleChainsContain(t, pMgr.staleChains)
	}
}

func TestStaleChainsEmptyAndGetAll(t *testing.T) {
	ioshim := common.NewMockIOShim(nil)
	defer ioshim.VerifyCalls(t, nil)
	pMgr := NewPolicyManager(ioshim, IPSetAndNoRebootConfig)
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
	defer ioshim.VerifyCalls(t, calls)
	pMgr := NewPolicyManager(ioshim, IPSetAndNoRebootConfig)

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
	defer ioshim.VerifyCalls(t, calls)
	pMgr := NewPolicyManager(ioshim, IPSetAndNoRebootConfig)

	pMgr.staleChains.add(testChain1)
	pMgr.staleChains.add(testChain2)
	pMgr.staleChains.add(testChain3)
	chainsToCleanup := pMgr.staleChains.emptyAndGetAll()
	sort.Strings(chainsToCleanup)
	require.Error(t, pMgr.cleanupChains(chainsToCleanup))
	assertStaleChainsContain(t, pMgr.staleChains, testChain1, testChain3)
}

func TestInitChainsCreator(t *testing.T) {
	ioshim := common.NewMockIOShim(nil)
	defer ioshim.VerifyCalls(t, nil)
	pMgr := NewPolicyManager(ioshim, IPSetAndNoRebootConfig)
	creator := pMgr.creatorForInitChains() // doesn't make any exec calls
	actualLines := strings.Split(creator.ToString(), "\n")
	expectedLines := []string{"*filter"}
	expectedLines = append(expectedLines, []string{
		":AZURE-NPM - -",
		":AZURE-NPM-INGRESS - -",
		":AZURE-NPM-INGRESS-ALLOW-MARK - -",
		":AZURE-NPM-EGRESS - -",
		":AZURE-NPM-ACCEPT - -",
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

func TestInitChains(t *testing.T) {
	tests := []struct {
		name    string
		calls   []testutils.TestCmd
		wantErr bool
	}{
		{
			name:    "success",
			calls:   GetInitializeTestCalls(),
			wantErr: false,
		},
		{
			name:    "failure on restore",
			calls:   []testutils.TestCmd{fakeIPTablesRestoreFailureCommand},
			wantErr: true,
		},
		{
			name: "failure on position",
			calls: []testutils.TestCmd{
				fakeIPTablesRestoreCommand,
				{Cmd: listLineNumbersCommandStrings, PipedToCommand: true, HasStartError: true, ExitCode: 1},
				{Cmd: []string{"grep", "AZURE-NPM"}},
			},
			wantErr: true,
		},
	}
	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			ioshim := common.NewMockIOShim(tt.calls)
			defer ioshim.VerifyCalls(t, tt.calls)
			pMgr := NewPolicyManager(ioshim, IPSetAndNoRebootConfig)
			err := pMgr.initialize()
			if tt.wantErr {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
			}
		})
	}
}

func TestCreatorAndChainsForResetSuccess(t *testing.T) {
	creatorCalls := []testutils.TestCmd{
		{Cmd: listPolicyChainNamesCommandStrings, PipedToCommand: true},
		{
			Cmd:    []string{"grep", "Chain AZURE-NPM"},
			Stdout: grepOutputAzureChainsWithPolicies,
		},
	}
	ioshim := common.NewMockIOShim(creatorCalls)
	defer ioshim.VerifyCalls(t, creatorCalls)
	pMgr := NewPolicyManager(ioshim, IPSetAndNoRebootConfig)
	creator, chainsToFlush, err := pMgr.creatorAndChainsForReset()
	require.NoError(t, err)
	expectedChainsToFlush := []string{
		"AZURE-NPM",
		"AZURE-NPM-ACCEPT",
		"AZURE-NPM-EGRESS",
		"AZURE-NPM-EGRESS-123456",
		"AZURE-NPM-INGRESS",
		"AZURE-NPM-INGRESS-123456",
		"AZURE-NPM-INGRESS-ALLOW-MARK",
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

func TestCreatorAndChainsForResetFailure(t *testing.T) {
	creatorCalls := []testutils.TestCmd{
		{Cmd: listPolicyChainNamesCommandStrings, PipedToCommand: true, HasStartError: true, ExitCode: 1},
		{Cmd: []string{"grep", "Chain AZURE-NPM"}},
	}
	ioshim := common.NewMockIOShim(creatorCalls)
	defer ioshim.VerifyCalls(t, creatorCalls)
	pMgr := NewPolicyManager(ioshim, IPSetAndNoRebootConfig)
	_, _, err := pMgr.creatorAndChainsForReset()
	require.Error(t, err)
}

func TestResetLinux(t *testing.T) {
	tests := []struct {
		name    string
		calls   []testutils.TestCmd
		wantErr bool
	}{
		{
			name: "success when there are chains currently",
			calls: []testutils.TestCmd{
				{Cmd: []string{"iptables", "-w", "60", "-D", "FORWARD", "-j", "AZURE-NPM", "-m", "conntrack", "--ctstate", "NEW"}},
				{Cmd: listPolicyChainNamesCommandStrings, PipedToCommand: true},
				{
					Cmd:    []string{"grep", "Chain AZURE-NPM"},
					Stdout: grepOutputAzureChainsWithoutPolicies,
				},
				fakeIPTablesRestoreCommand,
				getFakeDestroyCommand("AZURE-NPM"),
				getFakeDestroyCommand("AZURE-NPM-ACCEPT"),
				getFakeDestroyCommand("AZURE-NPM-EGRESS"),
				getFakeDestroyCommand("AZURE-NPM-INGRESS"),
				getFakeDestroyCommand("AZURE-NPM-INGRESS-ALLOW-MARK"),
			},
			wantErr: false,
		},
		{
			name: "success when there are no chains currently",
			calls: []testutils.TestCmd{
				{Cmd: []string{"iptables", "-w", "60", "-D", "FORWARD", "-j", "AZURE-NPM", "-m", "conntrack", "--ctstate", "NEW"}},
				{Cmd: listPolicyChainNamesCommandStrings, PipedToCommand: true},
				{Cmd: []string{"grep", "Chain AZURE-NPM"}, ExitCode: 1},
			},
			wantErr: false,
		},
		{
			name: "no error on failure to delete",
			calls: []testutils.TestCmd{
				{
					Cmd:      []string{"iptables", "-w", "60", "-D", "FORWARD", "-j", "AZURE-NPM", "-m", "conntrack", "--ctstate", "NEW"},
					ExitCode: 1, // delete failure
				},
				{Cmd: listPolicyChainNamesCommandStrings, PipedToCommand: true},
				{Cmd: []string{"grep", "Chain AZURE-NPM"}, ExitCode: 1},
			},
			wantErr: false,
		},
		{
			name: "failure on restore",
			calls: []testutils.TestCmd{
				{Cmd: []string{"iptables", "-w", "60", "-D", "FORWARD", "-j", "AZURE-NPM", "-m", "conntrack", "--ctstate", "NEW"}},
				{Cmd: listPolicyChainNamesCommandStrings, PipedToCommand: true},
				{
					Cmd:    []string{"grep", "Chain AZURE-NPM"},
					Stdout: grepOutputAzureChainsWithoutPolicies,
				},
				fakeIPTablesRestoreFailureCommand,
			},
			wantErr: true,
		},
		{
			name: "failure on destroy",
			calls: []testutils.TestCmd{
				{Cmd: []string{"iptables", "-w", "60", "-D", "FORWARD", "-j", "AZURE-NPM", "-m", "conntrack", "--ctstate", "NEW"}},
				{Cmd: listPolicyChainNamesCommandStrings, PipedToCommand: true},
				{
					Cmd:    []string{"grep", "Chain AZURE-NPM"},
					Stdout: grepOutputAzureChainsWithoutPolicies,
				},
				fakeIPTablesRestoreCommand,
				getFakeDestroyCommandWithExitCode("AZURE-NPM", 2),
				getFakeDestroyCommandWithExitCode("AZURE-NPM-ACCEPT", 2),
				getFakeDestroyCommand("AZURE-NPM-EGRESS"),
				getFakeDestroyCommand("AZURE-NPM-INGRESS"),
				getFakeDestroyCommand("AZURE-NPM-INGRESS-ALLOW-MARK"),
			},
			wantErr: true,
		},
	}
	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			ioshim := common.NewMockIOShim(tt.calls)
			defer ioshim.VerifyCalls(t, tt.calls)
			pMgr := NewPolicyManager(ioshim, IPSetAndNoRebootConfig)
			err := pMgr.reset()
			if tt.wantErr {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
			}
			assertStaleChainsContain(t, pMgr.staleChains)
		})
	}
}

func TestCreatorAndChainsForConfigureDeactivatedSuccess(t *testing.T) {
	// TODO
}

func TestCreatorAndChainsForConfigureDeactivatedFailure(t *testing.T) {
	// TODO
}

func TestConfigureDeactivated(t *testing.T) {
	// TODO
}

func TestPositionAzureChainJumpRule(t *testing.T) {
	tests := []struct {
		name    string
		calls   []testutils.TestCmd
		wantErr bool
	}{
		{
			name: "no jump rule yet",
			calls: []testutils.TestCmd{
				{Cmd: listLineNumbersCommandStrings, PipedToCommand: true},
				{Cmd: []string{"grep", "AZURE-NPM"}, ExitCode: 1},
				{Cmd: []string{"iptables", "-w", "60", "-I", "FORWARD", "-j", "AZURE-NPM", "-m", "conntrack", "--ctstate", "NEW"}},
			},
			wantErr: false,
		},
		{
			name: "no jump rule yet and insert fails",
			calls: []testutils.TestCmd{
				{Cmd: listLineNumbersCommandStrings, PipedToCommand: true},
				{Cmd: []string{"grep", "AZURE-NPM"}, ExitCode: 1},
				{Cmd: []string{"iptables", "-w", "60", "-I", "FORWARD", "-j", "AZURE-NPM", "-m", "conntrack", "--ctstate", "NEW"}, ExitCode: 1},
			},
			wantErr: true,
		},
		{
			name: "command error while grepping",
			calls: []testutils.TestCmd{
				{Cmd: listLineNumbersCommandStrings, PipedToCommand: true, HasStartError: true, ExitCode: 1},
				{Cmd: []string{"grep", "AZURE-NPM"}},
			},
			wantErr: true,
		},
		{
			name: "jump rule already at top",
			calls: []testutils.TestCmd{
				{Cmd: listLineNumbersCommandStrings, PipedToCommand: true},
				{
					Cmd:    []string{"grep", "AZURE-NPM"},
					Stdout: "1    AZURE-NPM  all  --  0.0.0.0/0            0.0.0.0/0    ...",
				},
			},
			wantErr: false,
		},
		{
			name: "jump rule not at top",
			calls: []testutils.TestCmd{
				{Cmd: listLineNumbersCommandStrings, PipedToCommand: true},
				{
					Cmd:    []string{"grep", "AZURE-NPM"},
					Stdout: "2    AZURE-NPM  all  --  0.0.0.0/0            0.0.0.0/0    ...",
				},
				{Cmd: []string{"iptables", "-w", "60", "-D", "FORWARD", "-j", "AZURE-NPM", "-m", "conntrack", "--ctstate", "NEW"}},
				{Cmd: []string{"iptables", "-w", "60", "-I", "FORWARD", "-j", "AZURE-NPM", "-m", "conntrack", "--ctstate", "NEW"}},
			},
			wantErr: false,
		},
		{
			name: "jump rule not at top and delete fails",
			calls: []testutils.TestCmd{
				{Cmd: listLineNumbersCommandStrings, PipedToCommand: true},
				{
					Cmd:    []string{"grep", "AZURE-NPM"},
					Stdout: "2    AZURE-NPM  all  --  0.0.0.0/0            0.0.0.0/0    ...",
				},
				{Cmd: []string{"iptables", "-w", "60", "-D", "FORWARD", "-j", "AZURE-NPM", "-m", "conntrack", "--ctstate", "NEW"}, ExitCode: 1},
			},
			wantErr: true,
		},
		{
			name: "jump rule not at top and insert fails",
			calls: []testutils.TestCmd{
				{Cmd: listLineNumbersCommandStrings, PipedToCommand: true},
				{
					Cmd:    []string{"grep", "AZURE-NPM"},
					Stdout: "2    AZURE-NPM  all  --  0.0.0.0/0            0.0.0.0/0    ...",
				},
				{Cmd: []string{"iptables", "-w", "60", "-D", "FORWARD", "-j", "AZURE-NPM", "-m", "conntrack", "--ctstate", "NEW"}},
				{Cmd: []string{"iptables", "-w", "60", "-I", "FORWARD", "-j", "AZURE-NPM", "-m", "conntrack", "--ctstate", "NEW"}, ExitCode: 1},
			},
			wantErr: true,
		},
	}
	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			ioshim := common.NewMockIOShim(tt.calls)
			defer ioshim.VerifyCalls(t, tt.calls)
			pMgr := NewPolicyManager(ioshim, IPSetAndNoRebootConfig)
			err := pMgr.positionAzureChainJumpRule()
			if tt.wantErr {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
			}
		})
	}
}

func TestChainLineNumber(t *testing.T) {
	testChainName := "TEST-CHAIN-NAME"
	tests := []struct {
		name            string
		calls           []testutils.TestCmd
		expectedLineNum int
		wantErr         bool
	}{
		{
			name: "chain exists",
			calls: []testutils.TestCmd{
				{Cmd: listLineNumbersCommandStrings, PipedToCommand: true},
				{
					Cmd:    []string{"grep", testChainName},
					Stdout: fmt.Sprintf("3    %s  all  --  0.0.0.0/0            0.0.0.0/0 ", testChainName),
				},
			},
			expectedLineNum: 3,
			wantErr:         false,
		},
		// TODO test for chain line number with 2+ digits
		{
			name: "ignore unexpected grep output",
			calls: []testutils.TestCmd{
				{Cmd: listLineNumbersCommandStrings, PipedToCommand: true},
				{
					Cmd:    []string{"grep", testChainName},
					Stdout: "3",
				},
			},
			expectedLineNum: 0,
			wantErr:         false,
		},
		{
			name: "chain doesn't exist",
			calls: []testutils.TestCmd{
				{Cmd: listLineNumbersCommandStrings, PipedToCommand: true},
				{Cmd: []string{"grep", testChainName}, ExitCode: 1},
			},
			expectedLineNum: 0,
			wantErr:         false,
		},
		{
			name: "command error while grepping",
			calls: []testutils.TestCmd{
				{Cmd: listLineNumbersCommandStrings, PipedToCommand: true, HasStartError: true, ExitCode: 1},
				{Cmd: []string{"grep", testChainName}},
			},
			expectedLineNum: 0,
			wantErr:         true,
		},
	}
	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			ioshim := common.NewMockIOShim(tt.calls)
			defer ioshim.VerifyCalls(t, tt.calls)
			pMgr := NewPolicyManager(ioshim, IPSetAndNoRebootConfig)
			lineNum, err := pMgr.chainLineNumber(testChainName)
			if tt.wantErr {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
			}
			require.Equal(t, tt.expectedLineNum, lineNum)
		})
	}
}

func TestAllCurrentAzureChains(t *testing.T) {
	tests := []struct {
		name           string
		calls          []testutils.TestCmd
		expectedChains []string
		wantErr        bool
	}{
		{
			name: "success with chains",
			calls: []testutils.TestCmd{
				{Cmd: listPolicyChainNamesCommandStrings, PipedToCommand: true},
				{
					Cmd:    []string{"grep", "Chain AZURE-NPM"},
					Stdout: grepOutputAzureChainsWithPolicies,
				},
			},
			expectedChains: []string{"AZURE-NPM", "AZURE-NPM-ACCEPT", "AZURE-NPM-EGRESS", "AZURE-NPM-EGRESS-123456", "AZURE-NPM-INGRESS", "AZURE-NPM-INGRESS-123456", "AZURE-NPM-INGRESS-ALLOW-MARK"},
			wantErr:        false,
		},
		{
			name: "ignore missing newline at end of grep result",
			calls: []testutils.TestCmd{
				{Cmd: []string{"iptables", "-w", "60", "-t", "filter", "-n", "-L"}, PipedToCommand: true},
				{
					Cmd: []string{"grep", "Chain AZURE-NPM"},
					Stdout: `Chain AZURE-NPM (1 references)
Chain AZURE-NPM-INGRESS (1 references)`,
				},
			},
			expectedChains: []string{"AZURE-NPM", "AZURE-NPM-INGRESS"},
			wantErr:        false,
		},
		{
			name: "ignore unexpected grep line (chain name too short)",
			calls: []testutils.TestCmd{
				{Cmd: []string{"iptables", "-w", "60", "-t", "filter", "-n", "-L"}, PipedToCommand: true},
				{
					Cmd: []string{"grep", "Chain AZURE-NPM"},
					Stdout: `Chain AZURE-NPM (1 references)
Chain abc (1 references)
Chain AZURE-NPM-INGRESS (1 references)
`,
				},
			},
			expectedChains: []string{"AZURE-NPM", "AZURE-NPM-INGRESS"},
			wantErr:        false,
		},
		{
			name: "ignore unexpected grep line (no space)",
			calls: []testutils.TestCmd{
				{Cmd: []string{"iptables", "-w", "60", "-t", "filter", "-n", "-L"}, PipedToCommand: true},
				{
					Cmd: []string{"grep", "Chain AZURE-NPM"},
					Stdout: `Chain AZURE-NPM (1 references)
abc
Chain AZURE-NPM-INGRESS (1 references)
`,
				},
			},
			expectedChains: []string{"AZURE-NPM", "AZURE-NPM-INGRESS"},
		},
		{
			name: "success with no chains",
			calls: []testutils.TestCmd{
				{Cmd: []string{"iptables", "-w", "60", "-t", "filter", "-n", "-L"}, PipedToCommand: true},
				{Cmd: []string{"grep", "Chain AZURE-NPM"}, ExitCode: 1},
			},
			expectedChains: nil,
			wantErr:        false,
		},
		{
			name: "grep failure",
			calls: []testutils.TestCmd{
				{Cmd: []string{"iptables", "-w", "60", "-t", "filter", "-n", "-L"}, PipedToCommand: true, HasStartError: true, ExitCode: 1},
				{Cmd: []string{"grep", "Chain AZURE-NPM"}},
			},
			expectedChains: nil,
			wantErr:        true,
		},
		{
			name: "invalid grep result",
			calls: []testutils.TestCmd{
				{Cmd: []string{"iptables", "-w", "60", "-t", "filter", "-n", "-L"}, PipedToCommand: true},
				{
					Cmd:    []string{"grep", "Chain AZURE-NPM"},
					Stdout: "",
				},
			},
			expectedChains: nil,
			wantErr:        true,
		},
	}
	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			ioshim := common.NewMockIOShim(tt.calls)
			defer ioshim.VerifyCalls(t, tt.calls)
			pMgr := NewPolicyManager(ioshim, IPSetAndNoRebootConfig)
			chains, err := pMgr.allCurrentAzureChains()
			if tt.wantErr {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
			}
			require.Equal(t, tt.expectedChains, chains)
		})
	}
}

func getFakeDestroyCommand(chain string) testutils.TestCmd {
	return testutils.TestCmd{Cmd: []string{"iptables", "-w", "60", "-X", chain}}
}

func getFakeDestroyCommandWithExitCode(chain string, exitCode int) testutils.TestCmd {
	command := getFakeDestroyCommand(chain)
	command.ExitCode = exitCode
	return command
}
