package policies

import (
	"fmt"
	"strings"
	"testing"

	"github.com/Azure/azure-container-networking/common"
	"github.com/Azure/azure-container-networking/npm/metrics"
	"github.com/Azure/azure-container-networking/npm/pkg/dataplane/ipsets"
	dptestutils "github.com/Azure/azure-container-networking/npm/pkg/dataplane/testutils"
	"github.com/Azure/azure-container-networking/npm/util"
	testutils "github.com/Azure/azure-container-networking/test/utils"
	"github.com/stretchr/testify/require"
)

// ACLs
var (
	ingressDeniedACL = &ACLPolicy{
		SrcList: []SetInfo{
			{
				ipsets.TestCIDRSet.Metadata,
				true,
				SrcMatch,
			},
			{
				ipsets.TestKeyPodSet.Metadata,
				false,
				DstMatch,
			},
		},
		Target:    Dropped,
		Direction: Ingress,
		DstPorts: Ports{
			222, 333,
		},
		Protocol: TCP,
	}
	ingressAllowedACL = &ACLPolicy{
		SrcList: []SetInfo{
			{
				ipsets.TestCIDRSet.Metadata,
				true,
				SrcMatch,
			},
		},
		Target:    Allowed,
		Direction: Ingress,
		Protocol:  UnspecifiedProtocol,
	}
	egressDeniedACL = &ACLPolicy{
		DstList: []SetInfo{
			{
				ipsets.TestCIDRSet.Metadata,
				true,
				DstMatch,
			},
		},
		Target:    Dropped,
		Direction: Egress,
		DstPorts:  Ports{144, 144},
		Protocol:  UDP,
	}
	egressAllowedACL = &ACLPolicy{
		DstList: []SetInfo{
			{
				ipsets.TestNamedportSet.Metadata,
				true,
				DstMatch,
			},
		},
		Target:    Allowed,
		Direction: Egress,
		Protocol:  UnspecifiedProtocol,
	}
)

// iptables rule constants for ACLs
const (
	ingressDropComment  = "DROP-FROM-cidr-test-cidr-set-AND-!podlabel-test-keyPod-set-ON-TCP-TO-PORT-222:333"
	ingressAllowComment = "ALLOW-FROM-cidr-test-cidr-set"
	egressDropComment   = "DROP-TO-cidr-test-cidr-set-ON-UDP-TO-PORT-144"
	egressAllowComment  = "ALLOW-ALL-TO-namedport:test-namedport-set"
)

// iptables rule variables for ACLs
var (
	ingressDropRule = fmt.Sprintf(
		"-j MARK --set-mark %s -p TCP --dport 222:333 -m set --match-set %s src -m set ! --match-set %s dst -m comment --comment %s",
		util.IptablesAzureIngressDropMarkHex,
		ipsets.TestCIDRSet.HashedName,
		ipsets.TestKeyPodSet.HashedName,
		ingressDropComment,
	)
	ingressAllowRule = fmt.Sprintf("-j AZURE-NPM-INGRESS-ALLOW-MARK -m set --match-set %s src -m comment --comment %s", ipsets.TestCIDRSet.HashedName, ingressAllowComment)
	egressDropRule   = fmt.Sprintf("-j MARK --set-mark %s -p UDP --dport 144 -m set --match-set %s dst -m comment --comment %s",
		util.IptablesAzureEgressDropMarkHex,
		ipsets.TestCIDRSet.HashedName,
		egressDropComment,
	)
	egressAllowRule = fmt.Sprintf("-j AZURE-NPM-ACCEPT -m set --match-set %s dst -m comment --comment %s", ipsets.TestNamedportSet.HashedName, egressAllowComment)
)

