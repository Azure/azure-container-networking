package npm

import (
	"reflect"
	"testing"

	networkingv1 "k8s.io/api/networking/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestSplitPolicy(t *testing.T) {
	policy := &networkingv1.NetworkPolicy{
		Spec: networkingv1.NetworkPolicySpec{
			PodSelector: metav1.LabelSelector{
				MatchLabels: map[string]string{
					"role":     "client",
					"protocol": "https",
				},
				MatchExpressions: []metav1.LabelSelectorRequirement{
					metav1.LabelSelectorRequirement{
						Key:      "testIn",
						Operator: metav1.LabelSelectorOpIn,
						Values: []string{
							"frontend",
							"backend",
						},
					},
					metav1.LabelSelectorRequirement{
						Key:      "testDoesNotExist",
						Operator: metav1.LabelSelectorOpDoesNotExist,
					},
				},
			},
		},
	}

	labels, policies := splitPolicy(policy)

	expectedLabels := []string{
		"role:client",
		"protocol:https",
		"testIn:frontend",
		"testIn:backend",
		"!testDoesNotExist",
	}

	expectedPolicies := []*networkingv1.NetworkPolicy{
		&networkingv1.NetworkPolicy{
			Spec: networkingv1.NetworkPolicySpec{
				PodSelector: metav1.LabelSelector{
					MatchLabels: map[string]string{
						"role": "client",
					},
					MatchExpressions: []metav1.LabelSelectorRequirement{},
				},
			},
		},
		&networkingv1.NetworkPolicy{
			Spec: networkingv1.NetworkPolicySpec{
				PodSelector: metav1.LabelSelector{
					MatchLabels: map[string]string{
						"protocol": "https",
					},
					MatchExpressions: []metav1.LabelSelectorRequirement{},
				},
			},
		},
		&networkingv1.NetworkPolicy{
			Spec: networkingv1.NetworkPolicySpec{
				PodSelector: metav1.LabelSelector{
					MatchLabels: map[string]string{
						"testIn": "frontend",
					},
					MatchExpressions: []metav1.LabelSelectorRequirement{},
				},
			},
		},
		&networkingv1.NetworkPolicy{
			Spec: networkingv1.NetworkPolicySpec{
				PodSelector: metav1.LabelSelector{
					MatchLabels: map[string]string{
						"testIn": "backend",
					},
					MatchExpressions: []metav1.LabelSelectorRequirement{},
				},
			},
		},
		&networkingv1.NetworkPolicy{
			Spec: networkingv1.NetworkPolicySpec{
				PodSelector: metav1.LabelSelector{
					MatchLabels: map[string]string{
						"!testDoesNotExist": "",
					},
					MatchExpressions: []metav1.LabelSelectorRequirement{},
				},
			},
		},
	}

	if len(labels) != len(expectedLabels) {
		t.Errorf("TestsplitPolicy failed @ labels length comparison")
	}

	if len(policies) != len(expectedPolicies) {
		t.Errorf("TestsplitPolicy failed @ policies length comparison")
	}

	for i := range labels {
		if labels[i] != expectedLabels[i] {
			t.Errorf("TestsplitPolicy failed @ label comparison")
		}

		if !reflect.DeepEqual(policies[i].Spec.PodSelector.MatchLabels, expectedPolicies[i].Spec.PodSelector.MatchLabels) {
			t.Errorf("TestsplitPolicy failed @ MatchLabels comparison")
		}

		if !reflect.DeepEqual(policies[i].Spec.PodSelector.MatchExpressions, expectedPolicies[i].Spec.PodSelector.MatchExpressions) {
			t.Errorf("TestsplitPolicy failed @ MatchExpressions comparison")
		}

		if !reflect.DeepEqual(policies[i].Spec.PodSelector, expectedPolicies[i].Spec.PodSelector) {
			t.Error("TestsplitPolicy failed @ PodSelector comparison")
		}

		if !reflect.DeepEqual(*(policies[i]), *(expectedPolicies[i])) {
			t.Error("TestsplitPolicy failed @ policy comparison")
		}
	}
}
