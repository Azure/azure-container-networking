package policies

import (
	"fmt"
	"strings"
	"testing"

	"github.com/Azure/azure-container-networking/common"
	"github.com/Azure/azure-container-networking/npm/pkg/dataplane/ipsets"
	dptestutils "github.com/Azure/azure-container-networking/npm/pkg/dataplane/testutils"
	"github.com/Azure/azure-container-networking/npm/util"
	testutils "github.com/Azure/azure-container-networking/test/utils"
	"github.com/stretchr/testify/require"
)

var (
	testPolicy1IngressChain = TestNetworkPolicies[0].ingressChainName()
	testPolicy1EgressChain  = TestNetworkPolicies[0].egressChainName()
	testPolicy2IngressChain = TestNetworkPolicies[1].ingressChainName()
	testPolicy3EgressChain  = TestNetworkPolicies[2].egressChainName()

	testPolicy1IngressJump = fmt.Sprintf("-j %s -m set --match-set %s dst", testPolicy1IngressChain, ipsets.TestKeyPodSet.HashedName)
	testPolicy1EgressJump  = fmt.Sprintf("-j %s -m set --match-set %s src", testPolicy1EgressChain, ipsets.TestKeyPodSet.HashedName)
	testPolicy2IngressJump = fmt.Sprintf("-j %s -m set --match-set %s dst -m set --match-set %s dst", testPolicy2IngressChain, ipsets.TestKeyPodSet.HashedName, ipsets.TestKVPodSet.HashedName)
	testPolicy3EgressJump  = fmt.Sprintf("-j %s", testPolicy3EgressChain)

	testACLRule1 = fmt.Sprintf(
		"-j MARK --set-mark 0x4000 -p TCP --dport 222:333 -m set --match-set %s src -m set ! --match-set %s dst -m comment --comment comment1",
		ipsets.TestCIDRSet.HashedName,
		ipsets.TestKeyPodSet.HashedName,
	)
	testACLRule2 = fmt.Sprintf("-j AZURE-NPM-INGRESS-ALLOW-MARK -p UDP -m set --match-set %s src -m comment --comment comment2", ipsets.TestCIDRSet.HashedName)
	testACLRule3 = fmt.Sprintf("-j MARK --set-mark 0x5000 -p UDP --dport 144 -m set --match-set %s src -m comment --comment comment3", ipsets.TestCIDRSet.HashedName)
	testACLRule4 = fmt.Sprintf("-j AZURE-NPM-ACCEPT -m set --match-set %s src -m comment --comment comment4", ipsets.TestCIDRSet.HashedName)
)

func TestChainNames(t *testing.T) {
	expectedName := fmt.Sprintf("AZURE-NPM-INGRESS-%s", util.Hash(TestNetworkPolicies[0].PolicyKey))
	require.Equal(t, expectedName, TestNetworkPolicies[0].ingressChainName())
	expectedName = fmt.Sprintf("AZURE-NPM-EGRESS-%s", util.Hash(TestNetworkPolicies[0].PolicyKey))
	require.Equal(t, expectedName, TestNetworkPolicies[0].egressChainName())
}

