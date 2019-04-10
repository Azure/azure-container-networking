package npm

import (
	"fmt"
	"reflect"
	"testing"

	"k8s.io/api/core/v1"
	networkingv1 "k8s.io/api/networking/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
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
		if !reflect.DeepEqual(labels, expectedLabels) {
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

func TestAddPolicy(t *testing.T) {
	tcp, udp := v1.ProtocolTCP, v1.ProtocolUDP
	port6783, port6784 := intstr.FromInt(6783), intstr.FromInt(6784)
	oldIngressPodSelector := &metav1.LabelSelector{
		MatchLabels: map[string]string{
			"app": "db",
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
		},
	}
	oldIngressNamespaceSelector := &metav1.LabelSelector{
		MatchLabels: map[string]string{
			"ns": "dev",
		},
		MatchExpressions: []metav1.LabelSelectorRequirement{
			metav1.LabelSelectorRequirement{
				Key:      "testIn",
				Operator: metav1.LabelSelectorOpIn,
				Values: []string{
					"frontend-ns",
					"backend-ns",
				},
			},
		},
	}
	oldEgressPodSelector := &metav1.LabelSelector{
		MatchLabels: map[string]string{
			"app": "sql",
		},
		MatchExpressions: []metav1.LabelSelectorRequirement{
			metav1.LabelSelectorRequirement{
				Key:      "testNotIn",
				Operator: metav1.LabelSelectorOpNotIn,
				Values: []string{
					"frontend",
					"backend",
				},
			},
		},
	}
	oldPolicy := networkingv1.NetworkPolicy{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: "testnamespace",
		},
		Spec: networkingv1.NetworkPolicySpec{
			PodSelector: metav1.LabelSelector{
				MatchLabels: map[string]string{
					"role": "client",
				},
			},
			Ingress: []networkingv1.NetworkPolicyIngressRule{
				networkingv1.NetworkPolicyIngressRule{
					Ports: []networkingv1.NetworkPolicyPort{
						networkingv1.NetworkPolicyPort{
							Protocol: &tcp,
							Port:     &port6783,
						},
					},
					From: []networkingv1.NetworkPolicyPeer{
						networkingv1.NetworkPolicyPeer{
							PodSelector: oldIngressPodSelector,
						},
						networkingv1.NetworkPolicyPeer{
							PodSelector: oldIngressNamespaceSelector,
						},
					},
				},
			},
			Egress: []networkingv1.NetworkPolicyEgressRule{
				networkingv1.NetworkPolicyEgressRule{
					Ports: []networkingv1.NetworkPolicyPort{},
					To: []networkingv1.NetworkPolicyPeer{
						networkingv1.NetworkPolicyPeer{
							PodSelector: oldEgressPodSelector,
						},
					},
				},
			},
			PolicyTypes: []networkingv1.PolicyType{
				networkingv1.PolicyTypeEgress,
			},
		},
	}

	newPolicy := oldPolicy
	npPort6784 := networkingv1.NetworkPolicyPort{
		Protocol: &udp,
		Port:     &port6784,
	}
	newPolicy.Spec.Ingress[0].Ports = append(newPolicy.Spec.Ingress[0].Ports, npPort6784)
	newPolicy.Spec.Ingress[0].From[0].PodSelector.MatchLabels["status"] = "ok"
	newIngressNamespaceSelector := &metav1.LabelSelector{
		MatchLabels: map[string]string{
			"ns": "new",
		},
		MatchExpressions: []metav1.LabelSelectorRequirement{
			metav1.LabelSelectorRequirement{
				Key:      "testNotIn",
				Operator: metav1.LabelSelectorOpNotIn,
				Values: []string{
					"frontend-ns",
					"backend-ns",
				},
			},
		},
	}
	newPolicy.Spec.Ingress[0].From[1].PodSelector = newIngressNamespaceSelector

	expectedIngress := append(oldPolicy.Spec.Ingress, newPolicy.Spec.Ingress...)
	expectedEgress := append(oldPolicy.Spec.Egress, newPolicy.Spec.Egress...)
	expectedPolicy := &networkingv1.NetworkPolicy{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: "testnamespace",
		},
		Spec: networkingv1.NetworkPolicySpec{
			PodSelector: metav1.LabelSelector{
				MatchLabels: map[string]string{
					"role": "client",
				},
			},
			Ingress: expectedIngress,
			Egress:  expectedEgress,
			PolicyTypes: []networkingv1.PolicyType{
				networkingv1.PolicyTypeEgress,
			},
		},
	}

	addedPolicy, err := addPolicy(&oldPolicy, &newPolicy)
	if err != nil || !reflect.DeepEqual(addedPolicy, expectedPolicy) {
		t.Errorf("TestMergePolicy failed")
		fmt.Println(addedPolicy)
		fmt.Println(expectedPolicy)
	}
}