// NetworkPolicies
var (
	bothDirectionsNetPol = &NPMNetworkPolicy{
		Namespace:   "x",
		PolicyKey:   "x/test1",
		ACLPolicyID: "azure-acl-x-test1",
		PodSelectorIPSets: []*ipsets.TranslatedIPSet{
			{Metadata: ipsets.TestKeyPodSet.Metadata},
		},
		PodSelectorList: []SetInfo{
			{
				IPSet:     ipsets.TestKeyPodSet.Metadata,
				Included:  true,
				MatchType: EitherMatch,
			},
		},
		ACLs: []*ACLPolicy{
			ingressDeniedACL,
			ingressAllowedACL,
			egressDeniedACL,
			egressAllowedACL,
		},
	}
	ingressNetPol = &NPMNetworkPolicy{
		Namespace:   "y",
		PolicyKey:   "y/test2",
		ACLPolicyID: "azure-acl-y-test2",
		PodSelectorIPSets: []*ipsets.TranslatedIPSet{
			{Metadata: ipsets.TestKeyPodSet.Metadata},
			{Metadata: ipsets.TestNSSet.Metadata},
		},
		PodSelectorList: []SetInfo{
			{
				IPSet:     ipsets.TestKeyPodSet.Metadata,
				Included:  true,
				MatchType: EitherMatch,
			},
			{
				IPSet:     ipsets.TestNSSet.Metadata,
				Included:  true,
				MatchType: EitherMatch,
			},
		},
		ACLs: []*ACLPolicy{
			ingressDeniedACL,
		},
	}
	egressNetPol = &NPMNetworkPolicy{
		Namespace:   "z",
		PolicyKey:   "z/test3",
		ACLPolicyID: "azure-acl-z-test3",
		ACLs: []*ACLPolicy{
			egressAllowedACL,
		},
	}
)

// iptables rule constants for NetworkPolicies
const (
	bothDirectionsNetPolIngressJumpComment = "INGRESS-POLICY-x/test1-TO-podlabel-test-keyPod-set-IN-ns-x"
	bothDirectionsNetPolEgressJumpComment  = "EGRESS-POLICY-x/test1-FROM-podlabel-test-keyPod-set-IN-ns-x"
	ingressNetPolJumpComment               = "INGRESS-POLICY-y/test2-TO-podlabel-test-keyPod-set-AND-ns-test-ns-set-IN-ns-y"
	egressNetPolJumpComment                = "EGRESS-POLICY-z/test3-FROM-all-IN-ns-z"
)

// iptable rule variables for NetworkPolicies
var (
	bothDirectionsNetPolIngressChain = bothDirectionsNetPol.ingressChainName()
	bothDirectionsNetPolEgressChain  = bothDirectionsNetPol.egressChainName()
	ingressNetPolChain               = ingressNetPol.ingressChainName()
	egressNetPolChain                = egressNetPol.egressChainName()

	ingressEgressNetPolIngressJump = fmt.Sprintf(
		"-j %s -m set --match-set %s dst -m comment --comment %s",
		bothDirectionsNetPolIngressChain,
		ipsets.TestKeyPodSet.HashedName,
		bothDirectionsNetPolIngressJumpComment,
	)
	ingressEgressNetPolEgressJump = fmt.Sprintf(
		"-j %s -m set --match-set %s src -m comment --comment %s",
		bothDirectionsNetPolEgressChain,
		ipsets.TestKeyPodSet.HashedName,
		bothDirectionsNetPolEgressJumpComment,
	)
	ingressNetPolJump = fmt.Sprintf(
		"-j %s -m set --match-set %s dst -m set --match-set %s dst -m comment --comment %s",
		ingressNetPolChain,
		ipsets.TestKeyPodSet.HashedName,
		ipsets.TestNSSet.HashedName,
		ingressNetPolJumpComment,
	)
	egressNetPolJump = fmt.Sprintf("-j %s -m comment --comment %s", egressNetPolChain, egressNetPolJumpComment)
)

var allTestNetworkPolicies = []*NPMNetworkPolicy{bothDirectionsNetPol, ingressNetPol, egressNetPol}

func TestChainNames(t *testing.T) {
	expectedName := fmt.Sprintf("AZURE-NPM-INGRESS-%s", util.Hash(bothDirectionsNetPol.PolicyKey))
	require.Equal(t, expectedName, bothDirectionsNetPol.ingressChainName())
	expectedName = fmt.Sprintf("AZURE-NPM-EGRESS-%s", util.Hash(bothDirectionsNetPol.PolicyKey))
	require.Equal(t, expectedName, bothDirectionsNetPol.egressChainName())
}

