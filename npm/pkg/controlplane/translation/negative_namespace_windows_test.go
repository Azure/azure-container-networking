package translation

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/require"
	networkingv1 "k8s.io/api/networking/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// TestTranslatePolicyRejectsNegativeNamespaceSelectorOnWindows verifies that a namespaceSelector
// with a negative requirement (multi-value NotIn or DoesNotExist) fails closed during translation
// on the Windows dataplane, for both directions and whether or not it is combined with a
// podSelector. Rejecting here keeps an unrepresentable negated set from reaching the dataplane,
// where an update would otherwise remove the working policy before the add is rejected.
func TestTranslatePolicyRejectsNegativeNamespaceSelectorOnWindows(t *testing.T) {
	for _, requirement := range []metav1.LabelSelectorRequirement{
		{Key: tenantLabelKey, Operator: metav1.LabelSelectorOpNotIn, Values: []string{"a", "b"}},
		{Key: tenantLabelKey, Operator: metav1.LabelSelectorOpDoesNotExist},
	} {
		for _, direction := range []networkingv1.PolicyType{networkingv1.PolicyTypeIngress, networkingv1.PolicyTypeEgress} {
			for _, combined := range []bool{false, true} {
				t.Run(fmt.Sprintf("%s/%s/combined=%t", direction, requirement.Operator, combined), func(t *testing.T) {
					peer := networkingv1.NetworkPolicyPeer{
						NamespaceSelector: &metav1.LabelSelector{MatchExpressions: []metav1.LabelSelectorRequirement{requirement}},
					}
					if combined {
						peer.PodSelector = &metav1.LabelSelector{MatchLabels: map[string]string{appLabelKey: "client"}}
					}
					policy := &networkingv1.NetworkPolicy{
						ObjectMeta: metav1.ObjectMeta{Name: "ns-negative", Namespace: defaultNS},
						Spec:       networkingv1.NetworkPolicySpec{PolicyTypes: []networkingv1.PolicyType{direction}},
					}
					if direction == networkingv1.PolicyTypeIngress {
						policy.Spec.Ingress = []networkingv1.NetworkPolicyIngressRule{{From: []networkingv1.NetworkPolicyPeer{peer}}}
					} else {
						policy.Spec.Egress = []networkingv1.NetworkPolicyEgressRule{{To: []networkingv1.NetworkPolicyPeer{peer}}}
					}
					translated, err := TranslatePolicy(policy, false)
					require.ErrorIs(t, err, ErrUnsupportedNegativeMatch)
					require.Nil(t, translated)
				})
			}
		}
	}
}

// TestTranslatePolicyMalformedNamespaceSelectorReportsValidationErrorOnWindows verifies that on
// Windows a malformed negative namespaceSelector (empty or invalid NotIn) reports the specific
// invalid-spec error, not ErrUnsupportedNegativeMatch, so validation is consistent with the
// podSelector path.
func TestTranslatePolicyMalformedNamespaceSelectorReportsValidationErrorOnWindows(t *testing.T) {
	for _, test := range []struct {
		name string
		req  metav1.LabelSelectorRequirement
		want error
	}{
		{"empty NotIn", metav1.LabelSelectorRequirement{Key: tenantLabelKey, Operator: metav1.LabelSelectorOpNotIn}, ErrEmptyMatchExpressionValues},
		{"invalid NotIn value", metav1.LabelSelectorRequirement{Key: tenantLabelKey, Operator: metav1.LabelSelectorOpNotIn, Values: []string{"bad value"}}, ErrInvalidMatchExpressionValues},
		{"unsupported operator", metav1.LabelSelectorRequirement{Key: tenantLabelKey, Operator: "Superset", Values: []string{"a"}}, ErrUnsupportedMatchExpressionOperator},
	} {
		t.Run(test.name, func(t *testing.T) {
			policy := &networkingv1.NetworkPolicy{
				ObjectMeta: metav1.ObjectMeta{Name: "ns-malformed", Namespace: defaultNS},
				Spec: networkingv1.NetworkPolicySpec{
					PolicyTypes: []networkingv1.PolicyType{networkingv1.PolicyTypeIngress},
					Ingress: []networkingv1.NetworkPolicyIngressRule{{From: []networkingv1.NetworkPolicyPeer{
						{NamespaceSelector: &metav1.LabelSelector{MatchExpressions: []metav1.LabelSelectorRequirement{test.req}}},
					}}},
				},
			}
			translated, err := TranslatePolicy(policy, false)
			require.ErrorIs(t, err, test.want)
			require.NotErrorIs(t, err, ErrUnsupportedNegativeMatch)
			require.Nil(t, translated)
		})
	}
}
