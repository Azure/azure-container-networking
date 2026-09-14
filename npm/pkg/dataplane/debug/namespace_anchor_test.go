package debug

import (
	"fmt"
	"testing"

	common "github.com/Azure/azure-container-networking/npm/pkg/controlplane/controllers/common"
	"github.com/Azure/azure-container-networking/npm/pkg/dataplane/pb"
	"github.com/Azure/azure-container-networking/npm/util"
	"github.com/stretchr/testify/require"
)

const (
	anchorPeerNamespace   = "peer"
	anchorTargetNamespace = "target"
	matchedTeamValue      = "blue"
	otherLabelValue       = "other"
)

func TestV2NamespaceAggregateMatch(t *testing.T) {
	cache := &common.Cache{NsMap: map[string]*common.Namespace{anchorPeerNamespace: {}}}
	for _, namespace := range []string{anchorPeerNamespace, "", "missing"} {
		for _, included := range []bool{true, false} {
			t.Run(fmt.Sprintf("namespace=%q/included=%t", namespace, included), func(t *testing.T) {
				set := &pb.RuleResponse_SetInfo{
					Name:     util.NamespaceLabelPrefix + util.KubeAllNamespacesFlagV2,
					Type:     pb.SetType_KEYLABELOFNAMESPACE,
					Included: included,
				}
				matched, err := evaluateSetInfo("src", set, &common.NpmPod{Namespace: namespace}, &pb.RuleResponse{}, cache)
				require.NoError(t, err)
				require.Equal(t, included == (namespace == anchorPeerNamespace), matched)
			})
		}
	}
}

func TestV2NamespaceKeyOnlyMatch(t *testing.T) {
	for _, test := range []struct {
		name    string
		labels  map[string]string
		present bool
	}{
		{"absent", nil, false},
		{"empty value", map[string]string{"feature": ""}, true},
		{"nonempty value", map[string]string{"feature": "enabled"}, true},
	} {
		for _, included := range []bool{false, true} {
			t.Run(fmt.Sprintf("%s/included=%t", test.name, included), func(t *testing.T) {
				cache := &common.Cache{NsMap: map[string]*common.Namespace{
					anchorPeerNamespace: {LabelsMap: test.labels},
				}}
				set := &pb.RuleResponse_SetInfo{
					Name: util.NamespaceLabelPrefix + "feature", Type: pb.SetType_KEYLABELOFNAMESPACE, Included: included,
				}
				matched, err := evaluateSetInfo("src", set, &common.NpmPod{Namespace: anchorPeerNamespace}, &pb.RuleResponse{}, cache)
				require.NoError(t, err)
				require.Equal(t, included == test.present, matched)
			})
		}
	}
}

func TestV1NamespaceMatchingIsUnchanged(t *testing.T) {
	cache := &common.Cache{NsMap: map[string]*common.Namespace{
		util.NamespacePrefix + anchorPeerNamespace: {LabelsMap: map[string]string{"team": matchedTeamValue}},
	}}
	pod := &common.NpmPod{
		Namespace: anchorPeerNamespace,
		Labels:    map[string]string{"nslabel-team": matchedTeamValue},
	}
	for _, set := range []*pb.RuleResponse_SetInfo{
		{Name: util.NamespacePrefix + "team:blue", Type: pb.SetType_KEYVALUELABELOFNAMESPACE, Included: true},
		{Name: "nslabel-team:blue", Type: pb.SetType_KEYVALUELABELOFPOD, Included: true},
	} {
		matched, err := matchNamespaceAnchorConditions("src", pod, []*pb.RuleResponse_SetInfo{set}, &pb.RuleResponse{}, cache)
		require.NoError(t, err)
		require.True(t, matched, "the v2 pre-check must leave v1 metadata alone")
		matched, err = evaluateSetInfo("src", set, pod, &pb.RuleResponse{}, cache)
		require.NoError(t, err)
		require.True(t, matched)
	}
}

