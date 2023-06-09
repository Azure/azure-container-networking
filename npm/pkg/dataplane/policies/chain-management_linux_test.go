package policies

import (
	"fmt"
	"sort"
	"strings"
	"testing"

	"github.com/Azure/azure-container-networking/common"
	"github.com/Azure/azure-container-networking/npm/metrics"
	dptestutils "github.com/Azure/azure-container-networking/npm/pkg/dataplane/testutils"
	"github.com/Azure/azure-container-networking/npm/util"
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

	grepOutputAzureV1Chains = `Chain AZURE-NPM
Chain AZURE-NPM (1 references)
Chain AZURE-NPM-INGRESS (1 references)
Chain AZURE-NPM-INGRESS-DROPS (1 references)
Chain AZURE-NPM-INGRESS-TO (1 references)
Chain AZURE-NPM-INGRESS-PORTS (1 references)
Chain AZURE-NPM-EGRESS (1 references)
Chain AZURE-NPM-EGRESS-DROPS (1 references)
Chain AZURE-NPM-EGRESS-FROM (1 references)
Chain AZURE-NPM-EGRESS-PORTS (1 references)
Chain AZURE-NPM-ACCEPT (1 references)
`
)

// similar to TestBootup in policymanager.go except an error occurs
func TestBootupFailure(t *testing.T) {
	metrics.ReinitializeAll()
	calls := []testutils.TestCmd{
		{Cmd: []string{"iptables", "-w", "60", "-D", "FORWARD", "-j", "AZURE-NPM"}, ExitCode: 2}, //nolint // AZURE-NPM chain didn't exist
		{Cmd: listAllCommandStrings, PipedToCommand: true, HasStartError: true, ExitCode: 1},
		{Cmd: []string{"grep", "Chain AZURE-NPM"}},
	}
	ioshim := common.NewMockIOShim(calls)
	defer ioshim.VerifyCalls(t, calls)
	pMgr := NewPolicyManager(ioshim, ipsetConfig)

	metrics.IncNumACLRules()
	metrics.IncNumACLRules()

	require.Error(t, pMgr.Bootup(nil))

	// make sure that the metrics were reset
	promVals{0, 0}.testPrometheusMetrics(t)
}

func TestStaleChainsForceLock(t *testing.T) {
	testChains := []string{}
	for i := 0; i < 100000; i++ {
		testChains = append(testChains, fmt.Sprintf("test-chain-%d", i))
	}
	calls := []testutils.TestCmd{}
	for _, chain := range testChains {
		calls = append(calls, getFakeDestroyCommand(chain))
	}
	ioshim := common.NewMockIOShim(calls)
	// don't verify calls because there shouldn't be as many commands as we create if forceLock works properly
	pMgr := NewPolicyManager(ioshim, ipsetConfig)

	start := make(chan struct{}, 1)
	done := make(chan struct{}, 1)
	go func() {
		pMgr.reconcileManager.Lock()
		defer pMgr.reconcileManager.Unlock()
		start <- struct{}{}
		require.NoError(t, pMgr.cleanupChains(testChains))
		done <- struct{}{}
	}()
	<-start
	pMgr.reconcileManager.forceLock()
	<-done
	// the releaseLockSignal should be empty, there should be some stale chains, and staleChains should be unlockable
	fmt.Println("weren't able to delete this many chains:", len(pMgr.staleChains.chainsToCleanup))
	require.NotEqual(t, 0, len(pMgr.staleChains.chainsToCleanup), "stale chains should not be empty")
	require.Equal(t, 0, len(pMgr.reconcileManager.releaseLockSignal), "releaseLockSignal should be empty")
	pMgr.reconcileManager.Unlock()
}

func TestStaleChainsForceUnlock(t *testing.T) {
	ioshim := common.NewMockIOShim(nil)
	defer ioshim.VerifyCalls(t, nil)
	pMgr := NewPolicyManager(ioshim, ipsetConfig)
	pMgr.reconcileManager.forceLock()
	require.Equal(t, 1, len(pMgr.reconcileManager.releaseLockSignal), "releaseLockSignal should be non-empty")
	pMgr.reconcileManager.forceUnlock()
	// the releaseLockSignal should be empty and staleChains should be lockable
	require.Equal(t, 0, len(pMgr.reconcileManager.releaseLockSignal), "releaseLockSignal should be empty")
	pMgr.reconcileManager.Lock()
}

