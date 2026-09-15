package translation

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/require"
	networkingv1 "k8s.io/api/networking/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestMalformedPodSelectorsFailBeforeTranslation(t *testing.T) {
	for _, test := range []struct {
		name        string
		requirement metav1.LabelSelectorRequirement
		cause       error
	}{
		{"unknown operator", metav1.LabelSelectorRequirement{Key: appLabelKey, Operator: "Unknown"}, ErrUnsupportedMatchExpressionOperator},
		{"empty In", metav1.LabelSelectorRequirement{Key: appLabelKey, Operator: metav1.LabelSelectorOpIn}, ErrEmptyMatchExpressionValues},
		{"empty NotIn", metav1.LabelSelectorRequirement{Key: appLabelKey, Operator: metav1.LabelSelectorOpNotIn}, ErrEmptyMatchExpressionValues},
		{"invalid value", metav1.LabelSelectorRequirement{Key: appLabelKey, Operator: metav1.LabelSelectorOpIn, Values: []string{"invalid value"}}, ErrInvalidMatchExpressionValues},
	} {
		for _, direction := range []networkingv1.PolicyType{networkingv1.PolicyTypeIngress, networkingv1.PolicyTypeEgress} {
			for _, position := range []string{"selected", "peer", "combined peer"} {
				t.Run(fmt.Sprintf("%s/%s/%s", test.name, direction, position), func(t *testing.T) {
					policy := combinedBudgetPolicy(direction, 0, 1, 1)
					selector := metav1.LabelSelector{MatchExpressions: []metav1.LabelSelectorRequirement{test.requirement}}
					if position == "selected" {
						policy.Spec.PodSelector = selector
					} else {
						var peer *networkingv1.NetworkPolicyPeer
						if direction == networkingv1.PolicyTypeIngress {
							peer = &policy.Spec.Ingress[0].From[0]
						} else {
							peer = &policy.Spec.Egress[0].To[0]
						}
						peer.PodSelector = &selector
						if position == "peer" {
							peer.NamespaceSelector = nil
						}
					}
					before := policy.DeepCopy()
					translated, err := TranslatePolicy(policy, false)
					require.ErrorIs(t, err, test.cause)
					require.Nil(t, translated)
					require.Equal(t, before, policy)
				})
			}
		}
	}
}