func TestCreatorForAddPolicies(t *testing.T) {
	calls := []testutils.TestCmd{fakeIPTablesRestoreCommand}
	ioshim := common.NewMockIOShim(calls)
	defer ioshim.VerifyCalls(t, calls)
	pMgr := NewPolicyManager(ioshim, IPSetConfig)

	// 1. test with activation
	policies := []*NPMNetworkPolicy{TestNetworkPolicies[0]}
	creator := pMgr.creatorForNewNetworkPolicies(allChainNames(policies), policies)
	actualLines := strings.Split(creator.ToString(), "\n")
	expectedLines := []string{
		"*filter",
		// all chains
		fmt.Sprintf(":%s - -", testPolicy1IngressChain),
		fmt.Sprintf(":%s - -", testPolicy1EgressChain),
		"-F AZURE-NPM",
		// activation rules for AZURE-NPM chain
		"-A AZURE-NPM -j AZURE-NPM-INGRESS",
		"-A AZURE-NPM -j AZURE-NPM-EGRESS",
		"-A AZURE-NPM -j AZURE-NPM-ACCEPT",
		// policy 1
		fmt.Sprintf("-A %s %s", testPolicy1IngressChain, testACLRule1),
		fmt.Sprintf("-A %s %s", testPolicy1IngressChain, testACLRule2),
		fmt.Sprintf("-A %s %s", testPolicy1EgressChain, testACLRule3),
		fmt.Sprintf("-A %s %s", testPolicy1EgressChain, testACLRule4),
		fmt.Sprintf("-I AZURE-NPM-INGRESS 1 %s", testPolicy1IngressJump),
		fmt.Sprintf("-I AZURE-NPM-EGRESS 1 %s", testPolicy1EgressJump),
		"COMMIT",
		"",
	}
	dptestutils.AssertEqualLines(t, expectedLines, actualLines)

	// 2. test without activation
	// add a policy to the cache so that we don't activate (the cache doesn't impact creatorForNewNetworkPolicies)
	require.NoError(t, pMgr.AddPolicy(TestNetworkPolicies[0], nil))
	creator = pMgr.creatorForNewNetworkPolicies(allChainNames(TestNetworkPolicies), TestNetworkPolicies)
	actualLines = strings.Split(creator.ToString(), "\n")
	expectedLines = []string{
		"*filter",
		// all chains
		fmt.Sprintf(":%s - -", testPolicy1IngressChain),
		fmt.Sprintf(":%s - -", testPolicy1EgressChain),
		fmt.Sprintf(":%s - -", testPolicy2IngressChain),
		fmt.Sprintf(":%s - -", testPolicy3EgressChain),
		// policy 1
		fmt.Sprintf("-A %s %s", testPolicy1IngressChain, testACLRule1),
		fmt.Sprintf("-A %s %s", testPolicy1IngressChain, testACLRule2),
		fmt.Sprintf("-A %s %s", testPolicy1EgressChain, testACLRule3),
		fmt.Sprintf("-A %s %s", testPolicy1EgressChain, testACLRule4),
		fmt.Sprintf("-I AZURE-NPM-INGRESS 1 %s", testPolicy1IngressJump),
		fmt.Sprintf("-I AZURE-NPM-EGRESS 1 %s", testPolicy1EgressJump),
		// policy 2
		fmt.Sprintf("-A %s %s", testPolicy2IngressChain, testACLRule1),
		fmt.Sprintf("-I AZURE-NPM-INGRESS 2 %s", testPolicy2IngressJump),
		// policy 3
		fmt.Sprintf("-A %s %s", testPolicy3EgressChain, testACLRule4),
		fmt.Sprintf("-I AZURE-NPM-EGRESS 2 %s", testPolicy3EgressJump),
		"COMMIT",
		"",
	}
	dptestutils.AssertEqualLines(t, expectedLines, actualLines)
}

func TestCreatorForRemovePolicies(t *testing.T) {
	calls := []testutils.TestCmd{fakeIPTablesRestoreCommand}
	ioshim := common.NewMockIOShim(calls)
	defer ioshim.VerifyCalls(t, calls)
	pMgr := NewPolicyManager(ioshim, IPSetConfig)

	// 1. test without deactivation
	// hack: the cache is empty (and len(cache) != len(TestNetworkPolicies)), so shouldDeactivate will be false
	creator := pMgr.creatorForRemovingPolicies(allChainNames(TestNetworkPolicies))
	actualLines := strings.Split(creator.ToString(), "\n")
	expectedLines := []string{
		"*filter",
		fmt.Sprintf("-F %s", testPolicy1IngressChain),
		fmt.Sprintf("-F %s", testPolicy1EgressChain),
		fmt.Sprintf("-F %s", testPolicy2IngressChain),
		fmt.Sprintf("-F %s", testPolicy3EgressChain),
		"COMMIT",
		"",
	}
	dptestutils.AssertEqualLines(t, expectedLines, actualLines)

	// 2. test with deactivation
	// add to the cache so that we deactivate
	policy := TestNetworkPolicies[0]
	require.NoError(t, pMgr.AddPolicy(policy, nil))
	creator = pMgr.creatorForRemovingPolicies(allChainNames([]*NPMNetworkPolicy{policy}))
	actualLines = strings.Split(creator.ToString(), "\n")
	expectedLines = []string{
		"*filter",
		"-F AZURE-NPM",
		fmt.Sprintf("-F %s", testPolicy1IngressChain),
		fmt.Sprintf("-F %s", testPolicy1EgressChain),
		"COMMIT",
		"",
	}
	dptestutils.AssertEqualLines(t, expectedLines, actualLines)
}