// similar to TestAddPolicy in policymanager.go except an error occurs
func TestAddPolicyFailure(t *testing.T) {
	metrics.ReinitializeAll()
	testNetPol := testNetworkPolicy()
	calls := GetAddPolicyFailureTestCalls(testNetPol)
	ioshim := common.NewMockIOShim(calls)
	defer ioshim.VerifyCalls(t, calls)
	pMgr := NewPolicyManager(ioshim, ipsetConfig)

	require.Error(t, pMgr.AddPolicy(testNetPol, nil))
	_, ok := pMgr.GetPolicy(testNetPol.PolicyKey)
	require.False(t, ok)
	promVals{0, 1}.testPrometheusMetrics(t)
}

func TestCreatorForAddPolicies(t *testing.T) {
	calls := []testutils.TestCmd{fakeIPTablesRestoreCommand}
	ioshim := common.NewMockIOShim(calls)
	defer ioshim.VerifyCalls(t, calls)
	pMgr := NewPolicyManager(ioshim, ipsetConfig)

	// 1. test with activation
	policies := []*NPMNetworkPolicy{allTestNetworkPolicies[0]}
	creator := pMgr.creatorForNewNetworkPolicies(chainNames(policies), policies)
	actualLines := strings.Split(creator.ToString(), "\n")
	expectedLines := []string{
		"*filter",
		// all chains
		fmt.Sprintf(":%s - -", bothDirectionsNetPolIngressChain),
		fmt.Sprintf(":%s - -", bothDirectionsNetPolEgressChain),
		"-F AZURE-NPM",
		// activation rules for AZURE-NPM chain
		"-A AZURE-NPM -j AZURE-NPM-INGRESS",
		"-A AZURE-NPM -j AZURE-NPM-EGRESS",
		"-A AZURE-NPM -j AZURE-NPM-ACCEPT",
		// policy 1
		fmt.Sprintf("-A %s %s", bothDirectionsNetPolIngressChain, ingressDropRule),
		fmt.Sprintf("-A %s %s", bothDirectionsNetPolIngressChain, ingressAllowRule),
		fmt.Sprintf("-A %s %s", bothDirectionsNetPolEgressChain, egressDropRule),
		fmt.Sprintf("-A %s %s", bothDirectionsNetPolEgressChain, egressAllowRule),
		fmt.Sprintf("-I AZURE-NPM-INGRESS 1 %s", ingressEgressNetPolIngressJump),
		fmt.Sprintf("-I AZURE-NPM-EGRESS 1 %s", ingressEgressNetPolEgressJump),
		"COMMIT",
		"",
	}
	dptestutils.AssertEqualLines(t, expectedLines, actualLines)

	// 2. test without activation
	// add a policy to the cache so that we don't activate (the cache doesn't impact creatorForNewNetworkPolicies)
	require.NoError(t, pMgr.AddPolicy(allTestNetworkPolicies[0], nil))
	creator = pMgr.creatorForNewNetworkPolicies(chainNames(allTestNetworkPolicies), allTestNetworkPolicies)
	actualLines = strings.Split(creator.ToString(), "\n")
	expectedLines = []string{
		"*filter",
		// all chains
		fmt.Sprintf(":%s - -", bothDirectionsNetPolIngressChain),
		fmt.Sprintf(":%s - -", bothDirectionsNetPolEgressChain),
		fmt.Sprintf(":%s - -", ingressNetPolChain),
		fmt.Sprintf(":%s - -", egressNetPolChain),
		// policy 1
		fmt.Sprintf("-A %s %s", bothDirectionsNetPolIngressChain, ingressDropRule),
		fmt.Sprintf("-A %s %s", bothDirectionsNetPolIngressChain, ingressAllowRule),
		fmt.Sprintf("-A %s %s", bothDirectionsNetPolEgressChain, egressDropRule),
		fmt.Sprintf("-A %s %s", bothDirectionsNetPolEgressChain, egressAllowRule),
		fmt.Sprintf("-I AZURE-NPM-INGRESS 1 %s", ingressEgressNetPolIngressJump),
		fmt.Sprintf("-I AZURE-NPM-EGRESS 1 %s", ingressEgressNetPolEgressJump),
		// policy 2
		fmt.Sprintf("-A %s %s", ingressNetPolChain, ingressDropRule),
		fmt.Sprintf("-I AZURE-NPM-INGRESS 2 %s", ingressNetPolJump),
		// policy 3
		fmt.Sprintf("-A %s %s", egressNetPolChain, egressAllowRule),
		fmt.Sprintf("-I AZURE-NPM-EGRESS 2 %s", egressNetPolJump),
		"COMMIT",
		"",
	}
	dptestutils.AssertEqualLines(t, expectedLines, actualLines)
}

