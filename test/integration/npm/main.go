package main

import (
	"github.com/Azure/azure-container-networking/common"
	"github.com/Azure/azure-container-networking/npm/pkg/dataplane"
	"github.com/Azure/azure-container-networking/npm/pkg/dataplane/ipsets"
	"github.com/Azure/azure-container-networking/npm/pkg/dataplane/policies"
	"github.com/Azure/azure-container-networking/npm/util"
)

type testSet struct {
	metadata   *ipsets.IPSetMetadata
	hashedName string
}

func createTestSet(name string, setType ipsets.SetType) *testSet {
	set := &testSet{
		metadata: &ipsets.IPSetMetadata{
			Name: name,
			Type: setType,
		},
	}
	set.hashedName = util.GetHashedName(set.metadata.GetPrefixName())
	return set
}

var (
	testNSSet        = createTestSet("test-ns-set", ipsets.Namespace)
	testKeyPodSet    = createTestSet("test-keyPod-set", ipsets.KeyLabelOfPod)
	testKVPodSet     = createTestSet("test-kvPod-set", ipsets.KeyValueLabelOfPod)
	testNamedportSet = createTestSet("test-namedport-set", ipsets.NamedPorts)
	testCIDRSet      = createTestSet("test-cidr-set", ipsets.CIDRBlocks)
	// testKeyNSList       = createTestSet("test-keyNS-list", ipsets.KeyLabelOfNameSpace)
	// testKVNSList        = createTestSet("test-kvNS-list", ipsets.KeyValueLabelOfNameSpace)
	// testNestedLabelList = createTestSet("test-nestedlabel-list", ipsets.NestedLabelOfPod)

	testNetworkPolicies = policies.GetTestNetworkPolicies()
)

func main() {
	dp := dataplane.NewDataPlane("", common.NewIOShim())

	// add all types of ipsets, some with members added
	dp.CreateIPSet(testNSSet.metadata)
	if err := dp.AddToSet([]*ipsets.IPSetMetadata{testNSSet.metadata}, "10.0.0.0", "a"); err != nil {
		panic(err)
	}
	if err := dp.AddToSet([]*ipsets.IPSetMetadata{testNSSet.metadata}, "10.0.0.1", "b"); err != nil {
		panic(err)
	}
	dp.CreateIPSet(testKeyPodSet.metadata)
	if err := dp.AddToSet([]*ipsets.IPSetMetadata{testKeyPodSet.metadata}, "10.0.0.5", "c"); err != nil {
		panic(err)
	}
	dp.CreateIPSet(testKVPodSet.metadata)
	dp.CreateIPSet(testNamedportSet.metadata)
	dp.CreateIPSet(testCIDRSet.metadata)

	// can't do lists on my computer

	if err := dp.ApplyDataPlane(); err != nil {
		panic(err)
	}

	// remove members from some sets and delete some sets
	if err := dp.RemoveFromSet([]*ipsets.IPSetMetadata{testNSSet.metadata}, "10.0.0.1", "b"); err != nil {
		panic(err)
	}
	dp.DeleteIPSet(testKVPodSet.metadata)
	if err := dp.ApplyDataPlane(); err != nil {
		panic(err)
	}

	testPolicyManager()
}

func testPolicyManager() {
	pMgr := policies.NewPolicyManager(common.NewIOShim())

	panicOnError(pMgr.Reset())
	// printAndWait()

	panicOnError(pMgr.AddPolicy(testNetworkPolicies[0], nil))
	// printAndWait()

	panicOnError(pMgr.AddPolicy(testNetworkPolicies[1], nil))
	// printAndWait()

	// remove something that doesn't exist
	panicOnError(pMgr.RemovePolicy(testNetworkPolicies[2].Name, nil))
	// printAndWait()

	panicOnError(pMgr.AddPolicy(testNetworkPolicies[2], nil))
	// printAndWait()

	// remove something that exists
	panicOnError(pMgr.RemovePolicy(testNetworkPolicies[1].Name, nil))
}

func panicOnError(err error) {
	if err != nil {
		panic(err)
	}
}