func TestStaleChainsAddAndRemove(t *testing.T) {
	ioshim := common.NewMockIOShim(nil)
	defer ioshim.VerifyCalls(t, nil)
	pMgr := NewPolicyManager(ioshim, ipsetConfig)

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
	pMgr := NewPolicyManager(ioshim, ipsetConfig)
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
		getFakeDestroyCommandWithExitCode(testChain2, 1), // exit code 1 means the chain does not exist
	}
	ioshim := common.NewMockIOShim(calls)
	defer ioshim.VerifyCalls(t, calls)
	pMgr := NewPolicyManager(ioshim, ipsetConfig)

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
	pMgr := NewPolicyManager(ioshim, ipsetConfig)

	pMgr.staleChains.add(testChain1)
	pMgr.staleChains.add(testChain2)
	pMgr.staleChains.add(testChain3)
	chainsToCleanup := pMgr.staleChains.emptyAndGetAll()
	sort.Strings(chainsToCleanup)
	require.Error(t, pMgr.cleanupChains(chainsToCleanup))
	assertStaleChainsContain(t, pMgr.staleChains, testChain1, testChain3)
}

func TestCreatorForBootup(t *testing.T) {
	v1Chains := []string{
		"AZURE-NPM-INGRESS-DROPS",
		"AZURE-NPM-INGRESS-TO",
		"AZURE-NPM-INGRESS-PORTS",
		"AZURE-NPM-EGRESS-DROPS",
		"AZURE-NPM-EGRESS-FROM",
		"AZURE-NPM-EGRESS-PORTS",
	}

	tests := []struct {
		name                string
		currentChains       []string
		expectedLines       []string
		expectedStaleChains []string
	}{
		{
			name:          "no NPM prior",
			currentChains: []string{},
			expectedLines: []string{
				"*filter",
				":AZURE-NPM - -",
				":AZURE-NPM-INGRESS - -",
				":AZURE-NPM-INGRESS-ALLOW-MARK - -",
				":AZURE-NPM-EGRESS - -",
				":AZURE-NPM-ACCEPT - -",
				"-A AZURE-NPM-INGRESS -j DROP -m mark --mark 0x400/0x400 -m comment --comment DROP-ON-INGRESS-DROP-MARK-0x400/0x400",
				"-A AZURE-NPM-INGRESS-ALLOW-MARK -j MARK --set-mark 0x200/0x200 -m comment --comment SET-INGRESS-ALLOW-MARK-0x200/0x200",
				"-A AZURE-NPM-INGRESS-ALLOW-MARK -j AZURE-NPM-EGRESS",
				"-A AZURE-NPM-EGRESS -j DROP -m mark --mark 0x800/0x800 -m comment --comment DROP-ON-EGRESS-DROP-MARK-0x800/0x800",
				"-A AZURE-NPM-EGRESS -j AZURE-NPM-ACCEPT -m mark --mark 0x200/0x200 -m comment --comment ACCEPT-ON-INGRESS-ALLOW-MARK-0x200/0x200",
				"-A AZURE-NPM-ACCEPT -j ACCEPT",
				"COMMIT",
				"",
			},
			expectedStaleChains: []string{},
		},
		{
			name: "NPM v2 existed before with old v2 policy chains",
			currentChains: []string{
				"AZURE-NPM",
				"AZURE-NPM-INGRESS",
				"AZURE-NPM-INGRESS-ALLOW-MARK",
				"AZURE-NPM-EGRESS",
				"AZURE-NPM-ACCEPT",
				"AZURE-NPM-INGRESS-123456",
				"AZURE-NPM-EGRESS-123456",
			},
			// same expected lines as "no NPM prior", except for the old v2 policy chains in the header
			expectedLines: []string{
				"*filter",
				"-F AZURE-NPM",
				"-F AZURE-NPM-INGRESS",
				"-F AZURE-NPM-INGRESS-ALLOW-MARK",
				"-F AZURE-NPM-EGRESS",
				"-F AZURE-NPM-ACCEPT",
				"-F AZURE-NPM-INGRESS-123456",
				"-F AZURE-NPM-EGRESS-123456",
				"-A AZURE-NPM-INGRESS -j DROP -m mark --mark 0x400/0x400 -m comment --comment DROP-ON-INGRESS-DROP-MARK-0x400/0x400",
				"-A AZURE-NPM-INGRESS-ALLOW-MARK -j MARK --set-mark 0x200/0x200 -m comment --comment SET-INGRESS-ALLOW-MARK-0x200/0x200",
				"-A AZURE-NPM-INGRESS-ALLOW-MARK -j AZURE-NPM-EGRESS",
				"-A AZURE-NPM-EGRESS -j DROP -m mark --mark 0x800/0x800 -m comment --comment DROP-ON-EGRESS-DROP-MARK-0x800/0x800",
				"-A AZURE-NPM-EGRESS -j AZURE-NPM-ACCEPT -m mark --mark 0x200/0x200 -m comment --comment ACCEPT-ON-INGRESS-ALLOW-MARK-0x200/0x200",
				"-A AZURE-NPM-ACCEPT -j ACCEPT",
				"COMMIT",
				"",
			},
			expectedStaleChains: []string{
				"AZURE-NPM-EGRESS-123456",
				"AZURE-NPM-INGRESS-123456",
			},
		},
		{
			name: "NPM v2 existed before but some chains are missing",
			currentChains: []string{
				"AZURE-NPM-INGRESS",
				"AZURE-NPM-INGRESS-ALLOW-MARK",
				"AZURE-NPM-ACCEPT",
			},
			// same expected lines as "no NPM prior", except for the old v2 policy chains in the header
			expectedLines: []string{
				"*filter",
				":AZURE-NPM - -",
				":AZURE-NPM-EGRESS - -",
				"-F AZURE-NPM-ACCEPT",
				"-F AZURE-NPM-INGRESS",
				"-F AZURE-NPM-INGRESS-ALLOW-MARK",
				"-A AZURE-NPM-INGRESS -j DROP -m mark --mark 0x400/0x400 -m comment --comment DROP-ON-INGRESS-DROP-MARK-0x400/0x400",
				"-A AZURE-NPM-INGRESS-ALLOW-MARK -j MARK --set-mark 0x200/0x200 -m comment --comment SET-INGRESS-ALLOW-MARK-0x200/0x200",
				"-A AZURE-NPM-INGRESS-ALLOW-MARK -j AZURE-NPM-EGRESS",
				"-A AZURE-NPM-EGRESS -j DROP -m mark --mark 0x800/0x800 -m comment --comment DROP-ON-EGRESS-DROP-MARK-0x800/0x800",
				"-A AZURE-NPM-EGRESS -j AZURE-NPM-ACCEPT -m mark --mark 0x200/0x200 -m comment --comment ACCEPT-ON-INGRESS-ALLOW-MARK-0x200/0x200",
				"-A AZURE-NPM-ACCEPT -j ACCEPT",
				"COMMIT",
				"",
			},
			expectedStaleChains: []string{},
		},
		{
			name:          "NPM v1 existed prior",
			currentChains: v1Chains,
			// same expected lines as "no NPM prior", except for the deprecated chains in the header
			expectedLines: []string{
				"*filter",
				":AZURE-NPM - -",
				":AZURE-NPM-INGRESS - -",
				":AZURE-NPM-INGRESS-ALLOW-MARK - -",
				":AZURE-NPM-EGRESS - -",
				":AZURE-NPM-ACCEPT - -",
				"-F AZURE-NPM-INGRESS-DROPS",
				"-F AZURE-NPM-INGRESS-TO",
				"-F AZURE-NPM-INGRESS-PORTS",
				"-F AZURE-NPM-EGRESS-DROPS",
				"-F AZURE-NPM-EGRESS-FROM",
				"-F AZURE-NPM-EGRESS-PORTS",
				"-A AZURE-NPM-INGRESS -j DROP -m mark --mark 0x400/0x400 -m comment --comment DROP-ON-INGRESS-DROP-MARK-0x400/0x400",
				"-A AZURE-NPM-INGRESS-ALLOW-MARK -j MARK --set-mark 0x200/0x200 -m comment --comment SET-INGRESS-ALLOW-MARK-0x200/0x200",
				"-A AZURE-NPM-INGRESS-ALLOW-MARK -j AZURE-NPM-EGRESS",
				"-A AZURE-NPM-EGRESS -j DROP -m mark --mark 0x800/0x800 -m comment --comment DROP-ON-EGRESS-DROP-MARK-0x800/0x800",
				"-A AZURE-NPM-EGRESS -j AZURE-NPM-ACCEPT -m mark --mark 0x200/0x200 -m comment --comment ACCEPT-ON-INGRESS-ALLOW-MARK-0x200/0x200",
				"-A AZURE-NPM-ACCEPT -j ACCEPT",
				"COMMIT",
				"",
			},
			expectedStaleChains: v1Chains,
		},
	}
	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			ioshim := common.NewMockIOShim(nil)
			defer ioshim.VerifyCalls(t, nil)
			pMgr := NewPolicyManager(ioshim, ipsetConfig)
			creator := pMgr.creatorForBootup(stringsToMap(tt.currentChains))
			actualLines := strings.Split(creator.ToString(), "\n")
			sortedActualLines := sortFlushes(actualLines)
			sortedExpectedLines := sortFlushes(tt.expectedLines)
			dptestutils.AssertEqualLines(t, sortedExpectedLines, sortedActualLines)
			assertStaleChainsContain(t, pMgr.staleChains, tt.expectedStaleChains...)
		})
	}
}

