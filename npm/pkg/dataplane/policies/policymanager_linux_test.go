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

var backgroundCfg = &PolicyManagerCfg{
	NodeIP:               "6.7.8.9",
	PolicyMode:           IPSetPolicyMode,
	PlaceAzureChainFirst: util.PlaceAzureChainAfterKubeServices,
	IPTablesInBackground: true,
}

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
func bothDirectionsNetPol() *NPMNetworkPolicy {
	return &NPMNetworkPolicy{
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
}

func ingressNetPol() *NPMNetworkPolicy {
	return &NPMNetworkPolicy{
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
}

func ingressNetPolWithUpdatedPodSelector() *NPMNetworkPolicy {
	p := ingressNetPol()
	p.PodSelectorList = append(p.PodSelectorList, SetInfo{
		IPSet:     ipsets.NewIPSetMetadata("test-abc", ipsets.Namespace),
		Included:  false,
		MatchType: EitherMatch,
	})
	return p
}

func egressNetPol() *NPMNetworkPolicy {
	return &NPMNetworkPolicy{
		Namespace:       "z",
		PolicyKey:       "z/test3",
		ACLPolicyID:     "azure-acl-z-test3",
		PodSelectorList: []SetInfo{},
		ACLs: []*ACLPolicy{
			egressAllowedACL,
		},
	}
}

// iptables rule constants for NetworkPolicies
const (
	bothDirectionsNetPolIngressJumpComment = "INGRESS-POLICY-x/test1-TO-podlabel-test-keyPod-set-IN-ns-x"
	bothDirectionsNetPolEgressJumpComment  = "EGRESS-POLICY-x/test1-FROM-podlabel-test-keyPod-set-IN-ns-x"
	ingressNetPolJumpComment               = "INGRESS-POLICY-y/test2-TO-podlabel-test-keyPod-set-AND-ns-test-ns-set-IN-ns-y"
	egressNetPolJumpComment                = "EGRESS-POLICY-z/test3-FROM-all-IN-ns-z"
)

// iptable rule variables for NetworkPolicies
var (
	bothDirectionsNetPolIngressChain = bothDirectionsNetPol().ingressChainName()
	bothDirectionsNetPolEgressChain  = bothDirectionsNetPol().egressChainName()
	ingressNetPolChain               = ingressNetPol().ingressChainName()
	egressNetPolChain                = egressNetPol().egressChainName()

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
	ingressNetPolJumpUpdatedPodSelector = fmt.Sprintf(
		"-j %s -m set --match-set %s dst -m set --match-set %s dst -m set ! --match-set %s dst -m comment --comment %s",
		ingressNetPolChain,
		ipsets.TestKeyPodSet.HashedName,
		ipsets.TestNSSet.HashedName,
		ipsets.NewIPSetMetadata("test-abc", ipsets.Namespace).GetHashedName(),
		"INGRESS-POLICY-y/test2-TO-podlabel-test-keyPod-set-AND-ns-test-ns-set-AND-!ns-test-abc-IN-ns-y",
	)
	egressNetPolJump = fmt.Sprintf("-j %s -m comment --comment %s", egressNetPolChain, egressNetPolJumpComment)
)

var allTestNetworkPolicies = []*NPMNetworkPolicy{bothDirectionsNetPol(), ingressNetPol(), egressNetPol()}

func TestChainNames(t *testing.T) {
	expectedName := fmt.Sprintf("AZURE-NPM-INGRESS-%s", util.Hash(bothDirectionsNetPol().PolicyKey))
	require.Equal(t, expectedName, bothDirectionsNetPol().ingressChainName())
	expectedName = fmt.Sprintf("AZURE-NPM-EGRESS-%s", util.Hash(bothDirectionsNetPol().PolicyKey))
	require.Equal(t, expectedName, bothDirectionsNetPol().egressChainName())
}

// similar to TestAddPolicy in policymanager.go except an error occurs
func TestAddPolicyFailure(t *testing.T) {
	metrics.ReinitializeAll()
	testNetPol := testNetworkPolicy()
	calls := GetAddPolicyFailureTestCalls(testNetPol)
	ioshim := common.NewMockIOShim(calls)
	defer ioshim.VerifyCalls(t, calls)
	pMgr := NewPolicyManager(ioshim, defaultCfg)

	require.Error(t, pMgr.AddPolicy(testNetPol, nil))
	_, ok := pMgr.GetPolicy(testNetPol.PolicyKey)
	require.False(t, ok)
	promVals{0, 1}.testPrometheusMetrics(t)
}

func TestCreatorForAddPolicies(t *testing.T) {
	calls := []testutils.TestCmd{fakeIPTablesRestoreCommand}
	ioshim := common.NewMockIOShim(calls)
	defer ioshim.VerifyCalls(t, calls)
	pMgr := NewPolicyManager(ioshim, defaultCfg)

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
	pMgr := NewPolicyManager(ioshim, defaultCfg)

	// 1. test without deactivation (i.e. flushing azure chain when removing the last policy)
	// hack: the cache is empty (and len(cache) != len(allTestNetworkPolicies)), so shouldDeactivate will be false
	creator := pMgr.creatorForRemovingPolicies(chainNames(allTestNetworkPolicies), false)
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

	// 2. test with deactivation (i.e. flushing azure chain when removing the last policy)
	// add to the cache so that we deactivate
	policy := TestNetworkPolicies[0]
	require.NoError(t, pMgr.AddPolicy(policy, nil))
	creator = pMgr.creatorForRemovingPolicies(chainNames([]*NPMNetworkPolicy{policy}), true)
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
	pMgr := NewPolicyManager(ioshim, defaultCfg)
	require.NoError(t, pMgr.AddPolicy(bothDirectionsNetPol(), epList))
	require.NoError(t, pMgr.RemovePolicy(bothDirectionsNetPol().PolicyKey))
	_, ok := pMgr.GetPolicy(bothDirectionsNetPol().PolicyKey)
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
			pMgr := NewPolicyManager(ioshim, defaultCfg)
			err := pMgr.AddPolicy(bothDirectionsNetPol(), nil)
			require.NoError(t, err)
			err = pMgr.RemovePolicy(bothDirectionsNetPol().PolicyKey)
			require.Error(t, err)

			promVals{6, 1}.testPrometheusMetrics(t)
		})
	}
}

