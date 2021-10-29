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
	testPolicy1IngressChain = TestNetworkPolicies[0].getIngressChainName()
	testPolicy1EgressChain  = TestNetworkPolicies[0].getEgressChainName()
	testPolicy2IngressChain = TestNetworkPolicies[1].getIngressChainName()
	testPolicy3EgressChain  = TestNetworkPolicies[2].getEgressChainName()

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
	creator := pMgr.getCreatorForNewNetworkPolicies(TestNetworkPolicies...)
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
	dptestutils.AssertEqualMultilineStrings(t, expectedFileString, fileString)

	err := pMgr.addPolicy(TestNetworkPolicies[0], nil)
	require.NoError(t, err)
}

func TestAddPoliciesError(t *testing.T) {
	calls := []testutils.TestCmd{fakeIPTablesRestoreFailureCommand}
	pMgr := NewPolicyManager(common.NewMockIOShim(calls))
	err := pMgr.addPolicy(TestNetworkPolicies[0], nil)
	require.Error(t, err)
}

func TestRemovePolicies(t *testing.T) {
	calls := []testutils.TestCmd{
		fakeIPTablesRestoreCommand,
		getFakeDeleteJumpCommand("AZURE-NPM-INGRESS", testPolicy1IngressJump),
		getFakeDeleteJumpCommandWithCode("AZURE-NPM-EGRESS", testPolicy1EgressJump, 2), // if the policy chain doesn't exist, we shouldn't error
		fakeIPTablesRestoreCommand,
	}
	pMgr := NewPolicyManager(common.NewMockIOShim(calls))
	creator := pMgr.getCreatorForRemovingPolicies(TestNetworkPolicies...)
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
	dptestutils.AssertEqualMultilineStrings(t, expectedFileString, fileString)

	err := pMgr.AddPolicy(TestNetworkPolicies[0], nil) // need the policy in the cache
	require.NoError(t, err)
	err = pMgr.RemovePolicy(TestNetworkPolicies[0].Name, nil)
	require.NoError(t, err)
}

func TestRemovePoliciesErrorOnRestore(t *testing.T) {
	calls := []testutils.TestCmd{
		fakeIPTablesRestoreCommand,
		getFakeDeleteJumpCommand("AZURE-NPM-INGRESS", testPolicy1IngressJump),
		getFakeDeleteJumpCommand("AZURE-NPM-EGRESS", testPolicy1EgressJump),
		fakeIPTablesRestoreFailureCommand,
	}
	pMgr := NewPolicyManager(common.NewMockIOShim(calls))
	err := pMgr.AddPolicy(TestNetworkPolicies[0], nil)
	require.NoError(t, err)
	err = pMgr.RemovePolicy(TestNetworkPolicies[0].Name, nil)
	require.Error(t, err)
}

func TestRemovePoliciesErrorOnIngressRule(t *testing.T) {
	calls := []testutils.TestCmd{
		fakeIPTablesRestoreCommand,
		getFakeDeleteJumpCommandWithCode("AZURE-NPM-INGRESS", testPolicy1IngressJump, 1), // anything but 0 or 2
	}
	pMgr := NewPolicyManager(common.NewMockIOShim(calls))
	err := pMgr.AddPolicy(TestNetworkPolicies[0], nil)
	require.NoError(t, err)
	err = pMgr.RemovePolicy(TestNetworkPolicies[0].Name, nil)
	require.Error(t, err)
}

func TestRemovePoliciesErrorOnEgressRule(t *testing.T) {
	calls := []testutils.TestCmd{
		fakeIPTablesRestoreCommand,
		getFakeDeleteJumpCommand("AZURE-NPM-INGRESS", testPolicy1IngressJump),
		getFakeDeleteJumpCommandWithCode("AZURE-NPM-EGRESS", testPolicy1EgressJump, 1), // anything but 0 or 2
	}
	pMgr := NewPolicyManager(common.NewMockIOShim(calls))
	err := pMgr.AddPolicy(TestNetworkPolicies[0], nil)
	require.NoError(t, err)
	err = pMgr.RemovePolicy(TestNetworkPolicies[0].Name, nil)
	require.Error(t, err)
}
