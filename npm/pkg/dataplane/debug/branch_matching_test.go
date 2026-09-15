package debug

import (
	"testing"

	common "github.com/Azure/azure-container-networking/npm/pkg/controlplane/controllers/common"
	"github.com/Azure/azure-container-networking/npm/pkg/dataplane/pb"
	"github.com/Azure/azure-container-networking/npm/util"
	"github.com/stretchr/testify/require"
)

func TestV2MatchedSetsIncludeEveryCondition(t *testing.T) {
	sets := []*pb.RuleResponse_SetInfo{
		{Name: util.NamespacePrefix + anchorPeerNamespace, HashedSetName: "namespace", Type: pb.SetType_NAMESPACE, Included: true},
		{Name: util.PodLabelPrefix + "app:shared", HashedSetName: "app-set", Type: pb.SetType_KEYLABELOFPOD, Included: true},
	}
	rule := &pb.RuleResponse{Allowed: true, SrcList: sets}
	hits, sourceSets, _, err := getHitRules(
		&common.NpmPod{Namespace: anchorPeerNamespace, Labels: map[string]string{diagnosticAppLabelKey: diagnosticSharedValue}},
		&common.NpmPod{Namespace: anchorTargetNamespace},
		map[*pb.RuleResponse]struct{}{rule: {}}, &common.Cache{}, true,
	)
	require.NoError(t, err)
	require.ElementsMatch(t, []*pb.RuleResponse{rule}, hits)
	require.Len(t, sourceSets, 2)
	for _, set := range sets {
		require.Equal(t, set, sourceSets[set.GetHashedSetName()])
	}
}

func TestV2CIDRNamedPortDoesNotEnterSelectorPrecheck(t *testing.T) {
	sets := []*pb.RuleResponse_SetInfo{
		{Name: util.CIDRPrefix + "peer", Type: pb.SetType_CIDRBLOCKS, Included: true},
		{Name: util.NamedPortIPSetPrefix + "web", Type: pb.SetType_NAMEDPORTS, Included: true},
	}
	matched, err := matchNamespaceAnchorConditions(
		"dst", &common.NpmPod{Namespace: anchorPeerNamespace},
		sets, &pb.RuleResponse{DstList: sets}, &common.Cache{}, true,
	)
	require.NoError(t, err)
	require.True(t, matched, "named ports must not turn an IPBlock peer into a selector peer")
}

func TestV2ParentBranchesRemainAlternatives(t *testing.T) {
	child := &pb.RuleResponse{
		Chain: EgressChainPrefix + "policy", Allowed: true, JumpTo: util.IptablesAzureAcceptChain,
		DstList: []*pb.RuleResponse_SetInfo{{
			Name: util.NamespacePrefix + anchorTargetNamespace, Type: pb.SetType_NAMESPACE, Included: true,
		}},
	}
	first := &pb.RuleResponse{
		Chain: EgressChain, JumpTo: child.GetChain(), Comment: "first parent",
		SrcList: []*pb.RuleResponse_SetInfo{{
			Name: util.NamespacePrefix + anchorPeerNamespace, Type: pb.SetType_NAMESPACE, Included: true,
		}},
	}
	second := &pb.RuleResponse{
		Chain: EgressChain, JumpTo: child.GetChain(), Comment: "second parent",
		SrcList: []*pb.RuleResponse_SetInfo{{
			Name: util.NamespacePrefix + "another", Type: pb.SetType_NAMESPACE, Included: true,
		}},
	}
	merged := mergeV2ParentBranches(map[*pb.RuleResponse]struct{}{child: {}, first: {}, second: {}})
	require.Len(t, merged, 2)
	require.Empty(t, child.GetSrcList(), "merging must not mutate the original child")
	for branch := range merged {
		require.Len(t, branch.GetSrcList(), 1)
		require.Len(t, branch.GetDstList(), 1)
		require.Equal(t, child.JumpTo, branch.JumpTo)
		require.Contains(t, []string{first.Comment, second.Comment}, branch.Comment)
	}
	for _, namespace := range []string{anchorPeerNamespace, "another"} {
		hits, _, _, err := getHitRules(
			&common.NpmPod{Namespace: namespace}, &common.NpmPod{Namespace: anchorTargetNamespace},
			merged, &common.Cache{}, true,
		)
		require.NoError(t, err)
		require.Len(t, hits, 1)
		require.Equal(t, child.GetChain(), hits[0].GetChain())
	}
}