func TestRemovePoliciesError(t *testing.T) {
	tests := []struct {
		name  string
		calls []testutils.TestCmd
	}{
		{
			name: "error on restore",
			calls: []testutils.TestCmd{
				fakeIPTablesRestoreCommand,
				getFakeDeleteJumpCommand("AZURE-NPM-INGRESS", testPolicy1IngressJump),
				getFakeDeleteJumpCommand("AZURE-NPM-EGRESS", testPolicy1EgressJump),
				fakeIPTablesRestoreFailureCommand,
			},
		},
		{
			name: "error on delete for ingress",
			calls: []testutils.TestCmd{
				fakeIPTablesRestoreCommand,
				getFakeDeleteJumpCommandWithCode("AZURE-NPM-INGRESS", testPolicy1IngressJump, 1), // anything but 0 or 2
			},
		},
		{
			name: "error on delete for egress",
			calls: []testutils.TestCmd{
				fakeIPTablesRestoreCommand,
				getFakeDeleteJumpCommand("AZURE-NPM-INGRESS", testPolicy1IngressJump),
				getFakeDeleteJumpCommandWithCode("AZURE-NPM-EGRESS", testPolicy1EgressJump, 1), // anything but 0 or 2
			},
		},
	}
	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			ioshim := common.NewMockIOShim(tt.calls)
			defer ioshim.VerifyCalls(t, tt.calls)
			pMgr := NewPolicyManager(ioshim, IPSetConfig)
			err := pMgr.AddPolicy(TestNetworkPolicies[0], nil)
			require.NoError(t, err)
			err = pMgr.RemovePolicy(TestNetworkPolicies[0].PolicyKey, nil)
			require.Error(t, err)
		})
	}
}

func TestUpdatingStaleChains(t *testing.T) {
	calls := GetAddPolicyTestCalls(TestNetworkPolicies[0])
	calls = append(calls, GetRemovePolicyTestCalls(TestNetworkPolicies[0])...)
	calls = append(calls, GetAddPolicyTestCalls(TestNetworkPolicies[1])...)
	calls = append(calls, GetRemovePolicyFailureTestCalls(TestNetworkPolicies[1])...)
	calls = append(calls, GetAddPolicyTestCalls(TestNetworkPolicies[2])...)
	calls = append(calls, GetRemovePolicyTestCalls(TestNetworkPolicies[2])...)
	calls = append(calls, GetAddPolicyFailureTestCalls(TestNetworkPolicies[0])...)
	calls = append(calls, GetAddPolicyTestCalls(TestNetworkPolicies[0])...)
	ioshim := common.NewMockIOShim(calls)
	defer ioshim.VerifyCalls(t, calls)
	pMgr := NewPolicyManager(ioshim, IPSetConfig)

	// add so we can remove. no stale chains to start
	require.NoError(t, pMgr.AddPolicy(TestNetworkPolicies[0], nil))
	assertStaleChainsContain(t, pMgr.staleChains)

	// successful removal, so mark the policy's chains as stale
	require.NoError(t, pMgr.RemovePolicy(TestNetworkPolicies[0].PolicyKey, nil))
	assertStaleChainsContain(t, pMgr.staleChains, testPolicy1IngressChain, testPolicy1EgressChain)

	// successful add, so keep the same stale chains
	require.NoError(t, pMgr.AddPolicy(TestNetworkPolicies[1], nil))
	assertStaleChainsContain(t, pMgr.staleChains, testPolicy1IngressChain, testPolicy1EgressChain)

	// failure to remove, so keep the same stale chains
	require.Error(t, pMgr.RemovePolicy(TestNetworkPolicies[1].PolicyKey, nil))
	assertStaleChainsContain(t, pMgr.staleChains, testPolicy1IngressChain, testPolicy1EgressChain)

	// successfully add a new policy. keep the same stale chains
	require.NoError(t, pMgr.AddPolicy(TestNetworkPolicies[2], nil))
	assertStaleChainsContain(t, pMgr.staleChains, testPolicy1IngressChain, testPolicy1EgressChain)

	// successful removal, so mark the policy's chains as stale
	require.NoError(t, pMgr.RemovePolicy(TestNetworkPolicies[2].PolicyKey, nil))
	assertStaleChainsContain(t, pMgr.staleChains, testPolicy1IngressChain, testPolicy1EgressChain, testPolicy3EgressChain)

	// failure to add, so keep the same stale chains the same
	require.Error(t, pMgr.AddPolicy(TestNetworkPolicies[0], nil))
	assertStaleChainsContain(t, pMgr.staleChains, testPolicy1IngressChain, testPolicy1EgressChain, testPolicy3EgressChain)

	// successful add, so remove the policy's chains from the stale chains
	require.NoError(t, pMgr.AddPolicy(TestNetworkPolicies[0], nil))
	assertStaleChainsContain(t, pMgr.staleChains, testPolicy3EgressChain)
}
