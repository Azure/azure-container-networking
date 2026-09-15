package debug

import (
	"fmt"
	"testing"

	common "github.com/Azure/azure-container-networking/npm/pkg/controlplane/controllers/common"
	"github.com/Azure/azure-container-networking/npm/pkg/dataplane/pb"
	"github.com/Azure/azure-container-networking/npm/util"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
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

func TestCIDRNamedPortConditionsRespectVersion(t *testing.T) {
	const (
		matchingIP   = "10.0.0.10"
		ruleProtocol = "tcp"
	)
	webPort := []corev1.ContainerPort{{Name: "web", ContainerPort: 8080, Protocol: corev1.ProtocolTCP}}
	for _, enableV2 := range []bool{false, true} {
		for _, sourceSide := range []bool{false, true} {
			for _, cidrFirst := range []bool{false, true} {
				for _, test := range []struct {
					name   string
					ip     string
					ports  []corev1.ContainerPort
					wantV1 bool
					wantV2 bool
				}{
					{"both match", matchingIP, webPort, true, true},
					{"CIDR only", matchingIP, nil, true, false},
					{"named port only", "10.1.0.10", webPort, true, false},
					{"neither matches", "10.1.0.10", nil, false, false},
					{
						"wrong protocol", matchingIP,
						[]corev1.ContainerPort{{Name: "web", ContainerPort: 8080, Protocol: corev1.ProtocolUDP}},
						true, false,
					},
				} {
					t.Run(fmt.Sprintf("v2=%t/source=%t/cidrFirst=%t/%s", enableV2, sourceSide, cidrFirst, test.name), func(t *testing.T) {
						cidr := &pb.RuleResponse_SetInfo{
							Name: util.CIDRPrefix + "peer", HashedSetName: "cidr", Type: pb.SetType_CIDRBLOCKS,
							Included: true, Contents: []string{"10.0.0.0/24"},
						}
						namedPort := &pb.RuleResponse_SetInfo{
							Name: util.NamedPortIPSetPrefix + "web", HashedSetName: "named-port", Type: pb.SetType_NAMEDPORTS,
							Included: true,
						}
						peerSets := []*pb.RuleResponse_SetInfo{namedPort, cidr}
						if cidrFirst {
							peerSets = []*pb.RuleResponse_SetInfo{cidr, namedPort}
						}
						targetSets := []*pb.RuleResponse_SetInfo{{
							Name: util.NamespacePrefix + anchorTargetNamespace, HashedSetName: "target",
							Type: pb.SetType_NAMESPACE, Included: true,
						}}
						peer := &common.NpmPod{Namespace: anchorPeerNamespace, PodIP: test.ip, ContainerPorts: test.ports}
						target := &common.NpmPod{Namespace: anchorTargetNamespace}
						allow := &pb.RuleResponse{
							Allowed: true, Protocol: ruleProtocol, Direction: pb.Direction_EGRESS,
							SrcList: targetSets, DstList: peerSets,
						}
						deny := &pb.RuleResponse{Direction: pb.Direction_EGRESS, SrcList: targetSets}
						src, dst := target, peer
						if sourceSide {
							src, dst = peer, target
							allow.Direction, deny.Direction = pb.Direction_INGRESS, pb.Direction_INGRESS
							allow.SrcList, allow.DstList = peerSets, targetSets
							deny.SrcList, deny.DstList = nil, targetSets
						}
						hits, srcSets, dstSets, err := getHitRules(
							src, dst, map[*pb.RuleResponse]struct{}{allow: {}, deny: {}}, &common.Cache{}, enableV2,
						)
						require.NoError(t, err)
						wantMatch := test.wantV1
						if enableV2 {
							wantMatch = test.wantV2
						}
						want := []*pb.RuleResponse{deny}
						if wantMatch {
							want = append(want, allow)
						}
						require.ElementsMatch(t, want, hits)
						if enableV2 && wantMatch {
							matchedSets, port := dstSets, allow.GetDPort()
							if sourceSide {
								matchedSets, port = srcSets, allow.GetSPort()
							}
							require.Len(t, matchedSets, 2)
							require.Equal(t, cidr, matchedSets[cidr.GetHashedSetName()])
							require.Equal(t, namedPort, matchedSets[namedPort.GetHashedSetName()])
							require.Equal(t, int32(8080), port)
						}
					})
				}
			}
		}
	}
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
