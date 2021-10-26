package main

import (
	"fmt"
	"time"

	"github.com/Azure/azure-container-networking/common"
	"github.com/Azure/azure-container-networking/npm/pkg/dataplane"
	"github.com/Azure/azure-container-networking/npm/pkg/dataplane/ipsets"
	"github.com/Azure/azure-container-networking/npm/pkg/dataplane/policies"
	"github.com/Azure/azure-container-networking/npm/util"
)

const MaxSleepTime = 15

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
	nodeName            = "testNode"
	testNSSet           = createTestSet("test-ns-set", ipsets.Namespace)
	testKeyPodSet       = createTestSet("test-keyPod-set", ipsets.KeyLabelOfPod)
	testKVPodSet        = createTestSet("test-kvPod-set", ipsets.KeyValueLabelOfPod)
	testNamedportSet    = createTestSet("test-namedport-set", ipsets.NamedPorts)
	testCIDRSet         = createTestSet("test-cidr-set", ipsets.CIDRBlocks)
	testKeyNSList       = createTestSet("test-keyNS-list", ipsets.KeyLabelOfNamespace)
	testKVNSList        = createTestSet("test-kvNS-list", ipsets.KeyValueLabelOfNamespace)
	testNestedLabelList = createTestSet("test-nestedlabel-list", ipsets.NestedLabelOfPod)
	testNetPol          = &policies.NPMNetworkPolicy{
		Name: "test/test-netpol",
		PodSelectorIPSets: []*ipsets.TranslatedIPSet{
			{
				Metadata: testNSSet.metadata,
			},
			{
				Metadata: testKeyPodSet.metadata,
			},
		},
		RuleIPSets: []*ipsets.TranslatedIPSet{
			{
				Metadata: testNSSet.metadata,
			},
			{
				Metadata: testKeyPodSet.metadata,
			},
		},
		ACLs: []*policies.ACLPolicy{
			{
				PolicyID:  "azure-acl-123",
				Target:    policies.Dropped,
				Direction: policies.Ingress,
			},
			{
				PolicyID:  "azure-acl-234",
				Target:    policies.Allowed,
				Direction: policies.Ingress,
				SrcList: []policies.SetInfo{
					{
						IPSet:     testNSSet.metadata,
						Included:  true,
						MatchType: "src",
					},
					{
						IPSet:     testKeyPodSet.metadata,
						Included:  true,
						MatchType: "src",
					},
				},
			},
		},
	}
	// testKeyNSList       = createTestSet("test-keyNS-list", ipsets.KeyLabelOfNameSpace)
	// testKVNSList        = createTestSet("test-kvNS-list", ipsets.KeyValueLabelOfNameSpace)
	// testNestedLabelList = createTestSet("test-nestedlabel-list", ipsets.NestedLabelOfPod)
)

func main() {
	dp, err := dataplane.NewDataPlane(nodeName, common.NewIOShim())
	if err != nil {
		panic(err)
	}
	printAndWait()

	podMetadata := &dataplane.PodMetadata{
		PodKey:   "a",
		PodIP:    "10.0.0.0",
		NodeName: "",
	}

	// add all types of ipsets, some with members added
	if err := dp.AddToSets([]*ipsets.IPSetMetadata{testNSSet.metadata}, podMetadata); err != nil {
		panic(err)
	}
	podMetadataB := &dataplane.PodMetadata{
		PodKey:   "b",
		PodIP:    "10.0.0.1",
		NodeName: "",
	}
	if err := dp.AddToSets([]*ipsets.IPSetMetadata{testNSSet.metadata}, podMetadataB); err != nil {
		panic(err)
	}
	podMetadataC := &dataplane.PodMetadata{
		PodKey:   "c",
		PodIP:    "10.240.0.24",
		NodeName: nodeName,
	}
	if err := dp.AddToSets([]*ipsets.IPSetMetadata{testKeyPodSet.metadata, testNSSet.metadata}, podMetadataC); err != nil {
		panic(err)
	}
	dp.CreateIPSets([]*ipsets.IPSetMetadata{testKVPodSet.metadata, testNamedportSet.metadata, testCIDRSet.metadata})

	// can't do lists on my computer

	if err := dp.ApplyDataPlane(); err != nil {
		panic(err)
	}

	printAndWait()

	if err := dp.AddToLists([]*ipsets.IPSetMetadata{testKeyNSList.metadata, testKVNSList.metadata}, []*ipsets.IPSetMetadata{testNSSet.metadata}); err != nil {
		panic(err)
	}

	if err := dp.AddToLists([]*ipsets.IPSetMetadata{testNestedLabelList.metadata}, []*ipsets.IPSetMetadata{testKVPodSet.metadata, testKeyPodSet.metadata}); err != nil {
		panic(err)
	}

	// remove members from some sets and delete some sets
	if err := dp.RemoveFromSets([]*ipsets.IPSetMetadata{testNSSet.metadata}, podMetadataB); err != nil {
		panic(err)
	}
	dp.DeleteIPSet(testKVPodSet.metadata)
	if err := dp.ApplyDataPlane(); err != nil {
		panic(err)
	}

	printAndWait()
	if err := dp.RemoveFromSets([]*ipsets.IPSetMetadata{testNSSet.metadata}, podMetadata); err != nil {
		panic(err)
	}
	dp.DeleteIPSet(testNSSet.metadata)
	if err := dp.ApplyDataPlane(); err != nil {
		panic(err)
	}
	printAndWait()

	if err := dp.AddPolicy(testNetPol); err != nil {
		panic(err)
	}
}

func printAndWait() {
	fmt.Printf("#####################\nCompleted running, please check relevant commands, script will resume in %d secs\n#############\n", MaxSleepTime)
	for i := 0; i < MaxSleepTime; i++ {
		fmt.Print(".")
		time.Sleep(time.Second)
	}
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
