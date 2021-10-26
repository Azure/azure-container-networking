package policies

import (
	"fmt"
	"strings"
	"testing"

	"github.com/Azure/azure-container-networking/common"
	"github.com/Azure/azure-container-networking/npm/pkg/dataplane/ipsets"
	dptestutils "github.com/Azure/azure-container-networking/npm/pkg/dataplane/testutils"
	testutils "github.com/Azure/azure-container-networking/test/utils"
	"github.com/stretchr/testify/require"
)

var (
	fakeIPTablesRestoreCommand        = testutils.TestCmd{Cmd: []string{"iptables-restore", "-T", "filter", "--noflush"}}
	fakeIPTablesRestoreFailureCommand = testutils.TestCmd{Cmd: []string{"iptables-restore", "-T", "filter", "--noflush"}, ExitCode: 1}

	testACLs = []*ACLPolicy{
		{
			PolicyID: "test1",
			Comment:  "comment1",
			SrcList: []SetInfo{
				{
					ipsets.TestCIDRSet.Metadata,
					true,
					SrcMatch,
				},
			},
			DstList: []SetInfo{
				{
					ipsets.TestKeyPodSet.Metadata,
					false,
					DstMatch,
				},
			},
			Target:    Dropped,
			Direction: Ingress,
			SrcPorts: []Ports{
				{144, 255},
			},
			DstPorts: []Ports{
				{222, 333},
				{456, 456},
			},
			Protocol: TCP,
		},
		{
			PolicyID: "test2",
			Comment:  "comment2",
			SrcList: []SetInfo{
				{
					ipsets.TestCIDRSet.Metadata,
					true,
					SrcMatch,
				},
			},
			Target:    Allowed,
			Direction: Ingress,
			SrcPorts: []Ports{
				{144, 144},
			},
			Protocol: UDP,
		},
		{
			PolicyID: "test3",
			Comment:  "comment3",
			SrcList: []SetInfo{
				{
					ipsets.TestCIDRSet.Metadata,
					true,
					SrcMatch,
				},
			},
			Target:    Dropped,
			Direction: Egress,
			DstPorts: []Ports{
				{144, 144},
			},
			Protocol: UDP,
		},
		{
			PolicyID: "test4",
			Comment:  "comment4",
			SrcList: []SetInfo{
				{
					ipsets.TestCIDRSet.Metadata,
					true,
					SrcMatch,
				},
			},
			Target:    Allowed,
			Direction: Egress,
			Protocol:  AnyProtocol,
		},
	}

	testNetworkPolicies = []*NPMNetworkPolicy{
		{
			Name: "test1",
			PodSelectorIPSets: []*ipsets.TranslatedIPSet{
				{Metadata: ipsets.TestKVNSList.Metadata},
			},
			ACLs: testACLs,
		},
		{
			Name: "test2",
			PodSelectorIPSets: []*ipsets.TranslatedIPSet{
				{Metadata: ipsets.TestKVNSList.Metadata},
				{Metadata: ipsets.TestKeyPodSet.Metadata},
			},
			ACLs: []*ACLPolicy{
				testACLs[0],
			},
		},
		{
			Name: "test3",
			ACLs: []*ACLPolicy{
				testACLs[3],
			},
		},
	}

	testPolicy1IngressChain = testNetworkPolicies[0].getIngressChainName()
	testPolicy1EgressChain  = testNetworkPolicies[0].getEgressChainName()
	testPolicy2IngressChain = testNetworkPolicies[1].getIngressChainName()
	testPolicy3EgressChain  = testNetworkPolicies[2].getEgressChainName()

	testPolicy1IngressJump = fmt.Sprintf("-j %s -m set --match-set %s dst", testPolicy1IngressChain, ipsets.TestKVNSList.HashedName)
	testPolicy1EgressJump  = fmt.Sprintf("-j %s -m set --match-set %s src", testPolicy1EgressChain, ipsets.TestKVNSList.HashedName)
	testPolicy2IngressJump = fmt.Sprintf("-j %s -m set --match-set %s dst -m set --match-set %s dst", testPolicy2IngressChain, ipsets.TestKVNSList.HashedName, ipsets.TestKeyPodSet.HashedName)
	testPolicy3EgressJump  = fmt.Sprintf("-j %s", testPolicy3EgressChain)

	testACLRule1 = fmt.Sprintf(
		"-j MARK --set-mark 0x4000 -p tcp --sport 144:255 -m multiport --dports 222:333,456 -m set --match-set %s src -m set ! --match-set %s dst -m comment --comment comment1",
		ipsets.TestCIDRSet.HashedName,
		ipsets.TestKeyPodSet.HashedName,
	)
	testACLRule2 = fmt.Sprintf("-j AZURE-NPM-EGRESS -p udp --sport 144 -m set --match-set %s src -m comment --comment comment2", ipsets.TestCIDRSet.HashedName)
	testACLRule3 = fmt.Sprintf("-j MARK --set-mark 0x5000 -p udp --dport 144 -m set --match-set %s src -m comment --comment comment3", ipsets.TestCIDRSet.HashedName)
	testACLRule4 = fmt.Sprintf("-j AZURE-NPM-ACCEPT -p all -m set --match-set %s src -m comment --comment comment4", ipsets.TestCIDRSet.HashedName)
)