func TestCreatorForRemovePolicies(t *testing.T) {
	calls := []testutils.TestCmd{fakeIPTablesRestoreCommand}
	ioshim := common.NewMockIOShim(calls)
	defer ioshim.VerifyCalls(t, calls)
	pMgr := NewPolicyManager(ioshim, ipsetConfig)

	// 1. test without deactivation
	// hack: the cache is empty (and len(cache) != len(allTestNetworkPolicies)), so shouldDeactivate will be false
	creator := pMgr.creatorForRemovingPolicies(chainNames(allTestNetworkPolicies))
	actualLines := strings.Split(creator.ToString(), "\n")
	expectedLines := []string{
		"*filter",
		fmt.Sprintf("-F %s", bothDirectionsNetPolIngressChain),
		fmt.Sprintf("-F %s", bothDirectionsNetPolEgressChain),
		fmt.Sprintf("-F %s", ingressNetPolChain),
		fmt.Sprintf("-F %s", egressNetPolChain),
		"COMMIT",
		"",
	}
	dptestutils.AssertEqualLines(t, expectedLines, actualLines)

	// 2. test with deactivation
	// add to the cache so that we deactivate
	policy := TestNetworkPolicies[0]
	require.NoError(t, pMgr.AddPolicy(policy, nil))
	creator = pMgr.creatorForRemovingPolicies(chainNames([]*NPMNetworkPolicy{policy}))
	actualLines = strings.Split(creator.ToString(), "\n")
	expectedLines = []string{
		"*filter",
		"-F AZURE-NPM",
		fmt.Sprintf("-F %s", bothDirectionsNetPolIngressChain),
		fmt.Sprintf("-F %s", bothDirectionsNetPolEgressChain),
		"COMMIT",
		"",
	}
	dptestutils.AssertEqualLines(t, expectedLines, actualLines)
}

// similar to TestRemovePolicy in policymanager_test.go except an acceptable error occurs
func TestRemovePoliciesAcceptableError(t *testing.T) {
	metrics.ReinitializeAll()
	calls := []testutils.TestCmd{
		fakeIPTablesRestoreCommand,
		// ignore exit code 1
		getFakeDeleteJumpCommandWithCode("AZURE-NPM-INGRESS", ingressEgressNetPolIngressJump, 1),
		// ignore exit code 1
		getFakeDeleteJumpCommandWithCode("AZURE-NPM-EGRESS", ingressEgressNetPolEgressJump, 1),
		fakeIPTablesRestoreCommand,
	}
	ioshim := common.NewMockIOShim(calls)
	defer ioshim.VerifyCalls(t, calls)
	pMgr := NewPolicyManager(ioshim, ipsetConfig)
	require.NoError(t, pMgr.AddPolicy(bothDirectionsNetPol, epList))
	require.NoError(t, pMgr.RemovePolicy(bothDirectionsNetPol.PolicyKey))
	_, ok := pMgr.GetPolicy(bothDirectionsNetPol.PolicyKey)
	require.False(t, ok)
	promVals{0, 1}.testPrometheusMetrics(t)
}