func TestV2MixedNamespaceConditionsAreConjunctive(t *testing.T) {
	for _, tenant := range []string{"x", "y", otherLabelValue} {
		for _, positiveFirst := range []bool{true, false} {
			t.Run(fmt.Sprintf("tenant=%s/positiveFirst=%t", tenant, positiveFirst), func(t *testing.T) {
				cache := &common.Cache{NsMap: map[string]*common.Namespace{
					anchorPeerNamespace: {LabelsMap: map[string]string{"team": matchedTeamValue, "tenant": tenant}},
				}}
				positive := &pb.RuleResponse_SetInfo{Name: util.NamespaceLabelPrefix + "team:blue", Type: pb.SetType_KEYLABELOFNAMESPACE, Included: true}
				excludeX := &pb.RuleResponse_SetInfo{Name: util.NamespaceLabelPrefix + "tenant:x", Type: pb.SetType_KEYLABELOFNAMESPACE}
				excludeY := &pb.RuleResponse_SetInfo{Name: util.NamespaceLabelPrefix + "tenant:y", Type: pb.SetType_KEYLABELOFNAMESPACE}
				sets := []*pb.RuleResponse_SetInfo{positive, excludeX, excludeY}
				if !positiveFirst {
					sets = []*pb.RuleResponse_SetInfo{excludeX, excludeY, positive}
				}
				allow := &pb.RuleResponse{Allowed: true, SrcList: sets}
				deny := &pb.RuleResponse{DstList: []*pb.RuleResponse_SetInfo{{
					Name: util.NamespacePrefix + anchorTargetNamespace, Type: pb.SetType_NAMESPACE, Included: true,
				}}}
				hits, _, _, err := getHitRules(
					&common.NpmPod{Namespace: anchorPeerNamespace}, &common.NpmPod{Namespace: anchorTargetNamespace},
					map[*pb.RuleResponse]struct{}{allow: {}, deny: {}}, cache,
				)
				require.NoError(t, err)
				want := []*pb.RuleResponse{deny}
				if tenant == otherLabelValue {
					want = append(want, allow)
				}
				require.ElementsMatch(t, want, hits)
			})
		}
	}
}

func TestNamespaceAnchorRulesRequireEveryNamespaceMatch(t *testing.T) {
	orders := [][3]int{{0, 1, 2}, {0, 2, 1}, {1, 0, 2}, {1, 2, 0}, {2, 0, 1}, {2, 1, 0}}
	for _, direction := range []pb.Direction{pb.Direction_INGRESS, pb.Direction_EGRESS} {
		for _, tenant := range []string{"a", "b", "good", ""} {
			for _, order := range orders {
				t.Run(fmt.Sprintf("%s/tenant=%q/order=%v", direction, tenant, order), func(t *testing.T) {
					peer := &common.NpmPod{Namespace: anchorPeerNamespace}
					target := &common.NpmPod{Namespace: anchorTargetNamespace}
					cache := &common.Cache{
						NsMap: map[string]*common.Namespace{
							anchorPeerNamespace: {LabelsMap: map[string]string{"tenant": tenant}},
						},
					}

					allMatches := []*pb.RuleResponse_SetInfo{
						{Name: util.NamespaceLabelPrefix + "tenant:a", HashedSetName: "tenant-a", Type: pb.SetType_KEYVALUELABELOFNAMESPACE},
						{Name: util.NamespaceLabelPrefix + "tenant:b", HashedSetName: "tenant-b", Type: pb.SetType_KEYVALUELABELOFNAMESPACE},
						{
							Name: util.NamespaceLabelPrefix + util.KubeAllNamespacesFlagV2, HashedSetName: "aggregate",
							Type: pb.SetType_KEYLABELOFNAMESPACE, Included: true,
						},
					}
					converter := &Converter{EnableV2NPM: true}
					for _, set := range allMatches {
						set.Type, _ = converter.getSetTypeV2(set.GetName())
					}
					peerMatches := []*pb.RuleResponse_SetInfo{allMatches[order[0]], allMatches[order[1]], allMatches[order[2]]}
					targetMatches := []*pb.RuleResponse_SetInfo{{
						Name: util.NamespacePrefix + anchorTargetNamespace, HashedSetName: anchorTargetNamespace, Type: pb.SetType_NAMESPACE, Included: true,
					}}
					allow := &pb.RuleResponse{Allowed: true, Direction: direction, Chain: "allow"}
					deny := &pb.RuleResponse{Direction: direction, Chain: "deny"}
					src, dst := peer, target
					if direction == pb.Direction_INGRESS {
						allow.SrcList, allow.DstList = peerMatches, targetMatches
						deny.DstList = targetMatches
					} else {
						src, dst = target, peer
						allow.SrcList, allow.DstList = targetMatches, peerMatches
						deny.SrcList = targetMatches
					}
					rules := map[*pb.RuleResponse]struct{}{allow: {}, deny: {}}
					hits, _, _, err := getHitRules(src, dst, rules, cache)
					require.NoError(t, err)
					want := []*pb.RuleResponse{deny}
					if tenant != "a" && tenant != "b" {
						want = append(want, allow)
					}
					require.ElementsMatch(t, want, hits)

					peer.Namespace = ""
					hits, _, _, err = getHitRules(src, dst, rules, cache)
					require.NoError(t, err)
					require.ElementsMatch(t, []*pb.RuleResponse{deny}, hits, "the aggregate must not match an external endpoint")
				})
			}
		}
	}
}