func TestUpdatingStaleChains(t *testing.T) {
	calls := GetAddPolicyTestCalls(bothDirectionsNetPol())
	calls = append(calls, GetRemovePolicyTestCalls(bothDirectionsNetPol())...)
	calls = append(calls, GetAddPolicyTestCalls(ingressNetPol())...)
	calls = append(calls, GetRemovePolicyFailureTestCalls(ingressNetPol())...)
	calls = append(calls, GetAddPolicyTestCalls(egressNetPol())...)
	calls = append(calls, GetRemovePolicyTestCalls(egressNetPol())...)
	calls = append(calls, GetAddPolicyFailureTestCalls(bothDirectionsNetPol())...)
	calls = append(calls, GetAddPolicyTestCalls(bothDirectionsNetPol())...)
	ioshim := common.NewMockIOShim(calls)
	defer ioshim.VerifyCalls(t, calls)
	pMgr := NewPolicyManager(ioshim, defaultCfg)

	// add so we can remove. no stale chains to start
	require.NoError(t, pMgr.AddPolicy(bothDirectionsNetPol(), nil))
	assertStaleChainsContain(t, pMgr.staleChains)

	// successful removal, so mark the policy's chains as stale
	require.NoError(t, pMgr.RemovePolicy(bothDirectionsNetPol().PolicyKey))
	assertStaleChainsContain(t, pMgr.staleChains, bothDirectionsNetPolIngressChain, bothDirectionsNetPolEgressChain)

	// successful add, so keep the same stale chains
	require.NoError(t, pMgr.AddPolicy(ingressNetPol(), nil))
	assertStaleChainsContain(t, pMgr.staleChains, bothDirectionsNetPolIngressChain, bothDirectionsNetPolEgressChain)

	// failure to remove, so keep the same stale chains
	require.Error(t, pMgr.RemovePolicy(ingressNetPol().PolicyKey))
	assertStaleChainsContain(t, pMgr.staleChains, bothDirectionsNetPolIngressChain, bothDirectionsNetPolEgressChain)

	// successfully add a new policy. keep the same stale chains
	require.NoError(t, pMgr.AddPolicy(egressNetPol(), nil))
	assertStaleChainsContain(t, pMgr.staleChains, bothDirectionsNetPolIngressChain, bothDirectionsNetPolEgressChain)

	// successful removal, so mark the policy's chains as stale
	require.NoError(t, pMgr.RemovePolicy(egressNetPol().PolicyKey))
	assertStaleChainsContain(t, pMgr.staleChains, bothDirectionsNetPolIngressChain, bothDirectionsNetPolEgressChain, egressNetPolChain)

	// failure to add, so keep the same stale chains the same
	require.Error(t, pMgr.AddPolicy(bothDirectionsNetPol(), nil))
	assertStaleChainsContain(t, pMgr.staleChains, bothDirectionsNetPolIngressChain, bothDirectionsNetPolEgressChain, egressNetPolChain)

	// successful add, so remove the policy's chains from the stale chains
	require.NoError(t, pMgr.AddPolicy(bothDirectionsNetPol(), nil))
	assertStaleChainsContain(t, pMgr.staleChains, egressNetPolChain)
}

