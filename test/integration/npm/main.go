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
				PolicyID:  "azure-acl-234",
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
	printAndWait()

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
		PodIP:    "10.240.0.24",
		NodeName: nodeName,
	}
	panicOnError(dp.AddToSets([]*ipsets.IPSetMetadata{ipsets.TestKeyPodSet.Metadata, ipsets.TestNSSet.Metadata}, podMetadataC))
	dp.CreateIPSets([]*ipsets.IPSetMetadata{ipsets.TestKVPodSet.Metadata, ipsets.TestNamedportSet.Metadata, ipsets.TestCIDRSet.Metadata})

	// can't do lists on my computer

	panicOnError(dp.ApplyDataPlane())

	printAndWait()

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

	printAndWait()
	panicOnError(dp.RemoveFromSets([]*ipsets.IPSetMetadata{ipsets.TestNSSet.Metadata}, podMetadata))

	dp.DeleteIPSet(ipsets.TestNSSet.Metadata)
	panicOnError(dp.ApplyDataPlane())
	printAndWait()

	panicOnError(dp.AddPolicy(testNetPol))
	panicOnError(dp.AddPolicy(policies.TestNetworkPolicies[0]))
	panicOnError(dp.AddPolicy(policies.TestNetworkPolicies[1]))
	printAndWait()

	panicOnError(dp.RemovePolicy(policies.TestNetworkPolicies[2].Name)) // no-op
	panicOnError(dp.AddPolicy(policies.TestNetworkPolicies[2]))
	printAndWait()

	panicOnError(dp.RemovePolicy(policies.TestNetworkPolicies[1].Name))
}

func panicOnError(err error) {
	if err != nil {
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