func TestDeductPolicy(t *testing.T) {
	tcp, udp := v1.ProtocolTCP, v1.ProtocolUDP
	port6783, port6784 := intstr.FromInt(6783), intstr.FromInt(6784)
	oldIngressPodSelector := &metav1.LabelSelector{
		MatchLabels: map[string]string{
			"app": "db",
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
		},
	}
	oldIngressNamespaceSelector := &metav1.LabelSelector{
		MatchLabels: map[string]string{
			"ns": "dev",
		},
		MatchExpressions: []metav1.LabelSelectorRequirement{
			metav1.LabelSelectorRequirement{
				Key:      "testIn",
				Operator: metav1.LabelSelectorOpIn,
				Values: []string{
					"frontend-ns",
					"backend-ns",
				},
			},
		},
	}
	oldEgressPodSelector := &metav1.LabelSelector{
		MatchLabels: map[string]string{
			"app": "sql",
		},
		MatchExpressions: []metav1.LabelSelectorRequirement{
			metav1.LabelSelectorRequirement{
				Key:      "testNotIn",
				Operator: metav1.LabelSelectorOpNotIn,
				Values: []string{
					"frontend",
					"backend",
				},
			},
		},
	}
	oldPolicy := networkingv1.NetworkPolicy{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: "testnamespace",
		},
		Spec: networkingv1.NetworkPolicySpec{
			PodSelector: metav1.LabelSelector{
				MatchLabels: map[string]string{
					"role": "client",
				},
			},
			Ingress: []networkingv1.NetworkPolicyIngressRule{
				networkingv1.NetworkPolicyIngressRule{
					Ports: []networkingv1.NetworkPolicyPort{
						networkingv1.NetworkPolicyPort{
							Protocol: &tcp,
							Port:     &port6783,
						},
					},
					From: []networkingv1.NetworkPolicyPeer{
						networkingv1.NetworkPolicyPeer{
							PodSelector: oldIngressPodSelector,
						},
						networkingv1.NetworkPolicyPeer{
							PodSelector: oldIngressNamespaceSelector,
						},
					},
				},
			},
			Egress: []networkingv1.NetworkPolicyEgressRule{
				networkingv1.NetworkPolicyEgressRule{
					Ports: []networkingv1.NetworkPolicyPort{},
					To: []networkingv1.NetworkPolicyPeer{
						networkingv1.NetworkPolicyPeer{
							PodSelector: oldEgressPodSelector,
						},
					},
				},
			},
			PolicyTypes: []networkingv1.PolicyType{
				networkingv1.PolicyTypeEgress,
			},
		},
	}

	newPolicy := oldPolicy
	newPolicy.Spec.Ingress[0].From = newPolicy.Spec.Ingress[0].From[0:1]
	newPolicy.Spec.Egress[0].To[0] = networkingv1.NetworkPolicyPeer{
		PodSelector: &metav1.LabelSelector{},
	}

	expectedPolicy := networkingv1.NetworkPolicy{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: "testnamespace",
		},
		Spec: networkingv1.NetworkPolicySpec{
			PodSelector: metav1.LabelSelector{
				MatchLabels: map[string]string{
					"role": "client",
				},
			},
			Ingress: []networkingv1.NetworkPolicyIngressRule{
				networkingv1.NetworkPolicyIngressRule{
					Ports: []networkingv1.NetworkPolicyPort{
						networkingv1.NetworkPolicyPort{
							Protocol: &tcp,
							Port:     &port6783,
						},
					},
					From: []networkingv1.NetworkPolicyPeer{
						networkingv1.NetworkPolicyPeer{
							PodSelector: oldIngressNamespaceSelector,
						},
					},
				},
			},
			Egress: []networkingv1.NetworkPolicyEgressRule{
				networkingv1.NetworkPolicyEgressRule{
					Ports: []networkingv1.NetworkPolicyPort{},
					To: []networkingv1.NetworkPolicyPeer{
						networkingv1.NetworkPolicyPeer{
							PodSelector: oldEgressPodSelector,
						},
					},
				},
			},
			PolicyTypes: []networkingv1.PolicyType{
				networkingv1.PolicyTypeEgress,
			},
		},
	}

	deductedPolicy, err := deductPolicy(&oldPolicy, &newPolicy)
	if err != nil || !reflect.DeepEqual(deductedPolicy, expectedPolicy) {
		t.Errorf("TestMergePolicy failed")
		fmt.Println(deductedPolicy)
		fmt.Println(expectedPolicy)
	}
}
