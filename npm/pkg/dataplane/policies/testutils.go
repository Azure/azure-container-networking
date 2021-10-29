package policies

import "github.com/Azure/azure-container-networking/npm/pkg/dataplane/ipsets"

var (
	// TestNetworkPolicies for testing
	TestNetworkPolicies = []*NPMNetworkPolicy{
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
)