func sortFlushes(lines []string) []string {
	result := make([]string, len(lines))
	copy(result, lines)
	flushStart := len(lines)
	for i, line := range lines {
		if len(line) > 2 && line[:2] == "-F" {
			flushStart = i
			break
		}
	}
	flushLines := make([]string, 0)
	for i := flushStart; i < len(lines); i++ {
		line := lines[i]
		if line[:2] != "-F" {
			break
		}
		flushLines = append(flushLines, line)
	}
	sort.Strings(flushLines)
	for i, line := range flushLines {
		result[i+flushStart] = line
	}
	return result
}

func TestBootupLinux(t *testing.T) {
	tests := []struct {
		name    string
		calls   []testutils.TestCmd
		wantErr bool
	}{
		// all tests with "no NPM prior" work for any situation (with v1 or v2 prior),
		// but the fake command exit codes and stdouts are in line with having no NPM prior
		{
			name:    "success (no NPM prior)",
			calls:   GetBootupTestCalls(false),
			wantErr: false,
		},
		{
			name: "success after restore failure (no NPM prior)",
			calls: []testutils.TestCmd{
				{
					Cmd:      []string{"iptables", "-w", "60", "-D", "FORWARD", "-j", "AZURE-NPM"},
					ExitCode: 2,
					Stdout:   "iptables v1.8.4 (legacy): Couldn't load target `AZURE-NPM':No such file or directory",
				}, // AZURE-NPM chain didn't exist
				{Cmd: listAllCommandStrings, PipedToCommand: true},
				{Cmd: []string{"grep", "Chain AZURE-NPM"}, ExitCode: 1},
				fakeIPTablesRestoreFailureCommand, // e.g. xtables lock held by another app. Currently the stdout doesn't matter for retrying
				fakeIPTablesRestoreCommand,
				{Cmd: listLineNumbersCommandStrings, PipedToCommand: true},
				{Cmd: []string{"grep", "AZURE-NPM"}, ExitCode: 1},
				{Cmd: []string{"iptables", "-w", "60", "-I", "FORWARD", "-j", "AZURE-NPM", "-m", "conntrack", "--ctstate", "NEW"}},
			},
			wantErr: false,
		},
		{
			name: "success: v2 existed prior",
			calls: []testutils.TestCmd{
				{
					Cmd:      []string{"iptables", "-w", "60", "-D", "FORWARD", "-j", "AZURE-NPM"},
					ExitCode: 1,
					Stdout:   "No chain/target/match by that name",
				}, // deprecated rule did not exist
				{Cmd: listAllCommandStrings, PipedToCommand: true},
				{
					Cmd:    []string{"grep", "Chain AZURE-NPM"},
					Stdout: grepOutputAzureChainsWithoutPolicies,
				},
				fakeIPTablesRestoreCommand,
				{Cmd: listLineNumbersCommandStrings, PipedToCommand: true},
				{Cmd: []string{"grep", "AZURE-NPM"}, ExitCode: 1},
				{Cmd: []string{"iptables", "-w", "60", "-I", "FORWARD", "-j", "AZURE-NPM", "-m", "conntrack", "--ctstate", "NEW"}},
			},
			wantErr: false,
		},
		{
			name: "v1 existed prior: successfully delete deprecated jump",
			calls: []testutils.TestCmd{
				{Cmd: []string{"iptables", "-w", "60", "-D", "FORWARD", "-j", "AZURE-NPM"}}, // deprecated rule existed
				{Cmd: listAllCommandStrings, PipedToCommand: true},
				{
					Cmd:    []string{"grep", "Chain AZURE-NPM"},
					Stdout: grepOutputAzureV1Chains,
				},
				fakeIPTablesRestoreCommand,
				{Cmd: listLineNumbersCommandStrings, PipedToCommand: true},
				{Cmd: []string{"grep", "AZURE-NPM"}, ExitCode: 1},
				{Cmd: []string{"iptables", "-w", "60", "-I", "FORWARD", "-j", "AZURE-NPM", "-m", "conntrack", "--ctstate", "NEW"}},
			},
			wantErr: false,
		},
		{
			name: "v1 existed prior: unknown error while deleting deprecated jump",
			calls: []testutils.TestCmd{
				{Cmd: []string{"iptables", "-w", "60", "-D", "FORWARD", "-j", "AZURE-NPM"}, ExitCode: 3}, // unknown error
				{Cmd: listAllCommandStrings, PipedToCommand: true},
				{
					Cmd:    []string{"grep", "Chain AZURE-NPM"},
					Stdout: grepOutputAzureV1Chains,
				},
				fakeIPTablesRestoreCommand,
				{Cmd: listLineNumbersCommandStrings, PipedToCommand: true},
				{Cmd: []string{"grep", "AZURE-NPM"}, ExitCode: 1},
				{Cmd: []string{"iptables", "-w", "60", "-I", "FORWARD", "-j", "AZURE-NPM", "-m", "conntrack", "--ctstate", "NEW"}},
			},
			wantErr: false,
		},
		{
			name: "failure while finding current chains (no NPM prior)",
			calls: []testutils.TestCmd{
				{Cmd: []string{"iptables", "-w", "60", "-D", "FORWARD", "-j", "AZURE-NPM"}, ExitCode: 2}, // AZURE-NPM chain didn't exist
				{Cmd: listAllCommandStrings, PipedToCommand: true, HasStartError: true, ExitCode: 1},
				{Cmd: []string{"grep", "Chain AZURE-NPM"}},
			},
			wantErr: true,
		},
		{
			name: "failure twice on restore (no NPM prior)",
			calls: []testutils.TestCmd{
				{Cmd: []string{"iptables", "-w", "60", "-D", "FORWARD", "-j", "AZURE-NPM"}, ExitCode: 2}, // AZURE-NPM chain didn't exist
				{Cmd: listAllCommandStrings, PipedToCommand: true},
				{Cmd: []string{"grep", "Chain AZURE-NPM"}, ExitCode: 1},
				fakeIPTablesRestoreFailureCommand,
				fakeIPTablesRestoreFailureCommand,
			},
			wantErr: true,
		},
		{
			name: "failure on position (no NPM prior)",
			calls: []testutils.TestCmd{
				{Cmd: []string{"iptables", "-w", "60", "-D", "FORWARD", "-j", "AZURE-NPM"}, ExitCode: 2}, // AZURE-NPM chain didn't exist
				{Cmd: listAllCommandStrings, PipedToCommand: true},
				{
					Cmd:    []string{"grep", "Chain AZURE-NPM"},
					Stdout: grepOutputAzureChainsWithoutPolicies,
				},
				fakeIPTablesRestoreCommand,
				{Cmd: listLineNumbersCommandStrings, PipedToCommand: true},
				{Cmd: []string{"grep", "AZURE-NPM"}, ExitCode: 1},
				{
					Cmd:      []string{"iptables", "-w", "60", "-I", "FORWARD", "-j", "AZURE-NPM", "-m", "conntrack", "--ctstate", "NEW"},
					ExitCode: 1,
				},
			},
			wantErr: true,
		},
	}
	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			ioshim := common.NewMockIOShim(tt.calls)
			defer ioshim.VerifyCalls(t, tt.calls)
			pMgr := NewPolicyManager(ioshim, ipsetConfig)
			err := pMgr.bootup(nil)
			if tt.wantErr {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
			}
		})
	}
}