// similar to TestRemovePolicy in policymanager_test.go except an error occurs
func TestRemovePoliciesError(t *testing.T) {
	tests := []struct {
		name  string
		calls []testutils.TestCmd
	}{
		{
			name: "error on restore",
			calls: []testutils.TestCmd{
				fakeIPTablesRestoreCommand,
				getFakeDeleteJumpCommand("AZURE-NPM-INGRESS", ingressEgressNetPolIngressJump),
				getFakeDeleteJumpCommand("AZURE-NPM-EGRESS", ingressEgressNetPolEgressJump),
				fakeIPTablesRestoreFailureCommand,
				fakeIPTablesRestoreFailureCommand,
			},
		},
		{
			name: "error on delete for ingress",
			calls: []testutils.TestCmd{
				fakeIPTablesRestoreCommand,
				getFakeDeleteJumpCommandWithCode("AZURE-NPM-INGRESS", ingressEgressNetPolIngressJump, 2), // anything but 0 or 1
			},
		},
		{
			name: "error on delete for egress",
			calls: []testutils.TestCmd{
				fakeIPTablesRestoreCommand,
				getFakeDeleteJumpCommand("AZURE-NPM-INGRESS", ingressEgressNetPolIngressJump),
				getFakeDeleteJumpCommandWithCode("AZURE-NPM-EGRESS", ingressEgressNetPolEgressJump, 2), // anything but 0 or 1
			},
		},
	}
	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			metrics.ReinitializeAll()
			ioshim := common.NewMockIOShim(tt.calls)
			defer ioshim.VerifyCalls(t, tt.calls)
			pMgr := NewPolicyManager(ioshim, ipsetConfig)
			err := pMgr.AddPolicy(bothDirectionsNetPol, nil)
			require.NoError(t, err)
			err = pMgr.RemovePolicy(bothDirectionsNetPol.PolicyKey)
			require.Error(t, err)

			promVals{6, 1}.testPrometheusMetrics(t)
		})
	}
}

func TestUpdatingStaleChains(t *testing.T) {
	calls := GetAddPolicyTestCalls(bothDirectionsNetPol)
	calls = append(calls, GetRemovePolicyTestCalls(bothDirectionsNetPol)...)
	calls = append(calls, GetAddPolicyTestCalls(ingressNetPol)...)
	calls = append(calls, GetRemovePolicyFailureTestCalls(ingressNetPol)...)
	calls = append(calls, GetAddPolicyTestCalls(egressNetPol)...)
	calls = append(calls, GetRemovePolicyTestCalls(egressNetPol)...)
	calls = append(calls, GetAddPolicyFailureTestCalls(bothDirectionsNetPol)...)
	calls = append(calls, GetAddPolicyTestCalls(bothDirectionsNetPol)...)
	ioshim := common.NewMockIOShim(calls)
	defer ioshim.VerifyCalls(t, calls)
	pMgr := NewPolicyManager(ioshim, ipsetConfig)

	// add so we can remove. no stale chains to start
	require.NoError(t, pMgr.AddPolicy(bothDirectionsNetPol, nil))
	assertStaleChainsContain(t, pMgr.staleChains)

	// successful removal, so mark the policy's chains as stale
	require.NoError(t, pMgr.RemovePolicy(bothDirectionsNetPol.PolicyKey))
	assertStaleChainsContain(t, pMgr.staleChains, bothDirectionsNetPolIngressChain, bothDirectionsNetPolEgressChain)

	// successful add, so keep the same stale chains
	require.NoError(t, pMgr.AddPolicy(ingressNetPol, nil))
	assertStaleChainsContain(t, pMgr.staleChains, bothDirectionsNetPolIngressChain, bothDirectionsNetPolEgressChain)

	// failure to remove, so keep the same stale chains
	require.Error(t, pMgr.RemovePolicy(ingressNetPol.PolicyKey))
	assertStaleChainsContain(t, pMgr.staleChains, bothDirectionsNetPolIngressChain, bothDirectionsNetPolEgressChain)

	// successfully add a new policy. keep the same stale chains
	require.NoError(t, pMgr.AddPolicy(egressNetPol, nil))
	assertStaleChainsContain(t, pMgr.staleChains, bothDirectionsNetPolIngressChain, bothDirectionsNetPolEgressChain)

	// successful removal, so mark the policy's chains as stale
	require.NoError(t, pMgr.RemovePolicy(egressNetPol.PolicyKey))
	assertStaleChainsContain(t, pMgr.staleChains, bothDirectionsNetPolIngressChain, bothDirectionsNetPolEgressChain, egressNetPolChain)

	// failure to add, so keep the same stale chains the same
	require.Error(t, pMgr.AddPolicy(bothDirectionsNetPol, nil))
	assertStaleChainsContain(t, pMgr.staleChains, bothDirectionsNetPolIngressChain, bothDirectionsNetPolEgressChain, egressNetPolChain)

	// successful add, so remove the policy's chains from the stale chains
	require.NoError(t, pMgr.AddPolicy(bothDirectionsNetPol, nil))
	assertStaleChainsContain(t, pMgr.staleChains, egressNetPolChain)
}