func TestBackgroundQueue(t *testing.T) {
	type args struct {
		policyMap map[string]*NPMNetworkPolicy
		queue     map[string][]*event
	}

	tests := []struct {
		name             string
		args             *args
		toRemove         []*NPMNetworkPolicy
		toAdd            map[string]*NPMNetworkPolicy
		finalQueueIsSame bool
		// use if finalQueueIsSame == false
		finalQueue map[string][]*event
	}{
		{
			// can have multiple adds per policy
			name: "add two",
			args: &args{
				queue: map[string][]*event{
					egressNetPol().PolicyKey: {
						{op: add},
					},
					ingressNetPol().PolicyKey: {
						{op: add},
						{op: add},
					},
				},
				policyMap: map[string]*NPMNetworkPolicy{
					egressNetPol().PolicyKey:  egressNetPol(),
					ingressNetPol().PolicyKey: ingressNetPol(),
				},
			},
			toRemove: []*NPMNetworkPolicy{},
			toAdd: map[string]*NPMNetworkPolicy{
				egressNetPol().PolicyKey:  egressNetPol(),
				ingressNetPol().PolicyKey: ingressNetPol(),
			},
			finalQueueIsSame: true,
		},
		{
			name: "ignore add if not in policyMap",
			args: &args{
				queue: map[string][]*event{
					ingressNetPol().PolicyKey: {
						{op: add},
						{op: add},
					},
				},
				policyMap: map[string]*NPMNetworkPolicy{
					egressNetPol().PolicyKey: egressNetPol(),
				},
			},
			toRemove:   []*NPMNetworkPolicy{},
			toAdd:      map[string]*NPMNetworkPolicy{},
			finalQueue: map[string][]*event{},
		},
		{
			name: "ignore remove not in kernel",
			args: &args{
				queue: map[string][]*event{
					egressNetPol().PolicyKey: {
						{
							op: remove,
							deletedState: &deletedState{
								namespace:       egressNetPol().Namespace,
								direction:       Egress,
								podSelectorList: egressNetPol().PodSelectorList,
								wasInKernel:     false,
							},
						},
					},
					ingressNetPol().PolicyKey: {
						{op: add},
						{
							op: remove,
							deletedState: &deletedState{
								namespace:       ingressNetPol().Namespace,
								direction:       Egress,
								podSelectorList: ingressNetPol().PodSelectorList,
								wasInKernel:     false,
							},
						},
					},
					bothDirectionsNetPol().PolicyKey: {
						{
							op: remove,
							deletedState: &deletedState{
								namespace:       bothDirectionsNetPol().Namespace,
								direction:       Both,
								podSelectorList: bothDirectionsNetPol().PodSelectorList,
								wasInKernel:     false,
							},
						},
					},
				},
				policyMap: map[string]*NPMNetworkPolicy{},
			},
			toRemove:   []*NPMNetworkPolicy{},
			toAdd:      map[string]*NPMNetworkPolicy{},
			finalQueue: map[string][]*event{},
		},
		{
			// can have multiple removes per policy
			name: "remove all direction types",
			args: &args{
				queue: map[string][]*event{
					egressNetPol().PolicyKey: {
						{
							op: remove,
							deletedState: &deletedState{
								namespace:       egressNetPol().Namespace,
								direction:       Egress,
								podSelectorList: egressNetPol().PodSelectorList,
								wasInKernel:     true,
							},
						},
					},
					ingressNetPol().PolicyKey: {
						{op: add},
						{
							op: remove,
							deletedState: &deletedState{
								namespace:       ingressNetPol().Namespace,
								direction:       Ingress,
								podSelectorList: ingressNetPol().PodSelectorList,
								wasInKernel:     false,
							},
						},
						// unexpected to have this sequence, but the second remove should be noticed since it is in kernel
						{
							op: remove,
							deletedState: &deletedState{
								namespace:       ingressNetPol().Namespace,
								direction:       Ingress,
								podSelectorList: ingressNetPol().PodSelectorList,
								wasInKernel:     true,
							},
						},
					},
					bothDirectionsNetPol().PolicyKey: {
						{
							op: remove,
							deletedState: &deletedState{
								namespace:       bothDirectionsNetPol().Namespace,
								direction:       Both,
								podSelectorList: bothDirectionsNetPol().PodSelectorList,
								wasInKernel:     true,
							},
						},
						// second one should not be noticed
						{
							op: remove,
							deletedState: &deletedState{
								namespace:       bothDirectionsNetPol().Namespace,
								direction:       Both,
								podSelectorList: bothDirectionsNetPol().PodSelectorList,
								wasInKernel:     false,
							},
						},
					},
				},
				policyMap: map[string]*NPMNetworkPolicy{},
			},
			toRemove: []*NPMNetworkPolicy{
				{
					Namespace:       egressNetPol().Namespace,
					PolicyKey:       egressNetPol().PolicyKey,
					PodSelectorList: egressNetPol().PodSelectorList,
					ACLs: []*ACLPolicy{
						{
							Direction: Egress,
						},
					},
				},
				{
					Namespace:       ingressNetPol().Namespace,
					PolicyKey:       ingressNetPol().PolicyKey,
					PodSelectorList: ingressNetPol().PodSelectorList,
					ACLs: []*ACLPolicy{
						{
							Direction: Ingress,
						},
					},
				},
				{
					Namespace:       bothDirectionsNetPol().Namespace,
					PolicyKey:       bothDirectionsNetPol().PolicyKey,
					PodSelectorList: bothDirectionsNetPol().PodSelectorList,
					ACLs: []*ACLPolicy{
						{
							Direction: Both,
						},
					},
				},
			},
			toAdd:            map[string]*NPMNetworkPolicy{},
			finalQueueIsSame: true,
		},
		{
			name: "update",
			args: &args{
				queue: map[string][]*event{
					egressNetPol().PolicyKey: {
						{
							op: remove,
							deletedState: &deletedState{
								namespace:       egressNetPol().Namespace,
								direction:       Egress,
								podSelectorList: egressNetPol().PodSelectorList,
								wasInKernel:     true,
							},
						},
						{op: add},
					},
				},
				policyMap: map[string]*NPMNetworkPolicy{
					egressNetPol().PolicyKey: egressNetPol(),
				},
			},
			toRemove: []*NPMNetworkPolicy{
				{
					Namespace:       egressNetPol().Namespace,
					PolicyKey:       egressNetPol().PolicyKey,
					PodSelectorList: egressNetPol().PodSelectorList,
					ACLs: []*ACLPolicy{
						{
							Direction: Egress,
						},
					},
				},
			},
			toAdd: map[string]*NPMNetworkPolicy{
				egressNetPol().PolicyKey: egressNetPol(),
			},
			finalQueueIsSame: true,
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			var calls []testutils.TestCmd
			ioshim := common.NewMockIOShim(calls)
			defer ioshim.VerifyCalls(t, calls)
			pMgr := NewPolicyManager(ioshim, backgroundCfg)

			pMgr.policyMap.cache = tt.args.policyMap
			copiedQueue := make(map[string][]*event, len(tt.args.queue))
			for k, v := range tt.args.queue {
				copiedQueue[k] = v
			}
			pMgr.policyMap.dirtyCache.queue = copiedQueue

			toRemove, toAdd := pMgr.dirtyNetPols()

			// assert to remove
			for _, v := range tt.toRemove {
				require.Contains(t, toRemove, v, "expected: %+v. actual: %+v", tt.toRemove, toRemove)
			}
			require.Len(t, toRemove, len(tt.toRemove), "expected: %+v. actual: %+v", tt.toRemove, toRemove)

			// assert to add and final queue
			require.Equal(t, tt.toAdd, toAdd)
			if tt.finalQueueIsSame {
				require.Equal(t, tt.args.queue, pMgr.policyMap.dirtyCache.queue)
			} else {
				require.Equal(t, tt.finalQueue, pMgr.policyMap.dirtyCache.queue)
			}
		})
	}
}