func TestAddPolicies(t *testing.T) {
	calls := []testutils.TestCmd{fakeIPTablesRestoreCommand}
	pMgr := NewPolicyManager(common.NewMockIOShim(calls))
	creator := pMgr.getCreatorForNewNetworkPolicies(testNetworkPolicies...)
	fileString := creator.ToString()
	expectedLines := []string{
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
		"COMMIT\n",
	}
	expectedFileString := strings.Join(expectedLines, "\n")
	dptestutils.AssertEqualFileStrings(t, expectedFileString, fileString)

	err := pMgr.addPolicy(testNetworkPolicies[0], nil)
	require.NoError(t, err)
}

func TestAddPoliciesError(t *testing.T) {
	calls := []testutils.TestCmd{fakeIPTablesRestoreFailureCommand}
	pMgr := NewPolicyManager(common.NewMockIOShim(calls))
	err := pMgr.addPolicy(testNetworkPolicies[0], nil)
	require.Error(t, err)
}

func TestRemovePolicies(t *testing.T) {
	calls := []testutils.TestCmd{
		fakeIPTablesRestoreCommand,
		getFakeDeleteJumpCommand("AZURE-NPM-INGRESS", testPolicy1IngressJump),
		getFakeDeleteJumpCommand("AZURE-NPM-EGRESS", testPolicy1EgressJump),
		fakeIPTablesRestoreCommand,
	}
	pMgr := NewPolicyManager(common.NewMockIOShim(calls))
	creator := pMgr.getCreatorForRemovingPolicies(testNetworkPolicies...)
	fileString := creator.ToString()
	expectedLines := []string{
		"*filter",
		fmt.Sprintf(":%s - -", testPolicy1IngressChain),
		fmt.Sprintf(":%s - -", testPolicy1EgressChain),
		fmt.Sprintf(":%s - -", testPolicy2IngressChain),
		fmt.Sprintf(":%s - -", testPolicy3EgressChain),
		"COMMIT\n",
	}
	expectedFileString := strings.Join(expectedLines, "\n")
	dptestutils.AssertEqualFileStrings(t, expectedFileString, fileString)

	err := pMgr.AddPolicy(testNetworkPolicies[0], nil) // need the policy in the cache
	require.NoError(t, err)
	err = pMgr.removePolicy(testNetworkPolicies[0].Name, nil)
	require.NoError(t, err)
}

func TestRemovePoliciesError(t *testing.T) {
	calls := []testutils.TestCmd{
		fakeIPTablesRestoreCommand,
		getFakeDeleteJumpCommand("AZURE-NPM-INGRESS", testPolicy1IngressJump),
		getFakeDeleteJumpCommand("AZURE-NPM-EGRESS", testPolicy1EgressJump),
		fakeIPTablesRestoreFailureCommand,
	}
	pMgr := NewPolicyManager(common.NewMockIOShim(calls))
	err := pMgr.AddPolicy(testNetworkPolicies[0], nil)
	require.NoError(t, err)
	err = pMgr.RemovePolicy(testNetworkPolicies[0].Name, nil)
	require.Error(t, err)
}

func getFakeDeleteJumpCommand(chainName, jumpRule string) testutils.TestCmd {
	args := []string{"iptables", "-w", "60", "-D", chainName}
	args = append(args, strings.Split(jumpRule, " ")...)
	return testutils.TestCmd{Cmd: args}
}
