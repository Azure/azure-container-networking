package policies

import "github.com/Azure/azure-container-networking/npm/pkg/dataplane/ipsets"

// TODO: deprecate this file. Updating this file impacts multiple tests.
var (
	// TestNetworkPolicies for testing
	TestNetworkPolicies = []*NPMNetworkPolicy{
		{
			Name:      "test1",
			NameSpace: "x",
			PolicyKey: "x/test1",
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
			ACLs: testACLs,
		},
		{
			Name:      "test2",
			NameSpace: "y",
			PolicyKey: "y/test2",
			PodSelectorIPSets: []*ipsets.TranslatedIPSet{
				{Metadata: ipsets.TestKeyPodSet.Metadata},
				{Metadata: ipsets.TestKVPodSet.Metadata},
			},
			PodSelectorList: []SetInfo{
				{
					IPSet:     ipsets.TestKeyPodSet.Metadata,
					Included:  true,
					MatchType: EitherMatch,
				},
				{
					IPSet:     ipsets.TestKVPodSet.Metadata,
					Included:  true,
					MatchType: EitherMatch,
				},
			},
			ACLs: []*ACLPolicy{
				testACLs[0],
			},
		},
		{
			Name:      "test3",
			NameSpace: "z",
			PolicyKey: "z/test3",
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
			DstPorts: Ports{
				222, 333,
			},
			Protocol: TCP,
		},
		{
			PolicyID: "test1",
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
			Protocol:  UDP,
		},
		{
			PolicyID: "test1",
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
			DstPorts: Ports{
				144, 144,
			},
			Protocol: UDP,
		},
		{
			PolicyID: "test1",
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
			Protocol:  UnspecifiedProtocol,
		},
	}
)