func TestBackgroundE2E(t *testing.T) {
	metrics.ReinitializeAll()
	calls := []testutils.TestCmd{
		// add two NetPols, but never add one NetPol because it was removed early
		fakeIPTablesRestoreCommand,
		// delete NetPol
		getFakeDeleteJumpCommand("AZURE-NPM-EGRESS", egressNetPolJump),
		fakeIPTablesRestoreCommand,
		// update NetPol to have new PodSelector
		getFakeDeleteJumpCommand("AZURE-NPM-INGRESS", ingressNetPolJump),
		fakeIPTablesRestoreCommand,
		fakeIPTablesRestoreCommand,
		// be sure to delete NetPol even if recent delete is not inKernel
		getFakeDeleteJumpCommand("AZURE-NPM-INGRESS", ingressNetPolJumpUpdatedPodSelector),
		fakeIPTablesRestoreCommand,
	}
	ioshim := common.NewMockIOShim(calls)
	defer ioshim.VerifyCalls(t, calls)
	pMgr := NewPolicyManager(ioshim, backgroundCfg)
	require.Equal(t, pMgr.policyMap.policiesInKernel, 0)

	// add two NetPols, but never add one NetPol because it was removed early
	require.NoError(t, pMgr.AddPolicy(bothDirectionsNetPol(), nil))
	promVals{6, 1}.testPrometheusMetrics(t)
	require.Equal(t, map[string][]*event{
		bothDirectionsNetPol().PolicyKey: {
			{op: add},
		},
	}, pMgr.policyMap.dirtyCache.queue)
	require.Equal(t, map[string]*NPMNetworkPolicy{
		bothDirectionsNetPol().PolicyKey: bothDirectionsNetPol(),
	}, pMgr.policyMap.cache)
	require.Equal(t, pMgr.policyMap.policiesInKernel, 0)

	require.NoError(t, pMgr.AddPolicy(ingressNetPol(), nil))
	promVals{8, 2}.testPrometheusMetrics(t)
	require.Equal(t, map[string][]*event{
		bothDirectionsNetPol().PolicyKey: {
			{op: add},
		},
		ingressNetPol().PolicyKey: {
			{op: add},
		},
	}, pMgr.policyMap.dirtyCache.queue)
	require.Equal(t, map[string]*NPMNetworkPolicy{
		bothDirectionsNetPol().PolicyKey: bothDirectionsNetPol(),
		ingressNetPol().PolicyKey:        ingressNetPol(),
	}, pMgr.policyMap.cache)
	require.Equal(t, pMgr.policyMap.policiesInKernel, 0)

	require.NoError(t, pMgr.AddPolicy(egressNetPol(), nil))
	promVals{10, 3}.testPrometheusMetrics(t)
	require.Equal(t, map[string][]*event{
		bothDirectionsNetPol().PolicyKey: {
			{op: add},
		},
		ingressNetPol().PolicyKey: {
			{op: add},
		},
		egressNetPol().PolicyKey: {
			{op: add},
		},
	}, pMgr.policyMap.dirtyCache.queue)
	require.Equal(t, map[string]*NPMNetworkPolicy{
		bothDirectionsNetPol().PolicyKey: bothDirectionsNetPol(),
		ingressNetPol().PolicyKey:        ingressNetPol(),
		egressNetPol().PolicyKey:         egressNetPol(),
	}, pMgr.policyMap.cache)
	require.Equal(t, pMgr.policyMap.policiesInKernel, 0)

	require.NoError(t, pMgr.RemovePolicy(bothDirectionsNetPol().PolicyKey))
	promVals{4, 3}.testPrometheusMetrics(t)
	require.Equal(t, map[string][]*event{
		bothDirectionsNetPol().PolicyKey: {
			{op: add},
			{
				op: remove,
				deletedState: &deletedState{
					namespace:       bothDirectionsNetPol().Namespace,
					direction:       Both,
					podSelectorList: bothDirectionsNetPol().PodSelectorList,
					wasInKernel:     false,
				},
			},
		},
		ingressNetPol().PolicyKey: {
			{op: add},
		},
		egressNetPol().PolicyKey: {
			{op: add},
		},
	}, pMgr.policyMap.dirtyCache.queue)
	require.Equal(t, map[string]*NPMNetworkPolicy{
		ingressNetPol().PolicyKey: ingressNetPol(),
		egressNetPol().PolicyKey:  egressNetPol(),
	}, pMgr.policyMap.cache)
	require.Equal(t, pMgr.policyMap.policiesInKernel, 0)

	require.NoError(t, pMgr.RemovePolicy(ingressNetPol().PolicyKey))
	promVals{2, 3}.testPrometheusMetrics(t)
	require.Equal(t, map[string][]*event{
		bothDirectionsNetPol().PolicyKey: {
			{op: add},
			{
				op: remove,
				deletedState: &deletedState{
					namespace:       bothDirectionsNetPol().Namespace,
					direction:       Both,
					podSelectorList: bothDirectionsNetPol().PodSelectorList,
					wasInKernel:     false,
				},
			},
		},
		ingressNetPol().PolicyKey: {
			{op: add},
			{
				op: remove,
				deletedState: &deletedState{
					namespace:       ingressNetPol().Namespace,
					direction:       Ingress,
					podSelectorList: ingressNetPol().PodSelectorList,
					wasInKernel:     false,
				},
			},
		},
		egressNetPol().PolicyKey: {
			{op: add},
		},
	}, pMgr.policyMap.dirtyCache.queue)
	require.Equal(t, map[string]*NPMNetworkPolicy{
		egressNetPol().PolicyKey: egressNetPol(),
	}, pMgr.policyMap.cache)
	require.Equal(t, pMgr.policyMap.policiesInKernel, 0)

	require.NoError(t, pMgr.AddPolicy(ingressNetPol(), nil))
	promVals{4, 4}.testPrometheusMetrics(t)
	require.Equal(t, map[string][]*event{
		bothDirectionsNetPol().PolicyKey: {
			{op: add},
			{
				op: remove,
				deletedState: &deletedState{
					namespace:       bothDirectionsNetPol().Namespace,
					direction:       Both,
					podSelectorList: bothDirectionsNetPol().PodSelectorList,
					wasInKernel:     false,
				},
			},
		},
		ingressNetPol().PolicyKey: {
			{op: add},
			{
				op: remove,
				deletedState: &deletedState{
					namespace:       ingressNetPol().Namespace,
					direction:       Ingress,
					podSelectorList: ingressNetPol().PodSelectorList,
					wasInKernel:     false,
				},
			},
			{op: add},
		},
		egressNetPol().PolicyKey: {
			{op: add},
		},
	}, pMgr.policyMap.dirtyCache.queue)
	require.Equal(t, map[string]*NPMNetworkPolicy{
		ingressNetPol().PolicyKey: ingressNetPol(),
		egressNetPol().PolicyKey:  egressNetPol(),
	}, pMgr.policyMap.cache)
	require.Equal(t, pMgr.policyMap.policiesInKernel, 0)

	require.Nil(t, pMgr.ReconcileDirtyNetPols())
	promVals{4, 4}.testPrometheusMetrics(t)
	require.Equal(t, map[string][]*event{}, pMgr.policyMap.dirtyCache.queue)
	require.Equal(t, map[string]*NPMNetworkPolicy{
		ingressNetPol().PolicyKey: inKernel(ingressNetPol()),
		egressNetPol().PolicyKey:  inKernel(egressNetPol()),
	}, pMgr.policyMap.cache)
	require.Equal(t, pMgr.policyMap.policiesInKernel, 2)

	// delete NetPol
	require.NoError(t, pMgr.RemovePolicy(egressNetPol().PolicyKey))
	promVals{2, 4}.testPrometheusMetrics(t)
	require.Equal(t, map[string][]*event{
		egressNetPol().PolicyKey: {
			{
				op: remove,
				deletedState: &deletedState{
					namespace:       egressNetPol().Namespace,
					direction:       Egress,
					podSelectorList: egressNetPol().PodSelectorList,
					wasInKernel:     true,
				},
			},
		},
	}, pMgr.policyMap.dirtyCache.queue)
	require.Equal(t, map[string]*NPMNetworkPolicy{
		ingressNetPol().PolicyKey: inKernel(ingressNetPol()),
	}, pMgr.policyMap.cache)
	require.Equal(t, pMgr.policyMap.policiesInKernel, 2)

	require.NoError(t, pMgr.ReconcileDirtyNetPols())
	promVals{2, 4}.testPrometheusMetrics(t)
	require.Equal(t, map[string][]*event{}, pMgr.policyMap.dirtyCache.queue)
	require.Equal(t, map[string]*NPMNetworkPolicy{
		ingressNetPol().PolicyKey: inKernel(ingressNetPol()),
	}, pMgr.policyMap.cache)
	require.Equal(t, pMgr.policyMap.policiesInKernel, 1)

	// update NetPol to have new PodSelector
	require.NoError(t, pMgr.RemovePolicy(ingressNetPol().PolicyKey))
	promVals{0, 4}.testPrometheusMetrics(t)
	require.Equal(t, map[string][]*event{
		ingressNetPol().PolicyKey: {
			{
				op: remove,
				deletedState: &deletedState{
					namespace:       ingressNetPol().Namespace,
					direction:       Ingress,
					podSelectorList: ingressNetPol().PodSelectorList,
					wasInKernel:     true,
				},
			},
		},
	}, pMgr.policyMap.dirtyCache.queue)
	require.Equal(t, map[string]*NPMNetworkPolicy{}, pMgr.policyMap.cache)
	require.Equal(t, pMgr.policyMap.policiesInKernel, 1)

	ingressNetPolWithUpdatedPodSelector()
	require.NoError(t, pMgr.AddPolicy(ingressNetPolWithUpdatedPodSelector(), nil))
	promVals{2, 5}.testPrometheusMetrics(t)
	require.Equal(t, map[string][]*event{
		ingressNetPol().PolicyKey: {
			{
				op: remove,
				deletedState: &deletedState{
					namespace:       ingressNetPol().Namespace,
					direction:       Ingress,
					podSelectorList: ingressNetPol().PodSelectorList,
					wasInKernel:     true,
				},
			},
			{op: add},
		},
	}, pMgr.policyMap.dirtyCache.queue)
	require.Equal(t, map[string]*NPMNetworkPolicy{
		ingressNetPol().PolicyKey: ingressNetPolWithUpdatedPodSelector(),
	}, pMgr.policyMap.cache)

	require.NoError(t, pMgr.ReconcileDirtyNetPols())
	promVals{2, 5}.testPrometheusMetrics(t)
	require.Equal(t, map[string][]*event{}, pMgr.policyMap.dirtyCache.queue)
	require.Equal(t, map[string]*NPMNetworkPolicy{
		ingressNetPol().PolicyKey: inKernel(ingressNetPolWithUpdatedPodSelector()),
	}, pMgr.policyMap.cache)
	require.Equal(t, pMgr.policyMap.policiesInKernel, 1)

	// be sure to delete NetPol even if recent delete is not inKernel
	require.NoError(t, pMgr.RemovePolicy(ingressNetPol().PolicyKey))
	promVals{0, 5}.testPrometheusMetrics(t)
	require.Equal(t, map[string][]*event{
		ingressNetPol().PolicyKey: {
			{
				op: remove,
				deletedState: &deletedState{
					namespace:       ingressNetPol().Namespace,
					direction:       Ingress,
					podSelectorList: ingressNetPolWithUpdatedPodSelector().PodSelectorList,
					wasInKernel:     true,
				},
			},
		},
	}, pMgr.policyMap.dirtyCache.queue)
	require.Equal(t, map[string]*NPMNetworkPolicy{}, pMgr.policyMap.cache)
	require.Equal(t, pMgr.policyMap.policiesInKernel, 1)

	require.NoError(t, pMgr.AddPolicy(ingressNetPol(), nil))
	promVals{2, 6}.testPrometheusMetrics(t)
	require.Equal(t, map[string][]*event{
		ingressNetPol().PolicyKey: {
			{
				op: remove,
				deletedState: &deletedState{
					namespace:       ingressNetPol().Namespace,
					direction:       Ingress,
					podSelectorList: ingressNetPolWithUpdatedPodSelector().PodSelectorList,
					wasInKernel:     true,
				},
			},
			{op: add},
		},
	}, pMgr.policyMap.dirtyCache.queue)
	require.Equal(t, map[string]*NPMNetworkPolicy{
		ingressNetPol().PolicyKey: ingressNetPol(),
	}, pMgr.policyMap.cache)
	require.Equal(t, pMgr.policyMap.policiesInKernel, 1)

	require.NoError(t, pMgr.RemovePolicy(ingressNetPol().PolicyKey))
	promVals{0, 6}.testPrometheusMetrics(t)
	require.Equal(t, map[string][]*event{
		ingressNetPol().PolicyKey: {
			{
				op: remove,
				deletedState: &deletedState{
					namespace:       ingressNetPol().Namespace,
					direction:       Ingress,
					podSelectorList: ingressNetPolWithUpdatedPodSelector().PodSelectorList,
					wasInKernel:     true,
				},
			},
			{op: add},
			{
				op: remove,
				deletedState: &deletedState{
					namespace:       ingressNetPol().Namespace,
					direction:       Ingress,
					podSelectorList: ingressNetPol().PodSelectorList,
					wasInKernel:     false,
				},
			},
		},
	}, pMgr.policyMap.dirtyCache.queue)
	require.Equal(t, map[string]*NPMNetworkPolicy{}, pMgr.policyMap.cache)
	require.Equal(t, pMgr.policyMap.policiesInKernel, 1)

	require.NoError(t, pMgr.ReconcileDirtyNetPols())
	promVals{0, 6}.testPrometheusMetrics(t)
	require.Equal(t, map[string][]*event{}, pMgr.policyMap.dirtyCache.queue)
	require.Equal(t, map[string]*NPMNetworkPolicy{}, pMgr.policyMap.cache)
	require.Equal(t, pMgr.policyMap.policiesInKernel, 0)
}

