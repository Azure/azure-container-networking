package policies

import "github.com/Azure/azure-container-networking/npm/pkg/dataplane/ipsets"

// TODO: deprecate this file. Updating this file impacts multiple tests.
var (
	// TestNetworkPolicies for testing
	TestNetworkPolicies = []*NPMNetworkPolicy{
		{
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
			// derived from testACLs
			RuleIPSets: []*ipsets.TranslatedIPSet{
				{Metadata: ipsets.TestCIDRSet.Metadata, Members: nil},
				{Metadata: ipsets.TestKeyPodSet.Metadata, Members: nil},
			},
			ACLs: testACLs,
		},
		{
			Namespace:   "y",
			PolicyKey:   "y/test2",
			ACLPolicyID: "azure-acl-y-test2",
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
			RuleIPSets: []*ipsets.TranslatedIPSet{
				{Metadata: ipsets.TestCIDRSet.Metadata, Members: nil},
			},
			ACLs: []*ACLPolicy{
				testACLs[0],
			},
		},
		{
			Namespace:   "z",
			PolicyKey:   "z/test3",
			ACLPolicyID: "azure-acl-z-test3",
			RuleIPSets: []*ipsets.TranslatedIPSet{
				{Metadata: ipsets.TestCIDRSet.Metadata, Members: nil},
			},
			ACLs: []*ACLPolicy{
				testACLs[3],
			},
		},
	}

	testACLs = []*ACLPolicy{
		{
			Comment: "comment1",
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
			Comment: "comment2",
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
			Comment: "comment3",
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
			Comment: "comment4",
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
