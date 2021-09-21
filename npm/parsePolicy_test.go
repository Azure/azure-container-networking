package npm

import (
	"reflect"
	"testing"

	v1 "k8s.io/api/core/v1"
	networkingv1 "k8s.io/api/networking/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
)

func TestAddPolicy(t *testing.T) {
	tcp, udp := v1.ProtocolTCP, v1.ProtocolUDP
	port6783, port6784 := intstr.FromInt(6783), intstr.FromInt(6784)
	oldIngressPodSelector := &metav1.LabelSelector{
		MatchLabels: map[string]string{
			"app": "db",
		},
		MatchExpressions: []metav1.LabelSelectorRequirement{
			{
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
			{
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
			{
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
				{
					Ports: []networkingv1.NetworkPolicyPort{
						{
							Protocol: &tcp,
							Port:     &port6783,
						},
					},
					From: []networkingv1.NetworkPolicyPeer{
						{
							PodSelector: oldIngressPodSelector,
						},
						{
							PodSelector: oldIngressNamespaceSelector,
						},
					},
				},
			},
			Egress: []networkingv1.NetworkPolicyEgressRule{
				{
					Ports: []networkingv1.NetworkPolicyPort{},
					To: []networkingv1.NetworkPolicyPeer{
						{
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
			{
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
		t.Errorf("TestAddPolicy failed")
	}
}

func TestDeductPolicy(t *testing.T) {
	tcp := v1.ProtocolTCP
	port6783 := intstr.FromInt(6783)
	oldIngressPodSelector := &metav1.LabelSelector{
		MatchLabels: map[string]string{
			"app": "db",
		},
		MatchExpressions: []metav1.LabelSelectorRequirement{
			{
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
			{
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
			{
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
				{
					Ports: []networkingv1.NetworkPolicyPort{
						{
							Protocol: &tcp,
							Port:     &port6783,
						},
					},
					From: []networkingv1.NetworkPolicyPeer{
						{
							PodSelector: oldIngressPodSelector,
						},
						{
							PodSelector: oldIngressNamespaceSelector,
						},
					},
				},
			},
			Egress: []networkingv1.NetworkPolicyEgressRule{
				{
					Ports: []networkingv1.NetworkPolicyPort{},
					To: []networkingv1.NetworkPolicyPeer{
						{
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

	newPolicy := networkingv1.NetworkPolicy{
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
				{
					Ports: []networkingv1.NetworkPolicyPort{
						{
							Protocol: &tcp,
							Port:     &port6783,
						},
					},
					From: []networkingv1.NetworkPolicyPeer{
						{
							PodSelector: oldIngressPodSelector,
						},
					},
				},
			},
			Egress: []networkingv1.NetworkPolicyEgressRule{
				{
					Ports: []networkingv1.NetworkPolicyPort{},
					To: []networkingv1.NetworkPolicyPeer{
						{
							PodSelector: &metav1.LabelSelector{},
						},
					},
				},
			},
			PolicyTypes: []networkingv1.PolicyType{
				networkingv1.PolicyTypeEgress,
			},
		},
	}

	expectedPolicy := oldPolicy
	expectedPolicy.Spec.PolicyTypes = []networkingv1.PolicyType{
		networkingv1.PolicyTypeEgress,
	}
	deductedPolicy, err := deductPolicy(&oldPolicy, &newPolicy)
	if err != nil || !reflect.DeepEqual(deductedPolicy, &expectedPolicy) {
		t.Errorf(
			"TestDeductPolicy failed.\n"+
				"Expected Policy: %v\n"+
				"DeductedPolicy: %v\n",
			&expectedPolicy,
			deductedPolicy,
		)
	}

	newPolicy = oldPolicy
	deductedPolicy, err = deductPolicy(&oldPolicy, &newPolicy)
	if err != nil || deductedPolicy != nil {
		t.Errorf(
			"TestDeductPolicy failed.\n"+
				"Expected Policy: %v\n"+
				"DeductedPolicy: %v\n",
			&expectedPolicy,
			deductedPolicy,
		)
	}

	newPolicy = networkingv1.NetworkPolicy{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: "testnamespace",
		},
		Spec: networkingv1.NetworkPolicySpec{
			PodSelector: metav1.LabelSelector{
				MatchLabels: map[string]string{
					"role": "client",
				},
			},
			Egress: []networkingv1.NetworkPolicyEgressRule{
				{
					Ports: []networkingv1.NetworkPolicyPort{},
					To: []networkingv1.NetworkPolicyPeer{
						{
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

	expectedPolicy = oldPolicy
	expectedPolicy.Spec.Egress = []networkingv1.NetworkPolicyEgressRule{}
	expectedPolicy.Spec.PolicyTypes = []networkingv1.PolicyType{
		networkingv1.PolicyTypeEgress,
	}
	deductedPolicy, err = deductPolicy(&oldPolicy, &newPolicy)
	if err != nil || !reflect.DeepEqual(deductedPolicy, &expectedPolicy) {
		t.Errorf(
			"TestDeductPolicy failed.\n"+
				"Expected Policy: %v\n"+
				"DeductedPolicy: %v\n",
			&expectedPolicy,
			deductedPolicy,
		)
	}
}