func inKernel(p *NPMNetworkPolicy) *NPMNetworkPolicy {
	p.inLinuxKernel = true
	return p
}

func TestBackgroundFailures(t *testing.T) {
	metrics.ReinitializeAll()
	calls := []testutils.TestCmd{
		// add both NetPols
		fakeIPTablesRestoreCommand,
		// update both NetPols
		// the first delete jump call fails for unknown reason
		getFakeDeleteJumpCommandWithCode("AZURE-NPM-EGRESS", egressNetPolJump, 9),
		// remove policies one by one
		// fail on first
		getFakeDeleteJumpCommandWithCode("AZURE-NPM-EGRESS", egressNetPolJump, 9),
		// success on second, even with error code 1
		getFakeDeleteJumpCommandWithCode("AZURE-NPM-INGRESS", ingressNetPolJump, 1),
		fakeIPTablesRestoreCommand,
		// add policy fails at first (includes a retry)
		fakeIPTablesRestoreFailureCommand,
		fakeIPTablesRestoreFailureCommand,
		// add policy fails again for the only policy (includes a retry)
		fakeIPTablesRestoreFailureCommand,
		fakeIPTablesRestoreFailureCommand,
	}
	ioshim := common.NewMockIOShim(calls)
	defer ioshim.VerifyCalls(t, calls)
	pMgr := NewPolicyManager(ioshim, backgroundCfg)
	require.Equal(t, pMgr.policyMap.policiesInKernel, 0)
	promVals{0, 0}.testPrometheusMetrics(t)

	require.Nil(t, pMgr.AddPolicy(egressNetPol(), nil))
	require.Nil(t, pMgr.AddPolicy(ingressNetPol(), nil))

	toAdd := map[string]*NPMNetworkPolicy{
		egressNetPol().PolicyKey:  egressNetPol(),
		ingressNetPol().PolicyKey: ingressNetPol(),
	}
	originalQueue := map[string][]*event{
		egressNetPol().PolicyKey: {
			{op: add},
		},
		ingressNetPol().PolicyKey: {
			{op: add},
		},
	}

	require.Equal(t, originalQueue, pMgr.policyMap.dirtyCache.queue)
	require.Equal(t, map[string]*NPMNetworkPolicy{
		egressNetPol().PolicyKey:  egressNetPol(),
		ingressNetPol().PolicyKey: ingressNetPol(),
	}, pMgr.policyMap.cache)
	require.Equal(t, pMgr.policyMap.policiesInKernel, 0)
	promVals{4, 2}.testPrometheusMetrics(t)

	toRemoveActual, toAddActual := pMgr.dirtyNetPols()
	require.Len(t, toRemoveActual, 0)
	require.Equal(t, toAdd, toAddActual)
	require.Equal(t, originalQueue, pMgr.policyMap.dirtyCache.queue)

	require.NoError(t, pMgr.reconcileDirtyNetPolsInKernel(toRemoveActual, toAddActual))
	require.Equal(t, map[string][]*event{}, pMgr.policyMap.dirtyCache.queue)
	require.Equal(t, map[string]*NPMNetworkPolicy{
		egressNetPol().PolicyKey:  inKernel(egressNetPol()),
		ingressNetPol().PolicyKey: inKernel(ingressNetPol()),
	}, pMgr.policyMap.cache)
	require.Equal(t, pMgr.policyMap.policiesInKernel, 2)
	promVals{4, 2}.testPrometheusMetrics(t)

	// update NetPols
	require.Nil(t, pMgr.RemovePolicy(egressNetPol().PolicyKey))
	require.Nil(t, pMgr.AddPolicy(egressNetPol(), nil))
	require.Nil(t, pMgr.RemovePolicy(ingressNetPol().PolicyKey))
	require.Nil(t, pMgr.AddPolicy(ingressNetPol(), nil))

	// toAdd is the same
	toRemove := []*NPMNetworkPolicy{
		{
			Namespace:       egressNetPol().Namespace,
			PolicyKey:       egressNetPol().PolicyKey,
			PodSelectorList: egressNetPol().PodSelectorList,
			ACLs: []*ACLPolicy{
				{
					Direction: Egress,
				},
			},
		},
		{
			Namespace:       ingressNetPol().Namespace,
			PolicyKey:       ingressNetPol().PolicyKey,
			PodSelectorList: ingressNetPol().PodSelectorList,
			ACLs: []*ACLPolicy{
				{
					Direction: Ingress,
				},
			},
		},
	}
	updateQueue := map[string][]*event{
		egressNetPol().PolicyKey: {
			{
				op: remove,
				deletedState: &deletedState{
					namespace:       egressNetPol().Namespace,
					direction:       Egress,
					podSelectorList: egressNetPol().PodSelectorList,
					wasInKernel:     true,
				},
			},
			{op: add},
		},
		ingressNetPol().PolicyKey: {
			{
				op: remove,
				deletedState: &deletedState{
					namespace:       ingressNetPol().Namespace,
					direction:       Ingress,
					podSelectorList: ingressNetPol().PodSelectorList,
					wasInKernel:     true,
				},
			},
			{op: add},
		},
	}

	require.Equal(t, updateQueue, pMgr.policyMap.dirtyCache.queue)
	require.Equal(t, map[string]*NPMNetworkPolicy{
		egressNetPol().PolicyKey:  egressNetPol(),
		ingressNetPol().PolicyKey: ingressNetPol(),
	}, pMgr.policyMap.cache)
	require.Equal(t, pMgr.policyMap.policiesInKernel, 2)
	promVals{4, 4}.testPrometheusMetrics(t)

	toRemoveActual, toAddActual = pMgr.dirtyNetPols()
	for _, v := range toRemove {
		require.Contains(t, toRemoveActual, v, "expected: %+v. actual: %+v", toRemove, toRemoveActual)
	}
	require.Len(t, toRemoveActual, len(toRemove), "expected: %+v. actual: %+v", toRemove, toRemoveActual)
	require.Equal(t, toAdd, toAddActual)
	require.Equal(t, updateQueue, pMgr.policyMap.dirtyCache.queue)

	require.Error(t, pMgr.reconcileDirtyNetPolsInKernel(toRemove, toAdd))
	require.Equal(t, map[string]*NPMNetworkPolicy{
		egressNetPol().PolicyKey:  egressNetPol(),
		ingressNetPol().PolicyKey: ingressNetPol(),
	}, pMgr.policyMap.cache)
	require.Equal(t, map[string][]*event{
		egressNetPol().PolicyKey: {
			{
				op: remove,
				deletedState: &deletedState{
					namespace:       egressNetPol().Namespace,
					direction:       Egress,
					podSelectorList: egressNetPol().PodSelectorList,
					wasInKernel:     true,
				},
			},
			{op: add},
		},
		ingressNetPol().PolicyKey: {
			{op: add},
		},
	}, pMgr.policyMap.dirtyCache.queue)
	require.Equal(t, 1, pMgr.policyMap.policiesInKernel)
}
