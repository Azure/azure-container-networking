package debug

import (
	"testing"

	common "github.com/Azure/azure-container-networking/npm/pkg/controlplane/controllers/common"
	"github.com/Azure/azure-container-networking/npm/pkg/controlplane/translation"
	"github.com/Azure/azure-container-networking/npm/pkg/dataplane/ipsets"
	"github.com/Azure/azure-container-networking/npm/pkg/dataplane/pb"
	"github.com/Azure/azure-container-networking/npm/util"
	"github.com/stretchr/testify/require"
	networkingv1 "k8s.io/api/networking/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestV1NamespacePrefixDoesNotSelectV2(t *testing.T) {
	const labelKey = "nslabel-team"
	cache := &common.Cache{NsMap: map[string]*common.Namespace{
		anchorPeerNamespace: {LabelsMap: map[string]string{labelKey: matchedTeamValue}},
	}}
	set := &pb.RuleResponse_SetInfo{
		Name: labelKey, Type: pb.SetType_KEYLABELOFNAMESPACE, Included: true,
	}
	matched, err := matchNamespaceAnchorConditions(
		"src", &common.NpmPod{Namespace: anchorPeerNamespace},
		[]*pb.RuleResponse_SetInfo{set}, &pb.RuleResponse{}, cache, false,
	)
	require.NoError(t, err)
	require.True(t, matched, "v1 user-controlled names must not enable the v2 pre-check")
}

func TestV2PodOnlyPeerRequiresNamespace(t *testing.T) {
	allow := &pb.RuleResponse{
		Allowed: true,
		SrcList: []*pb.RuleResponse_SetInfo{
			{Name: util.PodLabelPrefix + "app:shared", Type: pb.SetType_KEYLABELOFPOD, Included: true},
			{Name: util.NamespacePrefix + anchorPeerNamespace, Type: pb.SetType_NAMESPACE, Included: true},
		},
	}
	target := &common.NpmPod{Namespace: anchorTargetNamespace}
	deny := &pb.RuleResponse{DstList: []*pb.RuleResponse_SetInfo{{
		Name: util.NamespacePrefix + anchorTargetNamespace, Type: pb.SetType_NAMESPACE, Included: true,
	}}}
	rules := map[*pb.RuleResponse]struct{}{allow: {}, deny: {}}
	for _, namespace := range []string{anchorPeerNamespace, "different"} {
		peer := &common.NpmPod{Namespace: namespace, Labels: map[string]string{diagnosticAppLabelKey: diagnosticSharedValue}}
		hits, _, _, err := getHitRules(peer, target, rules, &common.Cache{}, true)
		require.NoError(t, err)
		want := []*pb.RuleResponse{deny}
		if namespace == anchorPeerNamespace {
			want = append(want, allow)
		}
		require.ElementsMatch(t, want, hits)
	}
}

func TestV2PodConditionsWithoutNamespaceAreConjunctive(t *testing.T) {
	peer := &common.NpmPod{Namespace: anchorPeerNamespace, Labels: map[string]string{diagnosticAppLabelKey: diagnosticSharedValue, "role": otherLabelValue}}
	allow := &pb.RuleResponse{Allowed: true, SrcList: []*pb.RuleResponse_SetInfo{
		{Name: util.PodLabelPrefix + "app:shared", Type: pb.SetType_KEYLABELOFPOD, Included: true},
		{Name: util.PodLabelPrefix + "role:required", Type: pb.SetType_KEYLABELOFPOD, Included: true},
	}}
	deny := &pb.RuleResponse{DstList: []*pb.RuleResponse_SetInfo{{
		Name: util.NamespacePrefix + anchorTargetNamespace, Type: pb.SetType_NAMESPACE, Included: true,
	}}}
	target := &common.NpmPod{Namespace: anchorTargetNamespace}
	rules := map[*pb.RuleResponse]struct{}{allow: {}, deny: {}}
	hits, _, _, err := getHitRules(peer, target, rules, &common.Cache{}, true)
	require.NoError(t, err)
	require.ElementsMatch(t, []*pb.RuleResponse{deny}, hits)
	peer.Labels["role"] = "required"
	hits, _, _, err = getHitRules(peer, target, rules, &common.Cache{}, true)
	require.NoError(t, err)
	require.ElementsMatch(t, []*pb.RuleResponse{allow, deny}, hits)
}

func TestV2NestedSelectorUsesTranslatedLabelKey(t *testing.T) {
	const labelKey = diagnosticAppLabelKey
	policy := &networkingv1.NetworkPolicy{
		ObjectMeta: metav1.ObjectMeta{Name: "nested", Namespace: anchorPeerNamespace},
		Spec: networkingv1.NetworkPolicySpec{
			PodSelector: metav1.LabelSelector{MatchExpressions: []metav1.LabelSelectorRequirement{{
				Key: labelKey, Operator: metav1.LabelSelectorOpIn, Values: []string{"one", "two"},
			}}},
			PolicyTypes: []networkingv1.PolicyType{networkingv1.PolicyTypeIngress},
			Ingress:     []networkingv1.NetworkPolicyIngressRule{{}},
		},
	}
	translated, err := translation.TranslatePolicy(policy, false)
	require.NoError(t, err)
	var nestedName string
	for _, set := range translated.PodSelectorIPSets {
		if set.Metadata.Type == ipsets.NestedLabelOfPod {
			nestedName = util.NestedLabelPrefix + set.Metadata.Name
		}
	}
	require.NotEmpty(t, nestedName)
	for _, test := range []struct {
		name   string
		labels map[string]string
		want   bool
	}{
		{"first value", map[string]string{labelKey: "one"}, true},
		{"second value", map[string]string{labelKey: "two"}, true},
		{"different value", map[string]string{labelKey: otherLabelValue}, false},
		{"missing key", nil, false},
		{"policy identity is not a key", map[string]string{translated.PolicyKey: labelKey}, false},
	} {
		t.Run(test.name, func(t *testing.T) {
			set := &pb.RuleResponse_SetInfo{Name: nestedName, Type: pb.SetType_NESTEDLABELOFPOD, Included: true}
			matched, err := evaluateSetInfo("src", set, &common.NpmPod{Labels: test.labels}, &pb.RuleResponse{}, &common.Cache{}, true)
			require.NoError(t, err)
			require.Equal(t, test.want, matched)
		})
	}
}