func TestPositionAzureChainJumpRule(t *testing.T) {
	tests := []struct {
		name                 string
		calls                []testutils.TestCmd
		placeAzureChainFirst bool
		wantErr              bool
	}{
		{
			name: "place first: no jump rule yet",
			calls: []testutils.TestCmd{
				{Cmd: listLineNumbersCommandStrings, PipedToCommand: true},
				{Cmd: []string{"grep", "AZURE-NPM"}, ExitCode: 1},
				{Cmd: []string{"iptables", "-w", "60", "-I", "FORWARD", "-j", "AZURE-NPM", "-m", "conntrack", "--ctstate", "NEW"}},
			},
			placeAzureChainFirst: util.PlaceAzureChainFirst,
			wantErr:              false,
		},
		{
			name: "place first: no jump rule yet and insert fails",
			calls: []testutils.TestCmd{
				{Cmd: listLineNumbersCommandStrings, PipedToCommand: true},
				{Cmd: []string{"grep", "AZURE-NPM"}, ExitCode: 1},
				{Cmd: []string{"iptables", "-w", "60", "-I", "FORWARD", "-j", "AZURE-NPM", "-m", "conntrack", "--ctstate", "NEW"}, ExitCode: 1},
			},
			placeAzureChainFirst: util.PlaceAzureChainFirst,
			wantErr:              true,
		},
		{
			name: "command error while grepping",
			calls: []testutils.TestCmd{
				{Cmd: listLineNumbersCommandStrings, PipedToCommand: true, HasStartError: true, ExitCode: 1},
				{Cmd: []string{"grep", "AZURE-NPM"}},
			},
			placeAzureChainFirst: util.PlaceAzureChainFirst,
			wantErr:              true,
		},
		{
			name: "place first: jump rule already at top",
			calls: []testutils.TestCmd{
				{Cmd: listLineNumbersCommandStrings, PipedToCommand: true},
				{
					Cmd:    []string{"grep", "AZURE-NPM"},
					Stdout: "1    AZURE-NPM  all  --  0.0.0.0/0            0.0.0.0/0    ...",
				},
			},
			placeAzureChainFirst: util.PlaceAzureChainFirst,
			wantErr:              false,
		},
		{
			name: "place first: jump rule not at top",
			calls: []testutils.TestCmd{
				{Cmd: listLineNumbersCommandStrings, PipedToCommand: true},
				{
					Cmd:    []string{"grep", "AZURE-NPM"},
					Stdout: "2    AZURE-NPM  all  --  0.0.0.0/0            0.0.0.0/0    ...",
				},
				{Cmd: []string{"iptables", "-w", "60", "-D", "FORWARD", "-j", "AZURE-NPM", "-m", "conntrack", "--ctstate", "NEW"}},
				{Cmd: []string{"iptables", "-w", "60", "-I", "FORWARD", "-j", "AZURE-NPM", "-m", "conntrack", "--ctstate", "NEW"}},
			},
			placeAzureChainFirst: util.PlaceAzureChainFirst,
			wantErr:              false,
		},
		{
			name: "place first: jump rule not at top and delete fails",
			calls: []testutils.TestCmd{
				{Cmd: listLineNumbersCommandStrings, PipedToCommand: true},
				{
					Cmd:    []string{"grep", "AZURE-NPM"},
					Stdout: "2    AZURE-NPM  all  --  0.0.0.0/0            0.0.0.0/0    ...",
				},
				{Cmd: []string{"iptables", "-w", "60", "-D", "FORWARD", "-j", "AZURE-NPM", "-m", "conntrack", "--ctstate", "NEW"}, ExitCode: 1},
			},
			placeAzureChainFirst: util.PlaceAzureChainFirst,
			wantErr:              true,
		},
		{
			name: "place first: jump rule not at top and insert fails",
			calls: []testutils.TestCmd{
				{Cmd: listLineNumbersCommandStrings, PipedToCommand: true},
				{
					Cmd:    []string{"grep", "AZURE-NPM"},
					Stdout: "2    AZURE-NPM  all  --  0.0.0.0/0            0.0.0.0/0    ...",
				},
				{Cmd: []string{"iptables", "-w", "60", "-D", "FORWARD", "-j", "AZURE-NPM", "-m", "conntrack", "--ctstate", "NEW"}},
				{Cmd: []string{"iptables", "-w", "60", "-I", "FORWARD", "-j", "AZURE-NPM", "-m", "conntrack", "--ctstate", "NEW"}, ExitCode: 1},
			},
			placeAzureChainFirst: util.PlaceAzureChainFirst,
			wantErr:              true,
		},
		{
			name: "after kube: neither jump rule exists",
			calls: []testutils.TestCmd{
				{Cmd: listLineNumbersCommandStrings, PipedToCommand: true},
				{Cmd: []string{"grep", "AZURE-NPM"}, ExitCode: 1},
				{Cmd: listLineNumbersCommandStrings, PipedToCommand: true},
				{Cmd: []string{"grep", "KUBE-SERVICES"}, ExitCode: 1},
				{Cmd: []string{"iptables", "-w", "60", "-I", "FORWARD", "-j", "AZURE-NPM", "-m", "conntrack", "--ctstate", "NEW"}},
			},
			placeAzureChainFirst: util.PlaceAzureChainAfterKubeServices,
			wantErr:              false,
		},
		{
			name: "after kube: no azure jump rule yet and kube jump rule exists",
			calls: []testutils.TestCmd{
				{Cmd: listLineNumbersCommandStrings, PipedToCommand: true},
				{Cmd: []string{"grep", "AZURE-NPM"}, ExitCode: 1},
				{Cmd: listLineNumbersCommandStrings, PipedToCommand: true},
				{
					Cmd:    []string{"grep", "KUBE-SERVICES"},
					Stdout: "3  KUBE-SERVICES  all  --  0.0.0.0/0            0.0.0.0/0    ...",
				},
				{Cmd: []string{"iptables", "-w", "60", "-I", "FORWARD", "4", "-j", "AZURE-NPM", "-m", "conntrack", "--ctstate", "NEW"}},
			},
			placeAzureChainFirst: util.PlaceAzureChainAfterKubeServices,
			wantErr:              false,
		},
		{
			name: "after kube: only azure jump rule exists and the position is correct",
			calls: []testutils.TestCmd{
				{Cmd: listLineNumbersCommandStrings, PipedToCommand: true},
				{
					Cmd:    []string{"grep", "AZURE-NPM"},
					Stdout: "2    AZURE-NPM  all  --  0.0.0.0/0            0.0.0.0/0    ...",
				},
				{Cmd: listLineNumbersCommandStrings, PipedToCommand: true},
				{Cmd: []string{"grep", "KUBE-SERVICES"}, ExitCode: 1},
				{Cmd: []string{"iptables", "-w", "60", "-D", "FORWARD", "-j", "AZURE-NPM", "-m", "conntrack", "--ctstate", "NEW"}},
				{Cmd: []string{"iptables", "-w", "60", "-I", "FORWARD", "-j", "AZURE-NPM", "-m", "conntrack", "--ctstate", "NEW"}},
			},
			placeAzureChainFirst: util.PlaceAzureChainAfterKubeServices,
			wantErr:              false,
		},
		{
			name: "after kube: only azure jump rule exists and the position is wrong",
			calls: []testutils.TestCmd{
				{Cmd: listLineNumbersCommandStrings, PipedToCommand: true},
				{
					Cmd:    []string{"grep", "AZURE-NPM"},
					Stdout: "2    AZURE-NPM  all  --  0.0.0.0/0            0.0.0.0/0    ...",
				},
				{Cmd: listLineNumbersCommandStrings, PipedToCommand: true},
				{Cmd: []string{"grep", "KUBE-SERVICES"}, ExitCode: 1},
				{Cmd: []string{"iptables", "-w", "60", "-D", "FORWARD", "-j", "AZURE-NPM", "-m", "conntrack", "--ctstate", "NEW"}},
				{Cmd: []string{"iptables", "-w", "60", "-I", "FORWARD", "-j", "AZURE-NPM", "-m", "conntrack", "--ctstate", "NEW"}},
			},
			placeAzureChainFirst: util.PlaceAzureChainAfterKubeServices,
			wantErr:              false,
		},
		{
			name: "after kube: both jumps exist and positions are correct",
			calls: []testutils.TestCmd{
				{Cmd: listLineNumbersCommandStrings, PipedToCommand: true},
				{
					Cmd:    []string{"grep", "AZURE-NPM"},
					Stdout: "4    AZURE-NPM  all  --  0.0.0.0/0            0.0.0.0/0    ...",
				},
				{Cmd: listLineNumbersCommandStrings, PipedToCommand: true},
				{
					Cmd:    []string{"grep", "KUBE-SERVICES"},
					Stdout: "3  KUBE-SERVICES  all  --  0.0.0.0/0            0.0.0.0/0    ...",
				},
			},
			placeAzureChainFirst: util.PlaceAzureChainAfterKubeServices,
			wantErr:              false,
		},
		{
			name: "after kube: both jumps exist and the azure jump is too far below the kube jump",
			calls: []testutils.TestCmd{
				{Cmd: listLineNumbersCommandStrings, PipedToCommand: true},
				{
					Cmd:    []string{"grep", "AZURE-NPM"},
					Stdout: "5    AZURE-NPM  all  --  0.0.0.0/0            0.0.0.0/0    ...",
				},
				{Cmd: listLineNumbersCommandStrings, PipedToCommand: true},
				{
					Cmd:    []string{"grep", "KUBE-SERVICES"},
					Stdout: "3  KUBE-SERVICES  all  --  0.0.0.0/0            0.0.0.0/0    ...",
				},
				{Cmd: []string{"iptables", "-w", "60", "-D", "FORWARD", "-j", "AZURE-NPM", "-m", "conntrack", "--ctstate", "NEW"}},
				{Cmd: []string{"iptables", "-w", "60", "-I", "FORWARD", "4", "-j", "AZURE-NPM", "-m", "conntrack", "--ctstate", "NEW"}},
			},
			placeAzureChainFirst: util.PlaceAzureChainAfterKubeServices,
			wantErr:              false,
		},
		{
			name: "after kube: both jumps exist and the azure jump is above the kube jump",
			calls: []testutils.TestCmd{
				{Cmd: listLineNumbersCommandStrings, PipedToCommand: true},
				{
					Cmd:    []string{"grep", "AZURE-NPM"},
					Stdout: "2    AZURE-NPM  all  --  0.0.0.0/0            0.0.0.0/0    ...",
				},
				{Cmd: listLineNumbersCommandStrings, PipedToCommand: true},
				{
					Cmd:    []string{"grep", "KUBE-SERVICES"},
					Stdout: "3  KUBE-SERVICES  all  --  0.0.0.0/0            0.0.0.0/0    ...",
				},
				{Cmd: []string{"iptables", "-w", "60", "-D", "FORWARD", "-j", "AZURE-NPM", "-m", "conntrack", "--ctstate", "NEW"}},
				{Cmd: []string{"iptables", "-w", "60", "-I", "FORWARD", "3", "-j", "AZURE-NPM", "-m", "conntrack", "--ctstate", "NEW"}},
			},
			placeAzureChainFirst: util.PlaceAzureChainAfterKubeServices,
			wantErr:              false,
		},
		{
			name: "after kube: error getting kube jump line number",
			calls: []testutils.TestCmd{
				{Cmd: listLineNumbersCommandStrings, PipedToCommand: true},
				{Cmd: []string{"grep", "AZURE-NPM"}, ExitCode: 1},
				// should add ExitCode: 1 to below, but this causes an error for the previous commands actually...
				{Cmd: listLineNumbersCommandStrings, PipedToCommand: true, HasStartError: true},
				{Cmd: []string{"grep", "KUBE-SERVICES"}},
			},
			placeAzureChainFirst: util.PlaceAzureChainAfterKubeServices,
			wantErr:              true,
		},
		{
			name: "after kube: error creating azure jump when kube jump rule exists",
			calls: []testutils.TestCmd{
				{Cmd: listLineNumbersCommandStrings, PipedToCommand: true},
				{Cmd: []string{"grep", "AZURE-NPM"}, ExitCode: 1},
				{Cmd: listLineNumbersCommandStrings, PipedToCommand: true},
				{
					Cmd:    []string{"grep", "KUBE-SERVICES"},
					Stdout: "3  KUBE-SERVICES  all  --  0.0.0.0/0            0.0.0.0/0    ...",
				},
				{Cmd: []string{"iptables", "-w", "60", "-I", "FORWARD", "4", "-j", "AZURE-NPM", "-m", "conntrack", "--ctstate", "NEW"}, ExitCode: 1},
			},
			placeAzureChainFirst: util.PlaceAzureChainAfterKubeServices,
			wantErr:              true,
		},
		{
			name: "after kube: error deleting azure jump when positions are wrong",
			calls: []testutils.TestCmd{
				{Cmd: listLineNumbersCommandStrings, PipedToCommand: true},
				{
					Cmd:    []string{"grep", "AZURE-NPM"},
					Stdout: "2    AZURE-NPM  all  --  0.0.0.0/0            0.0.0.0/0    ...",
				},
				{Cmd: listLineNumbersCommandStrings, PipedToCommand: true},
				{
					Cmd:    []string{"grep", "KUBE-SERVICES"},
					Stdout: "3  KUBE-SERVICES  all  --  0.0.0.0/0            0.0.0.0/0    ...",
				},
				{Cmd: []string{"iptables", "-w", "60", "-D", "FORWARD", "-j", "AZURE-NPM", "-m", "conntrack", "--ctstate", "NEW"}, ExitCode: 1},
			},
			placeAzureChainFirst: util.PlaceAzureChainAfterKubeServices,
			wantErr:              true,
		},
		{
			name: "after kube: error adding back azure jump",
			calls: []testutils.TestCmd{
				{Cmd: listLineNumbersCommandStrings, PipedToCommand: true},
				{
					Cmd:    []string{"grep", "AZURE-NPM"},
					Stdout: "2    AZURE-NPM  all  --  0.0.0.0/0            0.0.0.0/0    ...",
				},
				{Cmd: listLineNumbersCommandStrings, PipedToCommand: true},
				{
					Cmd:    []string{"grep", "KUBE-SERVICES"},
					Stdout: "3  KUBE-SERVICES  all  --  0.0.0.0/0            0.0.0.0/0    ...",
				},
				{Cmd: []string{"iptables", "-w", "60", "-D", "FORWARD", "-j", "AZURE-NPM", "-m", "conntrack", "--ctstate", "NEW"}},
				{Cmd: []string{"iptables", "-w", "60", "-I", "FORWARD", "3", "-j", "AZURE-NPM", "-m", "conntrack", "--ctstate", "NEW"}, ExitCode: 1},
			},
			placeAzureChainFirst: util.PlaceAzureChainAfterKubeServices,
			wantErr:              true,
		},
	}
	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			ioshim := common.NewMockIOShim(tt.calls)
			defer ioshim.VerifyCalls(t, tt.calls)
			cfg := &PolicyManagerCfg{
				PolicyMode:           IPSetPolicyMode, // value doesn't matter for Linux
				PlaceAzureChainFirst: tt.placeAzureChainFirst,
			}
			pMgr := NewPolicyManager(ioshim, cfg)
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
					Stdout: fmt.Sprintf("12    %s  all  --  0.0.0.0/0            0.0.0.0/0 ", testChainName),
				},
			},
			expectedLineNum: 12,
			wantErr:         false,
		},
		{
			name: "unexpected grep output (too short)",
			calls: []testutils.TestCmd{
				{Cmd: listLineNumbersCommandStrings, PipedToCommand: true},
				{
					Cmd:    []string{"grep", testChainName},
					Stdout: "3",
				},
			},
			expectedLineNum: 0,
			wantErr:         true,
		},
		{
			name: "unexpected grep output (no space)",
			calls: []testutils.TestCmd{
				{Cmd: listLineNumbersCommandStrings, PipedToCommand: true},
				{
					Cmd:    []string{"grep", testChainName},
					Stdout: "345678",
				},
			},
			expectedLineNum: 0,
			wantErr:         true,
		},
		{
			name: "unexpected grep output (no line number)",
			calls: []testutils.TestCmd{
				{Cmd: listLineNumbersCommandStrings, PipedToCommand: true},
				{
					Cmd:    []string{"grep", testChainName},
					Stdout: "unexpected stuff",
				},
			},
			expectedLineNum: 0,
			wantErr:         true,
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
			pMgr := NewPolicyManager(ioshim, ipsetConfig)
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

func getFakeDestroyCommand(chain string) testutils.TestCmd {
	return testutils.TestCmd{Cmd: []string{"iptables", "-w", "60", "-X", chain}}
}

func getFakeDestroyCommandWithExitCode(chain string, exitCode int) testutils.TestCmd {
	command := getFakeDestroyCommand(chain)
	command.ExitCode = exitCode
	return command
}

func stringsToMap(items []string) map[string]struct{} {
	if items == nil {
		return nil
	}
	m := make(map[string]struct{})
	for _, s := range items {
		m[s] = struct{}{}
	}
	return m
}
