package main

import (
	"github.com/Azure/azure-container-networking/common"
	"github.com/Azure/azure-container-networking/npm/pkg/dataplane"
	"github.com/Azure/azure-container-networking/npm/pkg/dataplane/ipsets"
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
	testNSSet        = createTestSet("test-ns-set", ipsets.NameSpace)
	testKeyPodSet    = createTestSet("test-keyPod-set", ipsets.KeyLabelOfPod)
	testKVPodSet     = createTestSet("test-kvPod-set", ipsets.KeyValueLabelOfPod)
	testNamedportSet = createTestSet("test-namedport-set", ipsets.NamedPorts)
	testCIDRSet      = createTestSet("test-cidr-set", ipsets.CIDRBlocks)
	// testKeyNSList       = createTestSet("test-keyNS-list", ipsets.KeyLabelOfNameSpace)
	// testKVNSList        = createTestSet("test-kvNS-list", ipsets.KeyValueLabelOfNameSpace)
	// testNestedLabelList = createTestSet("test-nestedlabel-list", ipsets.NestedLabelOfPod)
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

	// NOTE for Linux
	/*
		ipset test SETNAME ENTRYNAME:
			Warning: 10.0.0.5 is in set azure-npm-2031808719.
			10.0.0.4 is NOT in set azure-npm-2031808719.

		ipset list (references are from setlist or iptables):
			Name: azure-npm-3382169694
			Type: hash:net
			Revision: 6
			Header: family inet hashsize 1024 maxelem 65536
			Size in memory: 512
			References: 0
			Number of entries: 1
			Members:
			10.0.0.0

			Name: azure-npm-2031808719
			Type: hash:net
			Revision: 6
			Header: family inet hashsize 1024 maxelem 65536
			Size in memory: 512
			References: 0
			Number of entries: 1
			Members:
			10.0.0.5

			Name: azure-npm-164288419
			Type: hash:ip,port
			Revision: 5
			Header: family inet hashsize 1024 maxelem 65536
			Size in memory: 192
			References: 0
			Number of entries: 0
			Members:

			Name: azure-npm-3216600258
			Type: hash:net
			Revision: 6
			Header: family inet hashsize 1024 maxelem 4294967295
			Size in memory: 448
			References: 0
			Number of entries: 0
			Members:
	*/
}
