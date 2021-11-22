package main

import (
	"fmt"
	"time"

	"github.com/Azure/azure-container-networking/common"
	"github.com/Azure/azure-container-networking/npm/pkg/dataplane"
	"github.com/Azure/azure-container-networking/npm/pkg/dataplane/ipsets"
	"github.com/Azure/azure-container-networking/npm/pkg/dataplane/policies"
)

const MaxSleepTime = 15

var (
	nodeName   = "testNode"
	testNetPol = &policies.NPMNetworkPolicy{
		Name: "test/test-netpol",
		PodSelectorIPSets: []*ipsets.TranslatedIPSet{
			{
				Metadata: ipsets.TestNSSet.Metadata,
			},
			{
				Metadata: ipsets.TestKeyPodSet.Metadata,
			},
		},
		RuleIPSets: []*ipsets.TranslatedIPSet{
			{
				Metadata: ipsets.TestNSSet.Metadata,
			},
			{
				Metadata: ipsets.TestKeyPodSet.Metadata,
			},
		},
		ACLs: []*policies.ACLPolicy{
			{
				PolicyID:  "azure-acl-123",
				Target:    policies.Dropped,
				Direction: policies.Ingress,
			},
			{
				PolicyID:  "azure-acl-123",
				Target:    policies.Allowed,
				Direction: policies.Ingress,
				SrcList: []policies.SetInfo{
					{
						IPSet:     ipsets.TestNSSet.Metadata,
						Included:  true,
						MatchType: policies.SrcMatch,
					},
					{
						IPSet:     ipsets.TestKeyPodSet.Metadata,
						Included:  true,
						MatchType: policies.SrcMatch,
					},
				},
			},
		},
	}
)

func main() {
	dp, err := dataplane.NewDataPlane(nodeName, common.NewIOShim())
	panicOnError(err)
	printAndWait(true)

	podMetadata := &dataplane.PodMetadata{
		PodKey:   "a",
		PodIP:    "10.0.0.0",
		NodeName: "",
	}

	// add all types of ipsets, some with members added
	panicOnError(dp.AddToSets([]*ipsets.IPSetMetadata{ipsets.TestNSSet.Metadata}, podMetadata))
	podMetadataB := &dataplane.PodMetadata{
		PodKey:   "b",
		PodIP:    "10.0.0.1",
		NodeName: "",
	}
	panicOnError(dp.AddToSets([]*ipsets.IPSetMetadata{ipsets.TestNSSet.Metadata}, podMetadataB))
	podMetadataC := &dataplane.PodMetadata{
		PodKey:   "c",
		PodIP:    "10.240.0.28",
		NodeName: nodeName,
	}
	panicOnError(dp.AddToSets([]*ipsets.IPSetMetadata{ipsets.TestKeyPodSet.Metadata, ipsets.TestNSSet.Metadata}, podMetadataC))
	dp.CreateIPSets([]*ipsets.IPSetMetadata{ipsets.TestKVPodSet.Metadata, ipsets.TestNamedportSet.Metadata, ipsets.TestCIDRSet.Metadata})

	// can't do lists on my computer

	panicOnError(dp.ApplyDataPlane())

	printAndWait(true)

	panicOnError(dp.AddToLists([]*ipsets.IPSetMetadata{ipsets.TestKeyNSList.Metadata, ipsets.TestKVNSList.Metadata}, []*ipsets.IPSetMetadata{ipsets.TestNSSet.Metadata}))

	panicOnError(dp.AddToLists([]*ipsets.IPSetMetadata{ipsets.TestNestedLabelList.Metadata}, []*ipsets.IPSetMetadata{ipsets.TestKVPodSet.Metadata, ipsets.TestKeyPodSet.Metadata}))

	// remove members from some sets and delete some sets
	panicOnError(dp.RemoveFromSets([]*ipsets.IPSetMetadata{ipsets.TestNSSet.Metadata}, podMetadataB))
	podMetadataD := &dataplane.PodMetadata{
		PodKey:   "d",
		PodIP:    "1.2.3.4",
		NodeName: "",
	}
	panicOnError(dp.AddToSets([]*ipsets.IPSetMetadata{ipsets.TestKeyPodSet.Metadata, ipsets.TestNSSet.Metadata}, podMetadataD))
	dp.DeleteIPSet(ipsets.TestKVPodSet.Metadata)
	panicOnError(dp.ApplyDataPlane())

	printAndWait(true)
	panicOnError(dp.RemoveFromSets([]*ipsets.IPSetMetadata{ipsets.TestNSSet.Metadata}, podMetadata))

	dp.DeleteIPSet(ipsets.TestNSSet.Metadata)
	panicOnError(dp.ApplyDataPlane())
	printAndWait(true)

	panicOnError(dp.AddPolicy(testNetPol))
	printAndWait(true)

	panicOnError(dp.RemovePolicy(testNetPol.Name))
	printAndWait(true)

	panicOnError(dp.AddPolicy(testNetPol))
	printAndWait(true)

	podMetadataD = &dataplane.PodMetadata{
		PodKey:   "d",
		PodIP:    "10.240.0.27",
		NodeName: nodeName,
	}
	panicOnError(dp.AddToSets([]*ipsets.IPSetMetadata{ipsets.TestKeyPodSet.Metadata, ipsets.TestNSSet.Metadata}, podMetadataD))
	panicOnError(dp.ApplyDataPlane())
	printAndWait(true)

	panicOnError(dp.RemovePolicy(testNetPol.Name))
	panicOnError(dp.AddPolicy(policies.TestNetworkPolicies[0]))
	panicOnError(dp.AddPolicy(policies.TestNetworkPolicies[1]))
	printAndWait(true)

	panicOnError(dp.RemovePolicy(policies.TestNetworkPolicies[2].Name)) // no-op
	panicOnError(dp.AddPolicy(policies.TestNetworkPolicies[2]))
	printAndWait(true)

	panicOnError(dp.RemovePolicy(policies.TestNetworkPolicies[1].Name))

	testPolicyManager()
}

func testPolicyManager() {
	pMgr := policies.NewPolicyManager(common.NewIOShim())

	panicOnError(pMgr.Reset(nil))
	printAndWait(false)

	panicOnError(pMgr.Initialize())
	printAndWait(false)

	panicOnError(pMgr.AddPolicy(policies.TestNetworkPolicies[0], nil))
	printAndWait(false)

	panicOnError(pMgr.AddPolicy(policies.TestNetworkPolicies[1], nil))
	printAndWait(false)

	// remove something that doesn't exist
	panicOnError(pMgr.RemovePolicy(policies.TestNetworkPolicies[2].Name, nil))
	printAndWait(false)

	panicOnError(pMgr.AddPolicy(policies.TestNetworkPolicies[2], nil))
	printAndWait(false)
}

func panicOnError(err error) {
	if err != nil {
		panic(err)
	}
}

func printAndWait(wait bool) {
	fmt.Printf("#####################\nCompleted running, please check relevant commands, script will resume in %d secs\n#############\n", MaxSleepTime)
	if wait {
		for i := 0; i < MaxSleepTime; i++ {
			fmt.Print(".")
			time.Sleep(time.Second)
		}
	}
}