func TestNamespaceAnchorConditionsDistinguishLabelPresence(t *testing.T) {
	for _, test := range []struct {
		name   string
		labels map[string]string
		want   bool
	}{
		{"missing label", map[string]string{}, true},
		{"empty value", map[string]string{util.KubeAllNamespacesFlag: ""}, false},
		{"nonempty value", map[string]string{util.KubeAllNamespacesFlag: "yes"}, false},
	} {
		t.Run(test.name, func(t *testing.T) {
			cache := &common.Cache{NsMap: map[string]*common.Namespace{
				anchorPeerNamespace:                        {LabelsMap: test.labels},
				util.NamespacePrefix + anchorPeerNamespace: {LabelsMap: map[string]string{util.KubeAllNamespacesFlag: otherLabelValue}},
			}}
			sets := []*pb.RuleResponse_SetInfo{
				{Name: util.NamespaceLabelPrefix + util.KubeAllNamespacesFlag, Type: pb.SetType_KEYLABELOFNAMESPACE},
				{Name: util.NamespaceLabelPrefix + util.KubeAllNamespacesFlagV2, Type: pb.SetType_KEYLABELOFNAMESPACE, Included: true},
			}
			matched, err := matchNamespaceAnchorConditions("src", &common.NpmPod{Namespace: anchorPeerNamespace}, sets, &pb.RuleResponse{}, cache)
			require.NoError(t, err)
			require.Equal(t, test.want, matched)
		})
	}
}

func TestNamespaceAnchorDoesNotOverridePodSelection(t *testing.T) {
	cache := &common.Cache{NsMap: map[string]*common.Namespace{anchorPeerNamespace: {}}}
	peer := &common.NpmPod{Namespace: anchorPeerNamespace, Labels: map[string]string{"app": otherLabelValue}}
	target := &common.NpmPod{Namespace: anchorTargetNamespace}
	converter := &Converter{EnableV2NPM: true}
	podSet := &pb.RuleResponse_SetInfo{
		Name: util.PodLabelPrefix + "app:required", Included: true,
	}
	podSet.Type, _ = converter.getSetTypeV2(podSet.GetName())
	targetSet := &pb.RuleResponse_SetInfo{
		Name: util.NamespacePrefix + anchorTargetNamespace, Type: pb.SetType_NAMESPACE, Included: true,
	}
	allow := &pb.RuleResponse{
		Allowed: true,
		SrcList: []*pb.RuleResponse_SetInfo{
			{Name: util.NamespaceLabelPrefix + util.KubeAllNamespacesFlagV2, Type: pb.SetType_KEYLABELOFNAMESPACE, Included: true},
			podSet,
		},
		DstList: []*pb.RuleResponse_SetInfo{targetSet},
	}
	deny := &pb.RuleResponse{DstList: []*pb.RuleResponse_SetInfo{targetSet}}
	rules := map[*pb.RuleResponse]struct{}{allow: {}, deny: {}}
	hits, _, _, err := getHitRules(peer, target, rules, cache)
	require.NoError(t, err)
	require.ElementsMatch(t, []*pb.RuleResponse{deny}, hits)

	peer.Labels["app"] = "required"
	hits, _, _, err = getHitRules(peer, target, rules, cache)
	require.NoError(t, err)
	require.ElementsMatch(t, []*pb.RuleResponse{allow, deny}, hits)
}

func TestNamespaceAnchorConditionsReportIncompleteSets(t *testing.T) {
	cache := &common.Cache{NsMap: map[string]*common.Namespace{anchorPeerNamespace: {}}}
	anchor := &pb.RuleResponse_SetInfo{
		Name: util.NamespaceLabelPrefix + util.KubeAllNamespacesFlagV2, Type: pb.SetType_KEYLABELOFNAMESPACE, Included: true,
	}
	for _, test := range []struct {
		name  string
		set   *pb.RuleResponse_SetInfo
		cause error
	}{
		{
			"nested identity without values",
			&pb.RuleResponse_SetInfo{Name: util.NestedLabelPrefix + "policy:key", Type: pb.SetType_NESTEDLABELOFPOD, Included: true},
			common.ErrInvalidInput,
		},
		{
			"unknown set type",
			&pb.RuleResponse_SetInfo{Name: "unknown", Type: pb.SetType_UNKNOWN, Included: true},
			common.ErrSetType,
		},
		{
			"missing label key",
			&pb.RuleResponse_SetInfo{Name: util.PodLabelPrefix + ":value", Type: pb.SetType_KEYLABELOFPOD, Included: true},
			common.ErrInvalidInput,
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			matched, err := matchNamespaceAnchorConditions(
				"src", &common.NpmPod{Namespace: anchorPeerNamespace},
				[]*pb.RuleResponse_SetInfo{anchor, test.set}, &pb.RuleResponse{}, cache,
			)
			require.False(t, matched)
			require.ErrorIs(t, err, test.cause)
		})
	}
}
