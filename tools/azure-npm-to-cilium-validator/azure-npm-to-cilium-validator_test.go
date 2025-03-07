package main

import (
	"testing"

	corev1 "k8s.io/api/core/v1"
	networkingv1 "k8s.io/api/networking/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	intstr "k8s.io/apimachinery/pkg/util/intstr"
)

// Test function for getEndportNetworkPolicies
func TestGetEndportNetworkPolicies(t *testing.T) {
	tests := []struct {
		name                           string
		policiesByNamespace            map[string][]*networkingv1.NetworkPolicy
		expectedIngressEndportPolicies []string
		expectedEgressEndportPolicies  []string
	}{
		{
			name:                           "No policies",
			policiesByNamespace:            map[string][]*networkingv1.NetworkPolicy{},
			expectedIngressEndportPolicies: []string{},
			expectedEgressEndportPolicies:  []string{},
		},
		{
			name: "No endport in policies",
			policiesByNamespace: map[string][]*networkingv1.NetworkPolicy{
				"namespace1": {
					{
						ObjectMeta: metav1.ObjectMeta{Name: "no-endport-policy"},
						Spec: networkingv1.NetworkPolicySpec{
							Ingress: []networkingv1.NetworkPolicyIngressRule{
								{
									Ports: []networkingv1.NetworkPolicyPort{
										{Port: intstrPtr(intstr.FromInt(80))},
									},
								},
							},
						},
					},
				},
			},
			expectedIngressEndportPolicies: []string{},
			expectedEgressEndportPolicies:  []string{},
		},
		{
			name: "Ingress endport in policy",
			policiesByNamespace: map[string][]*networkingv1.NetworkPolicy{
				"namespace1": {
					{
						ObjectMeta: metav1.ObjectMeta{Name: "ingress-endport-policy"},
						Spec: networkingv1.NetworkPolicySpec{
							Ingress: []networkingv1.NetworkPolicyIngressRule{
								{
									Ports: []networkingv1.NetworkPolicyPort{
										{Port: intstrPtr(intstr.FromInt(80)), EndPort: int32Ptr(90)},
									},
								},
							},
						},
					},
				},
			},
			expectedIngressEndportPolicies: []string{"namespace1/ingress-endport-policy"},
			expectedEgressEndportPolicies:  []string{},
		},
		{
			name: "Egress endport in policy",
			policiesByNamespace: map[string][]*networkingv1.NetworkPolicy{
				"namespace1": {
					{
						ObjectMeta: metav1.ObjectMeta{Name: "egress-endport-policy"},
						Spec: networkingv1.NetworkPolicySpec{
							Egress: []networkingv1.NetworkPolicyEgressRule{
								{
									Ports: []networkingv1.NetworkPolicyPort{
										{Port: intstrPtr(intstr.FromInt(80)), EndPort: int32Ptr(90)},
									},
								},
							},
						},
					},
				},
			},
			expectedIngressEndportPolicies: []string{},
			expectedEgressEndportPolicies:  []string{"namespace1/egress-endport-policy"},
		},
		{
			name: "Both ingress and egress endport in policy",
			policiesByNamespace: map[string][]*networkingv1.NetworkPolicy{
				"namespace1": {
					{
						ObjectMeta: metav1.ObjectMeta{Name: "ingress-and-egress-endport-policy"},
						Spec: networkingv1.NetworkPolicySpec{
							Ingress: []networkingv1.NetworkPolicyIngressRule{
								{
									Ports: []networkingv1.NetworkPolicyPort{
										{Port: intstrPtr(intstr.FromInt(80)), EndPort: int32Ptr(90)},
									},
								},
							},
							Egress: []networkingv1.NetworkPolicyEgressRule{
								{
									Ports: []networkingv1.NetworkPolicyPort{
										{Port: intstrPtr(intstr.FromInt(80)), EndPort: int32Ptr(90)},
									},
								},
							},
						},
					},
				},
			},
			expectedIngressEndportPolicies: []string{"namespace1/ingress-and-egress-endport-policy"},
			expectedEgressEndportPolicies:  []string{"namespace1/ingress-and-egress-endport-policy"},
		},
		{
			name: "Multiple polices in a namespace with ingress or egress endport",
			policiesByNamespace: map[string][]*networkingv1.NetworkPolicy{
				"namespace1": {
					{
						ObjectMeta: metav1.ObjectMeta{Name: "ingress-endport-policy"},
						Spec: networkingv1.NetworkPolicySpec{
							Ingress: []networkingv1.NetworkPolicyIngressRule{
								{
									Ports: []networkingv1.NetworkPolicyPort{
										{Port: intstrPtr(intstr.FromInt(80)), EndPort: int32Ptr(90)},
									},
								},
							},
						},
					},
					{
						ObjectMeta: metav1.ObjectMeta{Name: "egress-endport-policy"},
						Spec: networkingv1.NetworkPolicySpec{
							Egress: []networkingv1.NetworkPolicyEgressRule{
								{
									Ports: []networkingv1.NetworkPolicyPort{
										{Port: intstrPtr(intstr.FromInt(80)), EndPort: int32Ptr(90)},
									},
								},
							},
						},
					},
					{
						ObjectMeta: metav1.ObjectMeta{Name: "ingress-and-egress-endport-policy"},
						Spec: networkingv1.NetworkPolicySpec{
							Ingress: []networkingv1.NetworkPolicyIngressRule{
								{
									Ports: []networkingv1.NetworkPolicyPort{
										{Port: intstrPtr(intstr.FromInt(80)), EndPort: int32Ptr(90)},
									},
								},
							},
							Egress: []networkingv1.NetworkPolicyEgressRule{
								{
									Ports: []networkingv1.NetworkPolicyPort{
										{Port: intstrPtr(intstr.FromInt(80)), EndPort: int32Ptr(90)},
									},
								},
							},
						},
					},
				},
			},
			expectedIngressEndportPolicies: []string{"namespace1/ingress-endport-policy", "namespace1/ingress-and-egress-endport-policy"},
			expectedEgressEndportPolicies:  []string{"namespace1/egress-endport-policy", "namespace1/ingress-and-egress-endport-policy"},
		},
		{
			name: "Multiple polices in multiple namespaces with ingress or egress endport or no endport",
			policiesByNamespace: map[string][]*networkingv1.NetworkPolicy{
				"namespace1": {
					{
						ObjectMeta: metav1.ObjectMeta{Name: "ingress-endport-policy"},
						Spec: networkingv1.NetworkPolicySpec{
							Ingress: []networkingv1.NetworkPolicyIngressRule{
								{
									Ports: []networkingv1.NetworkPolicyPort{
										{Port: intstrPtr(intstr.FromInt(80)), EndPort: int32Ptr(90)},
									},
								},
							},
						},
					},
					{
						ObjectMeta: metav1.ObjectMeta{Name: "ingress-and-egress-endport-policy"},
						Spec: networkingv1.NetworkPolicySpec{
							Ingress: []networkingv1.NetworkPolicyIngressRule{
								{
									Ports: []networkingv1.NetworkPolicyPort{
										{Port: intstrPtr(intstr.FromInt(80)), EndPort: int32Ptr(90)},
									},
								},
							},
							Egress: []networkingv1.NetworkPolicyEgressRule{
								{
									Ports: []networkingv1.NetworkPolicyPort{
										{Port: intstrPtr(intstr.FromInt(80)), EndPort: int32Ptr(90)},
									},
								},
							},
						},
					},
				},
				"namespace2": {
					{
						ObjectMeta: metav1.ObjectMeta{Name: "egress-endport-policy"},
						Spec: networkingv1.NetworkPolicySpec{
							Egress: []networkingv1.NetworkPolicyEgressRule{
								{
									Ports: []networkingv1.NetworkPolicyPort{
										{Port: intstrPtr(intstr.FromInt(80)), EndPort: int32Ptr(90)},
									},
								},
							},
						},
					},
					{
						ObjectMeta: metav1.ObjectMeta{Name: "no-endport-policy"},
						Spec: networkingv1.NetworkPolicySpec{
							Ingress: []networkingv1.NetworkPolicyIngressRule{
								{
									Ports: []networkingv1.NetworkPolicyPort{
										{Port: intstrPtr(intstr.FromInt(80))},
									},
								},
							},
						},
					},
				},
				"namespace3": {
					{
						ObjectMeta: metav1.ObjectMeta{Name: "no-endport-policy"},
						Spec: networkingv1.NetworkPolicySpec{
							Ingress: []networkingv1.NetworkPolicyIngressRule{
								{
									Ports: []networkingv1.NetworkPolicyPort{
										{Port: intstrPtr(intstr.FromInt(80))},
									},
								},
							},
						},
					},
				},
			},
			expectedIngressEndportPolicies: []string{"namespace1/ingress-endport-policy", "namespace1/ingress-and-egress-endport-policy"},
			expectedEgressEndportPolicies:  []string{"namespace1/ingress-and-egress-endport-policy", "namespace2/egress-endport-policy"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ingressPolicies, egressPolicies := getEndportNetworkPolicies(tt.policiesByNamespace)
			if !equal(ingressPolicies, tt.expectedIngressEndportPolicies) {
				t.Errorf("expected ingress policies %v, got %v", tt.expectedIngressEndportPolicies, ingressPolicies)
			}
			if !equal(egressPolicies, tt.expectedEgressEndportPolicies) {
				t.Errorf("expected egress policies %v, got %v", tt.expectedEgressEndportPolicies, egressPolicies)
			}
		})
	}
}

func TestGetCIDRNetworkPolicies(t *testing.T) {
	tests := []struct {
		name                        string
		policiesByNamespace         map[string][]*networkingv1.NetworkPolicy
		expectedIngressCIDRPolicies []string
		expectedEgressCIDRPolicies  []string
	}{
		{
			name:                        "No policies",
			policiesByNamespace:         map[string][]*networkingv1.NetworkPolicy{},
			expectedIngressCIDRPolicies: []string{},
			expectedEgressCIDRPolicies:  []string{},
		},
		{
			name: "No CIDR in policies",
			policiesByNamespace: map[string][]*networkingv1.NetworkPolicy{
				"namespace1": {
					{
						ObjectMeta: metav1.ObjectMeta{Name: "no-cidr-policy"},
						Spec: networkingv1.NetworkPolicySpec{
							Ingress: []networkingv1.NetworkPolicyIngressRule{
								{
									From: []networkingv1.NetworkPolicyPeer{
										{PodSelector: &metav1.LabelSelector{}},
									},
								},
							},
						},
					},
				},
			},
			expectedIngressCIDRPolicies: []string{},
			expectedEgressCIDRPolicies:  []string{},
		},
		{
			name: "Ingress CIDR in policy",
			policiesByNamespace: map[string][]*networkingv1.NetworkPolicy{
				"namespace1": {
					{
						ObjectMeta: metav1.ObjectMeta{Name: "ingress-cidr-policy"},
						Spec: networkingv1.NetworkPolicySpec{
							Ingress: []networkingv1.NetworkPolicyIngressRule{
								{
									From: []networkingv1.NetworkPolicyPeer{
										{IPBlock: &networkingv1.IPBlock{CIDR: "192.168.0.0/16"}},
									},
								},
							},
						},
					},
				},
			},
			expectedIngressCIDRPolicies: []string{"namespace1/ingress-cidr-policy"},
			expectedEgressCIDRPolicies:  []string{},
		},
		{
			name: "Egress CIDR in policy",
			policiesByNamespace: map[string][]*networkingv1.NetworkPolicy{
				"namespace1": {
					{
						ObjectMeta: metav1.ObjectMeta{Name: "egress-cidr-policy"},
						Spec: networkingv1.NetworkPolicySpec{
							Egress: []networkingv1.NetworkPolicyEgressRule{
								{
									To: []networkingv1.NetworkPolicyPeer{
										{IPBlock: &networkingv1.IPBlock{CIDR: "192.168.0.0/16"}},
									},
								},
							},
						},
					},
				},
			},
			expectedIngressCIDRPolicies: []string{},
			expectedEgressCIDRPolicies:  []string{"namespace1/egress-cidr-policy"},
		},
		{
			name: "Both ingress and egress CIDR in policy",
			policiesByNamespace: map[string][]*networkingv1.NetworkPolicy{
				"namespace1": {
					{
						ObjectMeta: metav1.ObjectMeta{Name: "ingress-and-egress-cidr-policy"},
						Spec: networkingv1.NetworkPolicySpec{
							Ingress: []networkingv1.NetworkPolicyIngressRule{
								{
									From: []networkingv1.NetworkPolicyPeer{
										{IPBlock: &networkingv1.IPBlock{CIDR: "192.168.0.0/16"}},
									},
								},
							},
							Egress: []networkingv1.NetworkPolicyEgressRule{
								{
									To: []networkingv1.NetworkPolicyPeer{
										{IPBlock: &networkingv1.IPBlock{CIDR: "192.168.0.0/16"}},
									},
								},
							},
						},
					},
				},
			},
			expectedIngressCIDRPolicies: []string{"namespace1/ingress-and-egress-cidr-policy"},
			expectedEgressCIDRPolicies:  []string{"namespace1/ingress-and-egress-cidr-policy"},
		},
		{
			name: "Multiple polices in a namespace with ingress or egress CIDR",
			policiesByNamespace: map[string][]*networkingv1.NetworkPolicy{
				"namespace1": {
					{
						ObjectMeta: metav1.ObjectMeta{Name: "ingress-cidr-policy"},
						Spec: networkingv1.NetworkPolicySpec{
							Ingress: []networkingv1.NetworkPolicyIngressRule{
								{
									From: []networkingv1.NetworkPolicyPeer{
										{IPBlock: &networkingv1.IPBlock{CIDR: "192.168.0.0/16"}},
									},
								},
							},
						},
					},
					{
						ObjectMeta: metav1.ObjectMeta{Name: "egress-cidr-policy"},
						Spec: networkingv1.NetworkPolicySpec{
							Egress: []networkingv1.NetworkPolicyEgressRule{
								{
									To: []networkingv1.NetworkPolicyPeer{
										{IPBlock: &networkingv1.IPBlock{CIDR: "192.168.0.0/16"}},
									},
								},
							},
						},
					},
					{
						ObjectMeta: metav1.ObjectMeta{Name: "ingress-and-egress-cidr-policy"},
						Spec: networkingv1.NetworkPolicySpec{
							Ingress: []networkingv1.NetworkPolicyIngressRule{
								{
									From: []networkingv1.NetworkPolicyPeer{
										{IPBlock: &networkingv1.IPBlock{CIDR: "192.168.0.0/16"}},
									},
								},
							},
							Egress: []networkingv1.NetworkPolicyEgressRule{
								{
									To: []networkingv1.NetworkPolicyPeer{
										{IPBlock: &networkingv1.IPBlock{CIDR: "192.168.0.0/16"}},
									},
								},
							},
						},
					},
				},
			},
			expectedIngressCIDRPolicies: []string{"namespace1/ingress-cidr-policy", "namespace1/ingress-and-egress-cidr-policy"},
			expectedEgressCIDRPolicies:  []string{"namespace1/egress-cidr-policy", "namespace1/ingress-and-egress-cidr-policy"},
		},
		{
			name: "Multiple polices in multiple namespaces with ingress or egress CIDR or no CIDR",
			policiesByNamespace: map[string][]*networkingv1.NetworkPolicy{
				"namespace1": {
					{
						ObjectMeta: metav1.ObjectMeta{Name: "ingress-cidr-policy"},
						Spec: networkingv1.NetworkPolicySpec{
							Ingress: []networkingv1.NetworkPolicyIngressRule{
								{
									From: []networkingv1.NetworkPolicyPeer{
										{IPBlock: &networkingv1.IPBlock{CIDR: "192.168.0.0/16"}},
									},
								},
							},
						},
					},
					{
						ObjectMeta: metav1.ObjectMeta{Name: "ingress-and-egress-cidr-policy"},
						Spec: networkingv1.NetworkPolicySpec{
							Ingress: []networkingv1.NetworkPolicyIngressRule{
								{
									From: []networkingv1.NetworkPolicyPeer{
										{IPBlock: &networkingv1.IPBlock{CIDR: "192.168.0.0/16"}},
									},
								},
							},
							Egress: []networkingv1.NetworkPolicyEgressRule{
								{
									To: []networkingv1.NetworkPolicyPeer{
										{IPBlock: &networkingv1.IPBlock{CIDR: "192.168.0.0/16"}},
									},
								},
							},
						},
					},
				},
				"namespace2": {
					{
						ObjectMeta: metav1.ObjectMeta{Name: "egress-cidr-policy"},
						Spec: networkingv1.NetworkPolicySpec{
							Egress: []networkingv1.NetworkPolicyEgressRule{
								{
									To: []networkingv1.NetworkPolicyPeer{
										{IPBlock: &networkingv1.IPBlock{CIDR: "10.0.0.0/8"}},
									},
								},
							},
						},
					},
					{
						ObjectMeta: metav1.ObjectMeta{Name: "no-cidr-policy"},
						Spec: networkingv1.NetworkPolicySpec{
							Ingress: []networkingv1.NetworkPolicyIngressRule{
								{
									From: []networkingv1.NetworkPolicyPeer{
										{PodSelector: &metav1.LabelSelector{}},
									},
								},
							},
						},
					},
				},
				"namespace3": {
					{
						ObjectMeta: metav1.ObjectMeta{Name: "no-cidr-policy"},
						Spec: networkingv1.NetworkPolicySpec{
							Ingress: []networkingv1.NetworkPolicyIngressRule{
								{
									From: []networkingv1.NetworkPolicyPeer{
										{PodSelector: &metav1.LabelSelector{}},
									},
								},
							},
						},
					},
				},
			},
			expectedIngressCIDRPolicies: []string{"namespace1/ingress-cidr-policy", "namespace1/ingress-and-egress-cidr-policy"},
			expectedEgressCIDRPolicies:  []string{"namespace2/egress-cidr-policy", "namespace1/ingress-and-egress-cidr-policy"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ingressPolicies, egressPolicies := getCIDRNetworkPolicies(tt.policiesByNamespace)
			if !equal(ingressPolicies, tt.expectedIngressCIDRPolicies) {
				t.Errorf("expected ingress policies %v, got %v", tt.expectedIngressCIDRPolicies, ingressPolicies)
			}
			if !equal(egressPolicies, tt.expectedEgressCIDRPolicies) {
				t.Errorf("expected egress policies %v, got %v", tt.expectedEgressCIDRPolicies, egressPolicies)
			}
		})
	}
}

func TestGetNamedPortPolicies(t *testing.T) {
	tests := []struct {
		name                        string
		policiesByNamespace         map[string][]*networkingv1.NetworkPolicy
		expectedIngressCIDRPolicies []string
		expectedEgressCIDRPolicies  []string
	}{
		{
			name:                        "No policies",
			policiesByNamespace:         map[string][]*networkingv1.NetworkPolicy{},
			expectedIngressCIDRPolicies: []string{},
			expectedEgressCIDRPolicies:  []string{},
		},
		{
			name: "No named port in policies",
			policiesByNamespace: map[string][]*networkingv1.NetworkPolicy{
				"namespace1": {
					{
						ObjectMeta: metav1.ObjectMeta{Name: "no-cidr-policy"},
						Spec: networkingv1.NetworkPolicySpec{
							Ingress: []networkingv1.NetworkPolicyIngressRule{
								{
									From: []networkingv1.NetworkPolicyPeer{
										{PodSelector: &metav1.LabelSelector{}},
									},
								},
							},
						},
					},
				},
			},
			expectedIngressCIDRPolicies: []string{},
			expectedEgressCIDRPolicies:  []string{},
		},
		{
			name: "Ingress named port in policy",
			policiesByNamespace: map[string][]*networkingv1.NetworkPolicy{
				"namespace1": {
					{
						ObjectMeta: metav1.ObjectMeta{Name: "ingress-named-port-policy"},
						Spec: networkingv1.NetworkPolicySpec{
							Ingress: []networkingv1.NetworkPolicyIngressRule{
								{
									Ports: []networkingv1.NetworkPolicyPort{
										{
											Port: intstrPtr(intstr.FromString("http")),
											Protocol: func() *corev1.Protocol {
												protocol := corev1.ProtocolTCP
												return &protocol
											}(),
										},
									},
								},
							},
						},
					},
				},
			},
			expectedIngressCIDRPolicies: []string{"namespace1/ingress-named-port-policy"},
			expectedEgressCIDRPolicies:  []string{},
		},
		{
			name: "Ingress int port in policy",
			policiesByNamespace: map[string][]*networkingv1.NetworkPolicy{
				"namespace1": {
					{
						ObjectMeta: metav1.ObjectMeta{Name: "ingress-int-port-policy"},
						Spec: networkingv1.NetworkPolicySpec{
							Ingress: []networkingv1.NetworkPolicyIngressRule{
								{
									Ports: []networkingv1.NetworkPolicyPort{
										{
											Port: intstrPtr(intstr.FromInt(80)),
											Protocol: func() *corev1.Protocol {
												protocol := corev1.ProtocolTCP
												return &protocol
											}(),
										},
									},
								},
							},
						},
					},
				},
			},
			expectedIngressCIDRPolicies: []string{},
			expectedEgressCIDRPolicies:  []string{},
		},
		{
			name: "Egress named port in policy",
			policiesByNamespace: map[string][]*networkingv1.NetworkPolicy{
				"namespace1": {
					{
						ObjectMeta: metav1.ObjectMeta{Name: "egress-named-port-policy"},
						Spec: networkingv1.NetworkPolicySpec{
							Egress: []networkingv1.NetworkPolicyEgressRule{
								{
									Ports: []networkingv1.NetworkPolicyPort{
										{
											Port: intstrPtr(intstr.FromString("http")),
											Protocol: func() *corev1.Protocol {
												protocol := corev1.ProtocolTCP
												return &protocol
											}(),
										},
									},
								},
							},
						},
					},
				},
			},
			expectedIngressCIDRPolicies: []string{},
			expectedEgressCIDRPolicies:  []string{"namespace1/egress-named-port-policy"},
		},
		{
			name: "Egress int port in policy",
			policiesByNamespace: map[string][]*networkingv1.NetworkPolicy{
				"namespace1": {
					{
						ObjectMeta: metav1.ObjectMeta{Name: "egress-int-port-policy"},
						Spec: networkingv1.NetworkPolicySpec{
							Egress: []networkingv1.NetworkPolicyEgressRule{
								{
									Ports: []networkingv1.NetworkPolicyPort{
										{
											Port: intstrPtr(intstr.FromInt(80)),
											Protocol: func() *corev1.Protocol {
												protocol := corev1.ProtocolTCP
												return &protocol
											}(),
										},
									},
								},
							},
						},
					},
				},
			},
			expectedIngressCIDRPolicies: []string{},
			expectedEgressCIDRPolicies:  []string{},
		},
		{
			name: "Both ingress and egress name ports in policy",
			policiesByNamespace: map[string][]*networkingv1.NetworkPolicy{
				"namespace1": {
					{
						ObjectMeta: metav1.ObjectMeta{Name: "ingress-and-egress-named-port-policy"},
						Spec: networkingv1.NetworkPolicySpec{
							Ingress: []networkingv1.NetworkPolicyIngressRule{
								{
									Ports: []networkingv1.NetworkPolicyPort{
										{
											Port: intstrPtr(intstr.FromString("http")),
											Protocol: func() *corev1.Protocol {
												protocol := corev1.ProtocolTCP
												return &protocol
											}(),
										},
									},
								},
							},
							Egress: []networkingv1.NetworkPolicyEgressRule{
								{
									Ports: []networkingv1.NetworkPolicyPort{
										{
											Port: intstrPtr(intstr.FromString("http")),
											Protocol: func() *corev1.Protocol {
												protocol := corev1.ProtocolTCP
												return &protocol
											}(),
										},
									},
								},
							},
						},
					},
				},
			},
			expectedIngressCIDRPolicies: []string{"namespace1/ingress-and-egress-named-port-policy"},
			expectedEgressCIDRPolicies:  []string{"namespace1/ingress-and-egress-named-port-policy"},
		},
		{
			name: "Multiple polices in a namespace with ingress or egress named ports",
			policiesByNamespace: map[string][]*networkingv1.NetworkPolicy{
				"namespace1": {
					{
						ObjectMeta: metav1.ObjectMeta{Name: "ingress-named-port-policy"},
						Spec: networkingv1.NetworkPolicySpec{
							Ingress: []networkingv1.NetworkPolicyIngressRule{
								{
									Ports: []networkingv1.NetworkPolicyPort{
										{
											Port: intstrPtr(intstr.FromString("http")),
											Protocol: func() *corev1.Protocol {
												protocol := corev1.ProtocolTCP
												return &protocol
											}(),
										},
									},
								},
							},
						},
					},
					{
						ObjectMeta: metav1.ObjectMeta{Name: "egress-named-port-policy"},
						Spec: networkingv1.NetworkPolicySpec{
							Egress: []networkingv1.NetworkPolicyEgressRule{
								{
									Ports: []networkingv1.NetworkPolicyPort{
										{
											Port: intstrPtr(intstr.FromString("http")),
											Protocol: func() *corev1.Protocol {
												protocol := corev1.ProtocolTCP
												return &protocol
											}(),
										},
									},
								},
							},
						},
					},
					{
						ObjectMeta: metav1.ObjectMeta{Name: "ingress-and-egress-named-port-policy"},
						Spec: networkingv1.NetworkPolicySpec{
							Ingress: []networkingv1.NetworkPolicyIngressRule{
								{
									Ports: []networkingv1.NetworkPolicyPort{
										{
											Port: intstrPtr(intstr.FromString("http")),
											Protocol: func() *corev1.Protocol {
												protocol := corev1.ProtocolTCP
												return &protocol
											}(),
										},
									},
								},
							},
							Egress: []networkingv1.NetworkPolicyEgressRule{
								{
									Ports: []networkingv1.NetworkPolicyPort{
										{
											Port: intstrPtr(intstr.FromString("http")),
											Protocol: func() *corev1.Protocol {
												protocol := corev1.ProtocolTCP
												return &protocol
											}(),
										},
									},
								},
							},
						},
					},
				},
			},
			expectedIngressCIDRPolicies: []string{"namespace1/ingress-named-port-policy", "namespace1/ingress-and-egress-named-port-policy"},
			expectedEgressCIDRPolicies:  []string{"namespace1/egress-named-port-policy", "namespace1/ingress-and-egress-named-port-policy"},
		},
		{
			name: "Multiple polices in multiple namespaces with ingress or egress CIDR or no CIDR",
			policiesByNamespace: map[string][]*networkingv1.NetworkPolicy{
				"namespace1": {
					{
						ObjectMeta: metav1.ObjectMeta{Name: "ingress-named-port-policy"},
						Spec: networkingv1.NetworkPolicySpec{
							Ingress: []networkingv1.NetworkPolicyIngressRule{
								{
									Ports: []networkingv1.NetworkPolicyPort{
										{
											Port: intstrPtr(intstr.FromString("http")),
											Protocol: func() *corev1.Protocol {
												protocol := corev1.ProtocolTCP
												return &protocol
											}(),
										},
									},
								},
							},
						},
					},
					{
						ObjectMeta: metav1.ObjectMeta{Name: "ingress-and-egress-named-port-policy"},
						Spec: networkingv1.NetworkPolicySpec{
							Ingress: []networkingv1.NetworkPolicyIngressRule{
								{
									Ports: []networkingv1.NetworkPolicyPort{
										{
											Port: intstrPtr(intstr.FromString("http")),
											Protocol: func() *corev1.Protocol {
												protocol := corev1.ProtocolTCP
												return &protocol
											}(),
										},
									},
								},
							},
							Egress: []networkingv1.NetworkPolicyEgressRule{
								{
									Ports: []networkingv1.NetworkPolicyPort{
										{
											Port: intstrPtr(intstr.FromString("http")),
											Protocol: func() *corev1.Protocol {
												protocol := corev1.ProtocolTCP
												return &protocol
											}(),
										},
									},
								},
							},
						},
					},
					{
						ObjectMeta: metav1.ObjectMeta{Name: "egress-int-port-policy"},
						Spec: networkingv1.NetworkPolicySpec{
							Egress: []networkingv1.NetworkPolicyEgressRule{
								{
									Ports: []networkingv1.NetworkPolicyPort{
										{
											Port: intstrPtr(intstr.FromInt(80)),
											Protocol: func() *corev1.Protocol {
												protocol := corev1.ProtocolTCP
												return &protocol
											}(),
										},
									},
								},
							},
						},
					},
				},
				"namespace2": {
					{
						ObjectMeta: metav1.ObjectMeta{Name: "egress-named-port-policy"},
						Spec: networkingv1.NetworkPolicySpec{
							Egress: []networkingv1.NetworkPolicyEgressRule{
								{
									Ports: []networkingv1.NetworkPolicyPort{
										{
											Port: intstrPtr(intstr.FromString("http")),
											Protocol: func() *corev1.Protocol {
												protocol := corev1.ProtocolTCP
												return &protocol
											}(),
										},
									},
								},
							},
						},
					},
					{
						ObjectMeta: metav1.ObjectMeta{Name: "no-named-port-policy"},
						Spec: networkingv1.NetworkPolicySpec{
							Ingress: []networkingv1.NetworkPolicyIngressRule{
								{
									From: []networkingv1.NetworkPolicyPeer{
										{PodSelector: &metav1.LabelSelector{}},
									},
								},
							},
						},
					},
				},
				"namespace3": {
					{
						ObjectMeta: metav1.ObjectMeta{Name: "no-named-port-policy"},
						Spec: networkingv1.NetworkPolicySpec{
							Ingress: []networkingv1.NetworkPolicyIngressRule{
								{
									From: []networkingv1.NetworkPolicyPeer{
										{PodSelector: &metav1.LabelSelector{}},
									},
								},
							},
						},
					},
					{
						ObjectMeta: metav1.ObjectMeta{Name: "ingress-int-port-policy"},
						Spec: networkingv1.NetworkPolicySpec{
							Ingress: []networkingv1.NetworkPolicyIngressRule{
								{
									Ports: []networkingv1.NetworkPolicyPort{
										{
											Port: intstrPtr(intstr.FromInt(80)),
											Protocol: func() *corev1.Protocol {
												protocol := corev1.ProtocolTCP
												return &protocol
											}(),
										},
									},
								},
							},
						},
					},
				},
			},
			expectedIngressCIDRPolicies: []string{"namespace1/ingress-named-port-policy", "namespace1/ingress-and-egress-named-port-policy"},
			expectedEgressCIDRPolicies:  []string{"namespace2/egress-named-port-policy", "namespace1/ingress-and-egress-named-port-policy"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ingressPolicies, egressPolicies := getNamedPortPolicies(tt.policiesByNamespace)
			if !equal(ingressPolicies, tt.expectedIngressCIDRPolicies) {
				t.Errorf("expected ingress policies %v, got %v", tt.expectedIngressCIDRPolicies, ingressPolicies)
			}
			if !equal(egressPolicies, tt.expectedEgressCIDRPolicies) {
				t.Errorf("expected egress policies %v, got %v", tt.expectedEgressCIDRPolicies, egressPolicies)
			}
		})
	}
}

func TestGetEgressPolicies(t *testing.T) {
	tests := []struct {
		name                   string
		policiesByNamespace    map[string][]*networkingv1.NetworkPolicy
		expectedEgressPolicies []string
	}{
		{
			name:                   "No policies",
			policiesByNamespace:    map[string][]*networkingv1.NetworkPolicy{},
			expectedEgressPolicies: []string{},
		},
		{
			name: "No egress in policies",
			policiesByNamespace: map[string][]*networkingv1.NetworkPolicy{
				"namespace1": {
					{
						ObjectMeta: metav1.ObjectMeta{Name: "no-egress-policy"},
						Spec: networkingv1.NetworkPolicySpec{
							Ingress: []networkingv1.NetworkPolicyIngressRule{
								{
									From: []networkingv1.NetworkPolicyPeer{
										{PodSelector: &metav1.LabelSelector{}},
									},
								},
							},
						},
					},
				},
			},
			expectedEgressPolicies: []string{},
		},
		{
			name: "Allow all egress policy",
			policiesByNamespace: map[string][]*networkingv1.NetworkPolicy{
				"namespace1": {
					{
						ObjectMeta: metav1.ObjectMeta{Name: "allow-all-egress-policy"},
						Spec: networkingv1.NetworkPolicySpec{
							PolicyTypes: []networkingv1.PolicyType{"Egress"},
							Egress: []networkingv1.NetworkPolicyEgressRule{
								{},
							},
						},
					},
				},
			},
			expectedEgressPolicies: []string{},
		},
		{
			name: "Deny all egress policy",
			policiesByNamespace: map[string][]*networkingv1.NetworkPolicy{
				"namespace1": {
					{
						ObjectMeta: metav1.ObjectMeta{Name: "deny-all-egress-policy"},
						Spec: networkingv1.NetworkPolicySpec{
							PolicyTypes: []networkingv1.PolicyType{"Egress"},
						},
					},
				},
			},
			expectedEgressPolicies: []string{"namespace1/deny-all-egress-policy"},
		},
		{
			name: "Egress policy with To field",
			policiesByNamespace: map[string][]*networkingv1.NetworkPolicy{
				"namespace1": {
					{
						ObjectMeta: metav1.ObjectMeta{Name: "egress-to-policy"},
						Spec: networkingv1.NetworkPolicySpec{
							Egress: []networkingv1.NetworkPolicyEgressRule{
								{
									To: []networkingv1.NetworkPolicyPeer{
										{PodSelector: &metav1.LabelSelector{}},
									},
								},
							},
						},
					},
				},
			},
			expectedEgressPolicies: []string{"namespace1/egress-to-policy"},
		},
		{
			name: "Egress policy with Ports field",
			policiesByNamespace: map[string][]*networkingv1.NetworkPolicy{
				"namespace1": {
					{
						ObjectMeta: metav1.ObjectMeta{Name: "egress-ports-policy"},
						Spec: networkingv1.NetworkPolicySpec{
							Egress: []networkingv1.NetworkPolicyEgressRule{
								{
									Ports: []networkingv1.NetworkPolicyPort{
										{Port: intstrPtr(intstr.FromInt(80))},
									},
								},
							},
						},
					},
				},
			},
			expectedEgressPolicies: []string{"namespace1/egress-ports-policy"},
		},
		{
			name: "Egress policy with both To and Ports fields",
			policiesByNamespace: map[string][]*networkingv1.NetworkPolicy{
				"namespace1": {
					{
						ObjectMeta: metav1.ObjectMeta{Name: "egress-to-and-ports-policy"},
						Spec: networkingv1.NetworkPolicySpec{
							Egress: []networkingv1.NetworkPolicyEgressRule{
								{
									To: []networkingv1.NetworkPolicyPeer{
										{PodSelector: &metav1.LabelSelector{}},
									},
									Ports: []networkingv1.NetworkPolicyPort{
										{Port: intstrPtr(intstr.FromInt(80))},
									},
								},
							},
						},
					},
				},
			},
			expectedEgressPolicies: []string{"namespace1/egress-to-and-ports-policy"},
		},
		{
			name: "Multiple egress polices in a namespace with To or Port fields",
			policiesByNamespace: map[string][]*networkingv1.NetworkPolicy{
				"namespace1": {
					{
						ObjectMeta: metav1.ObjectMeta{Name: "egress-to-policy"},
						Spec: networkingv1.NetworkPolicySpec{
							Egress: []networkingv1.NetworkPolicyEgressRule{
								{
									To: []networkingv1.NetworkPolicyPeer{
										{PodSelector: &metav1.LabelSelector{}},
									},
								},
							},
						},
					},
					{
						ObjectMeta: metav1.ObjectMeta{Name: "egress-ports-policy"},
						Spec: networkingv1.NetworkPolicySpec{
							Egress: []networkingv1.NetworkPolicyEgressRule{
								{
									Ports: []networkingv1.NetworkPolicyPort{
										{Port: intstrPtr(intstr.FromInt(80))},
									},
								},
							},
						},
					},
					{
						ObjectMeta: metav1.ObjectMeta{Name: "egress-to-and-ports-policy"},
						Spec: networkingv1.NetworkPolicySpec{
							Egress: []networkingv1.NetworkPolicyEgressRule{
								{
									To: []networkingv1.NetworkPolicyPeer{
										{PodSelector: &metav1.LabelSelector{}},
									},
									Ports: []networkingv1.NetworkPolicyPort{
										{Port: intstrPtr(intstr.FromInt(80))},
									},
								},
							},
						},
					},
				},
			},
			expectedEgressPolicies: []string{"namespace1/egress-to-policy", "namespace1/egress-ports-policy", "namespace1/egress-to-and-ports-policy"},
		},
		{
			name: "Multiple egresss polices in multiple namespaces with To or Port fields or no egress",
			policiesByNamespace: map[string][]*networkingv1.NetworkPolicy{
				"namespace1": {
					{
						ObjectMeta: metav1.ObjectMeta{Name: "egress-to-policy"},
						Spec: networkingv1.NetworkPolicySpec{
							Egress: []networkingv1.NetworkPolicyEgressRule{
								{
									To: []networkingv1.NetworkPolicyPeer{
										{PodSelector: &metav1.LabelSelector{}},
									},
								},
							},
						},
					},
					{
						ObjectMeta: metav1.ObjectMeta{Name: "egress-to-and-ports-policy"},
						Spec: networkingv1.NetworkPolicySpec{
							Egress: []networkingv1.NetworkPolicyEgressRule{
								{
									To: []networkingv1.NetworkPolicyPeer{
										{PodSelector: &metav1.LabelSelector{}},
									},
									Ports: []networkingv1.NetworkPolicyPort{
										{Port: intstrPtr(intstr.FromInt(80))},
									},
								},
							},
						},
					},
				},
				"namespace2": {
					{
						ObjectMeta: metav1.ObjectMeta{Name: "egress-ports-policy"},
						Spec: networkingv1.NetworkPolicySpec{
							Egress: []networkingv1.NetworkPolicyEgressRule{
								{
									Ports: []networkingv1.NetworkPolicyPort{
										{Port: intstrPtr(intstr.FromInt(80))},
									},
								},
							},
						},
					},
					{
						ObjectMeta: metav1.ObjectMeta{Name: "no-egress-policy"},
						Spec: networkingv1.NetworkPolicySpec{
							Ingress: []networkingv1.NetworkPolicyIngressRule{
								{
									From: []networkingv1.NetworkPolicyPeer{
										{PodSelector: &metav1.LabelSelector{}},
									},
								},
							},
						},
					},
				},
				"namespace3": {
					{
						ObjectMeta: metav1.ObjectMeta{Name: "egress-to-policy"},
						Spec: networkingv1.NetworkPolicySpec{
							Egress: []networkingv1.NetworkPolicyEgressRule{
								{
									To: []networkingv1.NetworkPolicyPeer{
										{PodSelector: &metav1.LabelSelector{}},
									},
								},
							},
						},
					},
					{
						ObjectMeta: metav1.ObjectMeta{Name: "allow-all-egress-policy"},
						Spec: networkingv1.NetworkPolicySpec{
							PolicyTypes: []networkingv1.PolicyType{"Egress"},
							Egress: []networkingv1.NetworkPolicyEgressRule{
								{},
							},
						},
					},
				},
				"namespace4": {
					{
						ObjectMeta: metav1.ObjectMeta{Name: "no-egress-policy"},
						Spec: networkingv1.NetworkPolicySpec{
							Ingress: []networkingv1.NetworkPolicyIngressRule{
								{
									From: []networkingv1.NetworkPolicyPeer{
										{PodSelector: &metav1.LabelSelector{}},
									},
								},
							},
						},
					},
				},
			},
			expectedEgressPolicies: []string{"namespace1/egress-to-policy", "namespace1/egress-to-and-ports-policy", "namespace2/egress-ports-policy", "namespace3/egress-to-policy"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			egressPolicies := getEgressPolicies(tt.policiesByNamespace)
			if !equal(egressPolicies, tt.expectedEgressPolicies) {
				t.Errorf("expected egress policies %v, got %v", tt.expectedEgressPolicies, egressPolicies)
			}
		})
	}
}

func TestGetExternalTrafficPolicyClusterServices(t *testing.T) {
	tests := []struct {
		name                   string
		namespaces             *corev1.NamespaceList
		servicesByNamespace    map[string][]*corev1.Service
		policiesByNamespace    map[string][]*networkingv1.NetworkPolicy
		expectedUnsafeServices []string
	}{
		// Scenarios where there are no LoadBalancer or NodePort services
		{
			name: "No namespaces",
			namespaces: &corev1.NamespaceList{
				Items: []corev1.Namespace{},
			},
			servicesByNamespace:    map[string][]*corev1.Service{},
			policiesByNamespace:    map[string][]*networkingv1.NetworkPolicy{},
			expectedUnsafeServices: []string{},
		},
		{
			name: "Namespace with no policies and services",
			namespaces: &corev1.NamespaceList{
				Items: []corev1.Namespace{
					{ObjectMeta: metav1.ObjectMeta{Name: "namespace1"}},
				},
			},
			servicesByNamespace: map[string][]*corev1.Service{
				"namespace1": {},
			},
			policiesByNamespace: map[string][]*networkingv1.NetworkPolicy{
				"namespace1": {},
			},
			expectedUnsafeServices: []string{},
		},
		// Scenarios where there are LoadBalancer or NodePort services but externalTrafficPolicy is not Cluster
		{
			name: "LoadBalancer service with externalTrafficPolicy=Local with no selector and a deny all ingress policy with no selector",
			namespaces: &corev1.NamespaceList{
				Items: []corev1.Namespace{
					{ObjectMeta: metav1.ObjectMeta{Name: "namespace1"}},
				},
			},
			servicesByNamespace: map[string][]*corev1.Service{
				"namespace1": {
					{
						ObjectMeta: metav1.ObjectMeta{Name: "service-with-no-selector"},
						Spec: corev1.ServiceSpec{
							Type:                  corev1.ServiceTypeLoadBalancer,
							ExternalTrafficPolicy: corev1.ServiceExternalTrafficPolicyTypeLocal,
						},
					},
				},
			},
			policiesByNamespace: map[string][]*networkingv1.NetworkPolicy{
				"namespace1": {
					{
						ObjectMeta: metav1.ObjectMeta{Name: "deny-all-ingress-policy-with-no-selector"},
						Spec: networkingv1.NetworkPolicySpec{
							PodSelector: metav1.LabelSelector{},
							PolicyTypes: []networkingv1.PolicyType{"Ingress"},
						},
					},
				},
			},
			expectedUnsafeServices: []string{},
		},
		{
			name: "NodePort service with externalTrafficPolicy=Local with no selector and a deny all ingress policy with no selector",
			namespaces: &corev1.NamespaceList{
				Items: []corev1.Namespace{
					{ObjectMeta: metav1.ObjectMeta{Name: "namespace1"}},
				},
			},
			servicesByNamespace: map[string][]*corev1.Service{
				"namespace1": {
					{
						ObjectMeta: metav1.ObjectMeta{Name: "service-with-no-selector"},
						Spec: corev1.ServiceSpec{
							Type:                  corev1.ServiceTypeNodePort,
							ExternalTrafficPolicy: corev1.ServiceExternalTrafficPolicyTypeLocal,
						},
					},
				},
			},
			policiesByNamespace: map[string][]*networkingv1.NetworkPolicy{
				"namespace1": {
					{
						ObjectMeta: metav1.ObjectMeta{Name: "deny-all-ingress-policy-with-no-selector"},
						Spec: networkingv1.NetworkPolicySpec{
							PodSelector: metav1.LabelSelector{},
							PolicyTypes: []networkingv1.PolicyType{"Ingress"},
						},
					},
				},
			},
			expectedUnsafeServices: []string{},
		},
		// Scenarios where there are LoadBalancer or NodePort services with externalTrafficPolicy=Cluster but no policies
		{
			name: "LoadBalancer service with externalTrafficPolicy=Cluster with no selector and no policies",
			namespaces: &corev1.NamespaceList{
				Items: []corev1.Namespace{
					{ObjectMeta: metav1.ObjectMeta{Name: "namespace1"}},
				},
			},
			servicesByNamespace: map[string][]*corev1.Service{
				"namespace1": {
					{
						ObjectMeta: metav1.ObjectMeta{Name: "service-with-no-selector"},
						Spec: corev1.ServiceSpec{
							Type:                  corev1.ServiceTypeLoadBalancer,
							ExternalTrafficPolicy: corev1.ServiceExternalTrafficPolicyTypeCluster,
						},
					},
				},
			},
			policiesByNamespace: map[string][]*networkingv1.NetworkPolicy{
				"namespace1": {},
			},
			expectedUnsafeServices: []string{},
		},
		{
			name: "NodePort service with externalTrafficPolicy=Cluster with no selector and no policies",
			namespaces: &corev1.NamespaceList{
				Items: []corev1.Namespace{
					{ObjectMeta: metav1.ObjectMeta{Name: "namespace1"}},
				},
			},
			servicesByNamespace: map[string][]*corev1.Service{
				"namespace1": {
					{
						ObjectMeta: metav1.ObjectMeta{Name: "service-with-no-selector"},
						Spec: corev1.ServiceSpec{
							Type:                  corev1.ServiceTypeNodePort,
							ExternalTrafficPolicy: corev1.ServiceExternalTrafficPolicyTypeCluster,
						},
					},
				},
			},
			policiesByNamespace: map[string][]*networkingv1.NetworkPolicy{
				"namespace1": {},
			},
			expectedUnsafeServices: []string{},
		},
		// Scenarios where there are LoadBalancer or NodePort services with externalTrafficPolicy=Cluster and policies allow traffic
		{
			name: "LoadBalancer service with externalTrafficPolicy=Cluster with no selector and an allow all ingress policy with no selector",
			namespaces: &corev1.NamespaceList{
				Items: []corev1.Namespace{
					{ObjectMeta: metav1.ObjectMeta{Name: "namespace1"}},
				},
			},
			servicesByNamespace: map[string][]*corev1.Service{
				"namespace1": {
					{
						ObjectMeta: metav1.ObjectMeta{Name: "service-with-no-selector"},
						Spec: corev1.ServiceSpec{
							Type:                  corev1.ServiceTypeLoadBalancer,
							ExternalTrafficPolicy: corev1.ServiceExternalTrafficPolicyTypeCluster,
						},
					},
				},
			},
			policiesByNamespace: map[string][]*networkingv1.NetworkPolicy{
				"namespace1": {
					{
						ObjectMeta: metav1.ObjectMeta{Name: "allow-all-ingress-policy-with-no-selector"},
						Spec: networkingv1.NetworkPolicySpec{
							PodSelector: metav1.LabelSelector{},
							PolicyTypes: []networkingv1.PolicyType{"Ingress"},
							Ingress: []networkingv1.NetworkPolicyIngressRule{
								{},
							},
						},
					},
				},
			},
			expectedUnsafeServices: []string{},
		},
		{
			name: "LoadBalancer service with externalTrafficPolicy=Cluster with a selector and an allow all ingress policy with a matching selector",
			namespaces: &corev1.NamespaceList{
				Items: []corev1.Namespace{
					{ObjectMeta: metav1.ObjectMeta{Name: "namespace1"}},
				},
			},
			servicesByNamespace: map[string][]*corev1.Service{
				"namespace1": {
					{
						ObjectMeta: metav1.ObjectMeta{Name: "service-with-selector"},
						Spec: corev1.ServiceSpec{
							Type:                  corev1.ServiceTypeLoadBalancer,
							Selector:              map[string]string{"app": "test"},
							ExternalTrafficPolicy: corev1.ServiceExternalTrafficPolicyTypeCluster,
						},
					},
				},
			},
			policiesByNamespace: map[string][]*networkingv1.NetworkPolicy{
				"namespace1": {
					{
						ObjectMeta: metav1.ObjectMeta{Name: "allow-all-ingress-policy-with-selector"},
						Spec: networkingv1.NetworkPolicySpec{
							PodSelector: metav1.LabelSelector{
								MatchLabels: map[string]string{"app": "test"},
							},
							PolicyTypes: []networkingv1.PolicyType{"Ingress"},
							Ingress: []networkingv1.NetworkPolicyIngressRule{
								{},
							},
						},
					},
				},
			},
			expectedUnsafeServices: []string{},
		},
		{
			name: "LoadBalancer service with externalTrafficPolicy=Cluster with a selector and an ingress policy with a matching selector and ports",
			namespaces: &corev1.NamespaceList{
				Items: []corev1.Namespace{
					{ObjectMeta: metav1.ObjectMeta{Name: "namespace1"}},
				},
			},
			servicesByNamespace: map[string][]*corev1.Service{
				"namespace1": {
					{
						ObjectMeta: metav1.ObjectMeta{Name: "service-with-selector-and-ports"},
						Spec: corev1.ServiceSpec{
							Type:     corev1.ServiceTypeLoadBalancer,
							Selector: map[string]string{"app": "test"},
							Ports: []corev1.ServicePort{
								{
									Port:       80,
									Protocol:   corev1.ProtocolTCP,
									TargetPort: intstr.FromInt(80),
								},
							},
							ExternalTrafficPolicy: corev1.ServiceExternalTrafficPolicyTypeCluster,
						},
					},
				},
			},
			policiesByNamespace: map[string][]*networkingv1.NetworkPolicy{
				"namespace1": {
					{
						ObjectMeta: metav1.ObjectMeta{Name: "ingress-policy-with-selector-and-ports"},
						Spec: networkingv1.NetworkPolicySpec{
							PodSelector: metav1.LabelSelector{
								MatchLabels: map[string]string{"app": "test"},
							},
							PolicyTypes: []networkingv1.PolicyType{"Ingress"},
							Ingress: []networkingv1.NetworkPolicyIngressRule{
								{
									Ports: []networkingv1.NetworkPolicyPort{
										{
											Port: intstrPtr(intstr.FromInt(80)),
											Protocol: func() *corev1.Protocol {
												protocol := corev1.ProtocolTCP
												return &protocol
											}(),
										},
									},
								},
							},
						},
					},
				},
			},
			expectedUnsafeServices: []string{},
		},
		{
			name: "NodePort service with externalTrafficPolicy=Cluster with no selector and an allow all ingress policy with no selector",
			namespaces: &corev1.NamespaceList{
				Items: []corev1.Namespace{
					{ObjectMeta: metav1.ObjectMeta{Name: "namespace1"}},
				},
			},
			servicesByNamespace: map[string][]*corev1.Service{
				"namespace1": {
					{
						ObjectMeta: metav1.ObjectMeta{Name: "service-with-no-selector"},
						Spec: corev1.ServiceSpec{
							Type:                  corev1.ServiceTypeNodePort,
							ExternalTrafficPolicy: corev1.ServiceExternalTrafficPolicyTypeCluster,
						},
					},
				},
			},
			policiesByNamespace: map[string][]*networkingv1.NetworkPolicy{
				"namespace1": {
					{
						ObjectMeta: metav1.ObjectMeta{Name: "allow-all-ingress-policy-with-no-selector"},
						Spec: networkingv1.NetworkPolicySpec{
							PodSelector: metav1.LabelSelector{},
							PolicyTypes: []networkingv1.PolicyType{"Ingress"},
							Ingress: []networkingv1.NetworkPolicyIngressRule{
								{},
							},
						},
					},
				},
			},
			expectedUnsafeServices: []string{},
		},
		{
			name: "NodePort service with externalTrafficPolicy=Cluster with a selector and an allow all ingress policy with a matching selector",
			namespaces: &corev1.NamespaceList{
				Items: []corev1.Namespace{
					{ObjectMeta: metav1.ObjectMeta{Name: "namespace1"}},
				},
			},
			servicesByNamespace: map[string][]*corev1.Service{
				"namespace1": {
					{
						ObjectMeta: metav1.ObjectMeta{Name: "service-with-selector"},
						Spec: corev1.ServiceSpec{
							Type:                  corev1.ServiceTypeNodePort,
							Selector:              map[string]string{"app": "test"},
							ExternalTrafficPolicy: corev1.ServiceExternalTrafficPolicyTypeCluster,
						},
					},
				},
			},
			policiesByNamespace: map[string][]*networkingv1.NetworkPolicy{
				"namespace1": {
					{
						ObjectMeta: metav1.ObjectMeta{Name: "allow-all-ingress-policy-with-selector"},
						Spec: networkingv1.NetworkPolicySpec{
							PodSelector: metav1.LabelSelector{
								MatchLabels: map[string]string{"app": "test"},
							},
							PolicyTypes: []networkingv1.PolicyType{"Ingress"},
							Ingress: []networkingv1.NetworkPolicyIngressRule{
								{},
							},
						},
					},
				},
			},
			expectedUnsafeServices: []string{},
		},
		{
			name: "NodePort service with externalTrafficPolicy=Cluster with a selector and an allow all ingress policy with a matching selector and ports",
			namespaces: &corev1.NamespaceList{
				Items: []corev1.Namespace{
					{ObjectMeta: metav1.ObjectMeta{Name: "namespace1"}},
				},
			},
			servicesByNamespace: map[string][]*corev1.Service{
				"namespace1": {
					{
						ObjectMeta: metav1.ObjectMeta{Name: "service-with-selector-and-ports"},
						Spec: corev1.ServiceSpec{
							Type:     corev1.ServiceTypeNodePort,
							Selector: map[string]string{"app": "test"},
							Ports: []corev1.ServicePort{
								{
									Port:       80,
									Protocol:   corev1.ProtocolTCP,
									TargetPort: intstr.FromInt(80),
								},
							},
							ExternalTrafficPolicy: corev1.ServiceExternalTrafficPolicyTypeCluster,
						},
					},
				},
			},
			policiesByNamespace: map[string][]*networkingv1.NetworkPolicy{
				"namespace1": {
					{
						ObjectMeta: metav1.ObjectMeta{Name: "allow-all-ingress-policy-with-selector-and-ports"},
						Spec: networkingv1.NetworkPolicySpec{
							PodSelector: metav1.LabelSelector{
								MatchLabels: map[string]string{"app": "test"},
							},
							PolicyTypes: []networkingv1.PolicyType{"Ingress"},
							Ingress: []networkingv1.NetworkPolicyIngressRule{
								{
									Ports: []networkingv1.NetworkPolicyPort{
										{
											Port: intstrPtr(intstr.FromInt(80)),
											Protocol: func() *corev1.Protocol {
												protocol := corev1.ProtocolTCP
												return &protocol
											}(),
										},
									},
								},
							},
						},
					},
				},
			},
			expectedUnsafeServices: []string{},
		},
		// Scenarios where there are LoadBalancer or NodePort services with externalTrafficPolicy=Cluster and policies deny traffic
		{
			name: "LoadBalancer service with externalTrafficPolicy=Cluster with no selector and a deny all ingress policy with no selector",
			namespaces: &corev1.NamespaceList{
				Items: []corev1.Namespace{
					{ObjectMeta: metav1.ObjectMeta{Name: "namespace1"}},
				},
			},
			servicesByNamespace: map[string][]*corev1.Service{
				"namespace1": {
					{
						ObjectMeta: metav1.ObjectMeta{Name: "service-with-no-selector"},
						Spec: corev1.ServiceSpec{
							Type:                  corev1.ServiceTypeLoadBalancer,
							ExternalTrafficPolicy: corev1.ServiceExternalTrafficPolicyTypeCluster,
						},
					},
				},
			},
			policiesByNamespace: map[string][]*networkingv1.NetworkPolicy{
				"namespace1": {
					{
						ObjectMeta: metav1.ObjectMeta{Name: "deny-all-ingress-policy-with-no-selector"},
						Spec: networkingv1.NetworkPolicySpec{
							PodSelector: metav1.LabelSelector{},
							PolicyTypes: []networkingv1.PolicyType{"Ingress"},
						},
					},
				},
			},
			expectedUnsafeServices: []string{"namespace1/service-with-no-selector"},
		},
		{
			name: "LoadBalancer service with externalTrafficPolicy=Cluster with no selector and an allow all ingress policy with a selector",
			namespaces: &corev1.NamespaceList{
				Items: []corev1.Namespace{
					{ObjectMeta: metav1.ObjectMeta{Name: "namespace1"}},
				},
			},
			servicesByNamespace: map[string][]*corev1.Service{
				"namespace1": {
					{
						ObjectMeta: metav1.ObjectMeta{Name: "service-with-no-selector"},
						Spec: corev1.ServiceSpec{
							Type:                  corev1.ServiceTypeLoadBalancer,
							ExternalTrafficPolicy: corev1.ServiceExternalTrafficPolicyTypeCluster,
						},
					},
				},
			},
			policiesByNamespace: map[string][]*networkingv1.NetworkPolicy{
				"namespace1": {
					{
						ObjectMeta: metav1.ObjectMeta{Name: "allow-all-ingress-policy-with-no-selector"},
						Spec: networkingv1.NetworkPolicySpec{
							PodSelector: metav1.LabelSelector{
								MatchLabels: map[string]string{"app": "test"},
							},
							PolicyTypes: []networkingv1.PolicyType{"Ingress"},
							Ingress: []networkingv1.NetworkPolicyIngressRule{
								{},
							},
						},
					},
				},
			},
			expectedUnsafeServices: []string{"namespace1/service-with-no-selector"},
		},
		{
			name: "LoadBalancer service with externalTrafficPolicy=Cluster with a selector and an deny all ingress policy with a matching selector",
			namespaces: &corev1.NamespaceList{
				Items: []corev1.Namespace{
					{ObjectMeta: metav1.ObjectMeta{Name: "namespace1"}},
				},
			},
			servicesByNamespace: map[string][]*corev1.Service{
				"namespace1": {
					{
						ObjectMeta: metav1.ObjectMeta{Name: "service-with-selector"},
						Spec: corev1.ServiceSpec{
							Type:                  corev1.ServiceTypeLoadBalancer,
							Selector:              map[string]string{"app": "test"},
							ExternalTrafficPolicy: corev1.ServiceExternalTrafficPolicyTypeCluster,
						},
					},
				},
			},
			policiesByNamespace: map[string][]*networkingv1.NetworkPolicy{
				"namespace1": {
					{
						ObjectMeta: metav1.ObjectMeta{Name: "deny-all-ingress-policy-with-selector"},
						Spec: networkingv1.NetworkPolicySpec{
							PodSelector: metav1.LabelSelector{
								MatchLabels: map[string]string{"app": "test"},
							},
							PolicyTypes: []networkingv1.PolicyType{"Ingress"},
						},
					},
				},
			},
			expectedUnsafeServices: []string{"namespace1/service-with-selector"},
		},
		{
			name: "LoadBalancer service with externalTrafficPolicy=Cluster and matching policy that has a pod selector but no ports",
			namespaces: &corev1.NamespaceList{
				Items: []corev1.Namespace{
					{ObjectMeta: metav1.ObjectMeta{Name: "namespace1"}},
				},
			},
			servicesByNamespace: map[string][]*corev1.Service{
				"namespace1": {
					{
						ObjectMeta: metav1.ObjectMeta{Name: "external-traffic-policy-cluster-service"},
						Spec: corev1.ServiceSpec{
							Type:                  corev1.ServiceTypeLoadBalancer,
							ExternalTrafficPolicy: corev1.ServiceExternalTrafficPolicyTypeCluster,
							Selector:              map[string]string{"app": "test"},
						},
					},
				},
			},
			policiesByNamespace: map[string][]*networkingv1.NetworkPolicy{
				"namespace1": {
					{
						ObjectMeta: metav1.ObjectMeta{Name: "policy1"},
						Spec: networkingv1.NetworkPolicySpec{
							PodSelector: metav1.LabelSelector{
								MatchLabels: map[string]string{"app": "test"},
							},
							PolicyTypes: []networkingv1.PolicyType{"Ingress"},
							Ingress: []networkingv1.NetworkPolicyIngressRule{
								{
									From: []networkingv1.NetworkPolicyPeer{
										{
											PodSelector: &metav1.LabelSelector{
												MatchLabels: map[string]string{"app": "test"},
											},
										},
									},
								},
							},
						},
					},
				},
			},
			expectedUnsafeServices: []string{"namespace1/external-traffic-policy-cluster-service"},
		},
		{
			name: "LoadBalancer service with externalTrafficPolicy=Cluster with a selector and an allow all ingress policy with a selector that doesnt match",
			namespaces: &corev1.NamespaceList{
				Items: []corev1.Namespace{
					{ObjectMeta: metav1.ObjectMeta{Name: "namespace1"}},
				},
			},
			servicesByNamespace: map[string][]*corev1.Service{
				"namespace1": {
					{
						ObjectMeta: metav1.ObjectMeta{Name: "service-with-selector"},
						Spec: corev1.ServiceSpec{
							Type:                  corev1.ServiceTypeLoadBalancer,
							Selector:              map[string]string{"app": "test", "app3": "test3", "app4": "test4"},
							ExternalTrafficPolicy: corev1.ServiceExternalTrafficPolicyTypeCluster,
						},
					},
				},
			},
			policiesByNamespace: map[string][]*networkingv1.NetworkPolicy{
				"namespace1": {
					{
						ObjectMeta: metav1.ObjectMeta{Name: "allow-all-ingress-policy-with-selector"},
						Spec: networkingv1.NetworkPolicySpec{
							PodSelector: metav1.LabelSelector{
								MatchLabels: map[string]string{"app": "test", "app2": "test2"},
							},
							PolicyTypes: []networkingv1.PolicyType{"Ingress"},
							Ingress: []networkingv1.NetworkPolicyIngressRule{
								{},
							},
						},
					},
				},
			},
			expectedUnsafeServices: []string{"namespace1/service-with-selector"},
		},
		{
			name: "LoadBalancer service with externalTrafficPolicy=Cluster with a selector and an allow all ingress policy with a selector that has more labels",
			namespaces: &corev1.NamespaceList{
				Items: []corev1.Namespace{
					{ObjectMeta: metav1.ObjectMeta{Name: "namespace1"}},
				},
			},
			servicesByNamespace: map[string][]*corev1.Service{
				"namespace1": {
					{
						ObjectMeta: metav1.ObjectMeta{Name: "service-with-selector"},
						Spec: corev1.ServiceSpec{
							Type:                  corev1.ServiceTypeLoadBalancer,
							Selector:              map[string]string{"app": "test"},
							ExternalTrafficPolicy: corev1.ServiceExternalTrafficPolicyTypeCluster,
						},
					},
				},
			},
			policiesByNamespace: map[string][]*networkingv1.NetworkPolicy{
				"namespace1": {
					{
						ObjectMeta: metav1.ObjectMeta{Name: "allow-all-ingress-policy-with-selector"},
						Spec: networkingv1.NetworkPolicySpec{
							PodSelector: metav1.LabelSelector{
								MatchLabels: map[string]string{"app": "test", "app2": "test2", "app3": "test3"},
							},
							PolicyTypes: []networkingv1.PolicyType{"Ingress"},
							Ingress: []networkingv1.NetworkPolicyIngressRule{
								{},
							},
						},
					},
				},
			},
			expectedUnsafeServices: []string{"namespace1/service-with-selector"},
		},
		{
			name: "LoadBalancer service with externalTrafficPolicy=Cluster with a selector and an ingress policy with a matching selector but ports dont match",
			namespaces: &corev1.NamespaceList{
				Items: []corev1.Namespace{
					{ObjectMeta: metav1.ObjectMeta{Name: "namespace1"}},
				},
			},
			servicesByNamespace: map[string][]*corev1.Service{
				"namespace1": {
					{
						ObjectMeta: metav1.ObjectMeta{Name: "service-with-selector-and-named-ports"},
						Spec: corev1.ServiceSpec{
							Type:     corev1.ServiceTypeLoadBalancer,
							Selector: map[string]string{"app": "test"},
							Ports: []corev1.ServicePort{
								{
									Port:       80,
									Protocol:   corev1.ProtocolTCP,
									TargetPort: intstr.FromInt(80),
								},
								{
									Port:       100,
									Protocol:   corev1.ProtocolTCP,
									TargetPort: intstr.FromInt(100),
								},
							},
							ExternalTrafficPolicy: corev1.ServiceExternalTrafficPolicyTypeCluster,
						},
					},
				},
			},
			policiesByNamespace: map[string][]*networkingv1.NetworkPolicy{
				"namespace1": {
					{
						ObjectMeta: metav1.ObjectMeta{Name: "ingress-policy-with-selector-and-ports"},
						Spec: networkingv1.NetworkPolicySpec{
							PodSelector: metav1.LabelSelector{
								MatchLabels: map[string]string{"app": "test"},
							},
							PolicyTypes: []networkingv1.PolicyType{"Ingress"},
							Ingress: []networkingv1.NetworkPolicyIngressRule{
								{
									Ports: []networkingv1.NetworkPolicyPort{
										{
											Port: intstrPtr(intstr.FromInt(80)),
											Protocol: func() *corev1.Protocol {
												protocol := corev1.ProtocolTCP
												return &protocol
											}(),
										},
										{
											Port: intstrPtr(intstr.FromInt(90)),
											Protocol: func() *corev1.Protocol {
												protocol := corev1.ProtocolTCP
												return &protocol
											}(),
										},
									},
								},
							},
						},
					},
				},
			},
			expectedUnsafeServices: []string{"namespace1/service-with-selector-and-named-ports"},
		},
		{
			name: "LoadBalancer service with externalTrafficPolicy=Cluster with a selector and an ingress policy with a matching selector but uses named ports",
			namespaces: &corev1.NamespaceList{
				Items: []corev1.Namespace{
					{ObjectMeta: metav1.ObjectMeta{Name: "namespace1"}},
				},
			},
			servicesByNamespace: map[string][]*corev1.Service{
				"namespace1": {
					{
						ObjectMeta: metav1.ObjectMeta{Name: "service-with-selector-and-named-ports"},
						Spec: corev1.ServiceSpec{
							Type:     corev1.ServiceTypeLoadBalancer,
							Selector: map[string]string{"app": "test"},
							Ports: []corev1.ServicePort{
								{
									Port:       80,
									Protocol:   corev1.ProtocolTCP,
									TargetPort: intstr.FromString("http"),
								},
							},
							ExternalTrafficPolicy: corev1.ServiceExternalTrafficPolicyTypeCluster,
						},
					},
				},
			},
			policiesByNamespace: map[string][]*networkingv1.NetworkPolicy{
				"namespace1": {
					{
						ObjectMeta: metav1.ObjectMeta{Name: "ingress-policy-with-selector-and-named-ports"},
						Spec: networkingv1.NetworkPolicySpec{
							PodSelector: metav1.LabelSelector{
								MatchLabels: map[string]string{"app": "test"},
							},
							PolicyTypes: []networkingv1.PolicyType{"Ingress"},
							Ingress: []networkingv1.NetworkPolicyIngressRule{
								{
									Ports: []networkingv1.NetworkPolicyPort{
										{
											Port: intstrPtr(intstr.FromString("http")),
											Protocol: func() *corev1.Protocol {
												protocol := corev1.ProtocolTCP
												return &protocol
											}(),
										},
									},
								},
							},
						},
					},
				},
			},
			expectedUnsafeServices: []string{"namespace1/service-with-selector-and-named-ports"},
		},
		// Scenarios covering edge cases
		{
			name: "LoadBalancer service with externalTrafficPolicy=Cluster with no selector and a allow all and deny all ingress policy with no selector",
			namespaces: &corev1.NamespaceList{
				Items: []corev1.Namespace{
					{ObjectMeta: metav1.ObjectMeta{Name: "namespace1"}},
				},
			},
			servicesByNamespace: map[string][]*corev1.Service{
				"namespace1": {
					{
						ObjectMeta: metav1.ObjectMeta{Name: "service-with-no-selector"},
						Spec: corev1.ServiceSpec{
							Type:                  corev1.ServiceTypeLoadBalancer,
							ExternalTrafficPolicy: corev1.ServiceExternalTrafficPolicyTypeCluster,
						},
					},
				},
			},
			policiesByNamespace: map[string][]*networkingv1.NetworkPolicy{
				"namespace1": {
					{
						ObjectMeta: metav1.ObjectMeta{Name: "deny-all-ingress-policy-with-no-selector"},
						Spec: networkingv1.NetworkPolicySpec{
							PodSelector: metav1.LabelSelector{},
							PolicyTypes: []networkingv1.PolicyType{"Ingress"},
						},
					},
					{
						ObjectMeta: metav1.ObjectMeta{Name: "allow-all-ingress-policy-with-no-selector"},
						Spec: networkingv1.NetworkPolicySpec{
							PodSelector: metav1.LabelSelector{},
							PolicyTypes: []networkingv1.PolicyType{"Ingress"},
							Ingress: []networkingv1.NetworkPolicyIngressRule{
								{},
							},
						},
					},
				},
			},
			expectedUnsafeServices: []string{},
		},
		{
			name: "LoadBalancer service with externalTrafficPolicy=Cluster with a selector and a allow all and deny all ingress policy with a matching selector",
			namespaces: &corev1.NamespaceList{
				Items: []corev1.Namespace{
					{ObjectMeta: metav1.ObjectMeta{Name: "namespace1"}},
				},
			},
			servicesByNamespace: map[string][]*corev1.Service{
				"namespace1": {
					{
						ObjectMeta: metav1.ObjectMeta{Name: "service-with-selector"},
						Spec: corev1.ServiceSpec{
							Type:                  corev1.ServiceTypeLoadBalancer,
							Selector:              map[string]string{"app": "test"},
							ExternalTrafficPolicy: corev1.ServiceExternalTrafficPolicyTypeCluster,
						},
					},
				},
			},
			policiesByNamespace: map[string][]*networkingv1.NetworkPolicy{
				"namespace1": {
					{
						ObjectMeta: metav1.ObjectMeta{Name: "deny-all-ingress-policy-with-selector"},
						Spec: networkingv1.NetworkPolicySpec{
							PodSelector: metav1.LabelSelector{
								MatchLabels: map[string]string{"app": "test"},
							},
							PolicyTypes: []networkingv1.PolicyType{"Ingress"},
						},
					},
					{
						ObjectMeta: metav1.ObjectMeta{Name: "allow-all-ingress-policy-with-a-selector"},
						Spec: networkingv1.NetworkPolicySpec{
							PodSelector: metav1.LabelSelector{
								MatchLabels: map[string]string{"app": "test"},
							},
							PolicyTypes: []networkingv1.PolicyType{"Ingress"},
							Ingress: []networkingv1.NetworkPolicyIngressRule{
								{},
							},
						},
					},
				},
			},
			expectedUnsafeServices: []string{},
		},
		{
			name: "LoadBalancer service with externalTrafficPolicy=Cluster with a selector and no ports and an ingress policy with a matching selector and ports",
			namespaces: &corev1.NamespaceList{
				Items: []corev1.Namespace{
					{ObjectMeta: metav1.ObjectMeta{Name: "namespace1"}},
				},
			},
			servicesByNamespace: map[string][]*corev1.Service{
				"namespace1": {
					{
						ObjectMeta: metav1.ObjectMeta{Name: "service-with-selector"},
						Spec: corev1.ServiceSpec{
							Type:                  corev1.ServiceTypeLoadBalancer,
							Selector:              map[string]string{"app": "test"},
							ExternalTrafficPolicy: corev1.ServiceExternalTrafficPolicyTypeCluster,
						},
					},
				},
			},
			policiesByNamespace: map[string][]*networkingv1.NetworkPolicy{
				"namespace1": {
					{
						ObjectMeta: metav1.ObjectMeta{Name: "ingress-policy-with-selector-and-ports"},
						Spec: networkingv1.NetworkPolicySpec{
							PodSelector: metav1.LabelSelector{
								MatchLabels: map[string]string{"app": "test"},
							},
							PolicyTypes: []networkingv1.PolicyType{"Ingress"},
							Ingress: []networkingv1.NetworkPolicyIngressRule{
								{
									Ports: []networkingv1.NetworkPolicyPort{
										{
											Port: intstrPtr(intstr.FromInt(80)),
											Protocol: func() *corev1.Protocol {
												protocol := corev1.ProtocolTCP
												return &protocol
											}(),
										},
									},
								},
							},
						},
					},
				},
			},
			expectedUnsafeServices: []string{"namespace1/service-with-selector"},
		},
		{
			name: "LoadBalancer service with externalTrafficPolicy=Cluster with no selector and an allow all ingress policy with a matchExpressions selector",
			namespaces: &corev1.NamespaceList{
				Items: []corev1.Namespace{
					{ObjectMeta: metav1.ObjectMeta{Name: "namespace1"}},
				},
			},
			servicesByNamespace: map[string][]*corev1.Service{
				"namespace1": {
					{
						ObjectMeta: metav1.ObjectMeta{Name: "service-with-selector-and-ports"},
						Spec: corev1.ServiceSpec{
							Type:     corev1.ServiceTypeLoadBalancer,
							Selector: map[string]string{"app": "test"},
							Ports: []corev1.ServicePort{
								{
									Port:       80,
									Protocol:   corev1.ProtocolTCP,
									TargetPort: intstr.FromInt(80),
								},
							},
							ExternalTrafficPolicy: corev1.ServiceExternalTrafficPolicyTypeCluster,
						},
					},
				},
			},
			policiesByNamespace: map[string][]*networkingv1.NetworkPolicy{
				"namespace1": {
					{
						ObjectMeta: metav1.ObjectMeta{Name: "allow-all-ingress-policy-with-matchexpressins-selector"},
						Spec: networkingv1.NetworkPolicySpec{
							PodSelector: metav1.LabelSelector{
								MatchExpressions: []metav1.LabelSelectorRequirement{
									{
										Key:      "app",
										Operator: metav1.LabelSelectorOpIn,
										Values:   []string{"test"},
									},
								},
							},
							PolicyTypes: []networkingv1.PolicyType{"Ingress"},
							Ingress: []networkingv1.NetworkPolicyIngressRule{
								{},
							},
						},
					},
				},
			},
			expectedUnsafeServices: []string{"namespace1/service-with-selector-and-ports"},
		},
		{
			name: "LoadBalancer service with externalTrafficPolicy=Cluster with a selector and an allow all ingress policy with a matchExpressions selector",
			namespaces: &corev1.NamespaceList{
				Items: []corev1.Namespace{
					{ObjectMeta: metav1.ObjectMeta{Name: "namespace1"}},
				},
			},
			servicesByNamespace: map[string][]*corev1.Service{
				"namespace1": {
					{
						ObjectMeta: metav1.ObjectMeta{Name: "service-with-no-selector"},
						Spec: corev1.ServiceSpec{
							Type:                  corev1.ServiceTypeLoadBalancer,
							ExternalTrafficPolicy: corev1.ServiceExternalTrafficPolicyTypeCluster,
						},
					},
				},
			},
			policiesByNamespace: map[string][]*networkingv1.NetworkPolicy{
				"namespace1": {
					{
						ObjectMeta: metav1.ObjectMeta{Name: "allow-all-ingress-policy-with-matchexpressions-selector"},
						Spec: networkingv1.NetworkPolicySpec{
							PodSelector: metav1.LabelSelector{
								MatchExpressions: []metav1.LabelSelectorRequirement{
									{
										Key:      "app",
										Operator: metav1.LabelSelectorOpIn,
										Values:   []string{"test"},
									},
								},
							},
							PolicyTypes: []networkingv1.PolicyType{"Ingress"},
							Ingress: []networkingv1.NetworkPolicyIngressRule{
								{},
							},
						},
					},
				},
			},
			expectedUnsafeServices: []string{"namespace1/service-with-no-selector"},
		},
		{
			name: "LoadBalancer service with externalTrafficPolicy=Cluster with a selector and an ingress policy with a matching selector and protocol with no ports",
			namespaces: &corev1.NamespaceList{
				Items: []corev1.Namespace{
					{ObjectMeta: metav1.ObjectMeta{Name: "namespace1"}},
				},
			},
			servicesByNamespace: map[string][]*corev1.Service{
				"namespace1": {
					{
						ObjectMeta: metav1.ObjectMeta{Name: "service-with-selector-and-ports"},
						Spec: corev1.ServiceSpec{
							Type:     corev1.ServiceTypeLoadBalancer,
							Selector: map[string]string{"app": "test"},
							Ports: []corev1.ServicePort{
								{
									Port:       80,
									Protocol:   corev1.ProtocolTCP,
									TargetPort: intstr.FromInt(80),
								},
							},
							ExternalTrafficPolicy: corev1.ServiceExternalTrafficPolicyTypeCluster,
						},
					},
				},
			},
			policiesByNamespace: map[string][]*networkingv1.NetworkPolicy{
				"namespace1": {
					{
						ObjectMeta: metav1.ObjectMeta{Name: "ingress-policy-with-selector-and-ports"},
						Spec: networkingv1.NetworkPolicySpec{
							PodSelector: metav1.LabelSelector{
								MatchLabels: map[string]string{"app": "test"},
							},
							PolicyTypes: []networkingv1.PolicyType{"Ingress"},
							Ingress: []networkingv1.NetworkPolicyIngressRule{
								{
									Ports: []networkingv1.NetworkPolicyPort{
										{
											Protocol: func() *corev1.Protocol {
												protocol := corev1.ProtocolTCP
												return &protocol
											}(),
										},
									},
								},
							},
						},
					},
				},
			},
			expectedUnsafeServices: []string{},
		},
		{
			name: "LoadBalancer service with externalTrafficPolicy=Cluster with a selector and an allow all ingress policy with a matching selector and port and port=0",
			namespaces: &corev1.NamespaceList{
				Items: []corev1.Namespace{
					{ObjectMeta: metav1.ObjectMeta{Name: "namespace1"}},
				},
			},
			servicesByNamespace: map[string][]*corev1.Service{
				"namespace1": {
					{
						ObjectMeta: metav1.ObjectMeta{Name: "service-with-selector-and-ports"},
						Spec: corev1.ServiceSpec{
							Type:     corev1.ServiceTypeLoadBalancer,
							Selector: map[string]string{"app": "test"},
							Ports: []corev1.ServicePort{
								{
									Port:       80,
									Protocol:   corev1.ProtocolTCP,
									TargetPort: intstr.FromInt(80),
								},
							},
							ExternalTrafficPolicy: corev1.ServiceExternalTrafficPolicyTypeCluster,
						},
					},
				},
			},
			policiesByNamespace: map[string][]*networkingv1.NetworkPolicy{
				"namespace1": {
					{
						ObjectMeta: metav1.ObjectMeta{Name: "allow-all-ingress-policy-with-selector-and-ports"},
						Spec: networkingv1.NetworkPolicySpec{
							PodSelector: metav1.LabelSelector{
								MatchLabels: map[string]string{"app": "test"},
							},
							PolicyTypes: []networkingv1.PolicyType{"Ingress"},
							Ingress: []networkingv1.NetworkPolicyIngressRule{
								{
									Ports: []networkingv1.NetworkPolicyPort{
										{
											Port: intstrPtr(intstr.FromInt(0)),
											Protocol: func() *corev1.Protocol {
												protocol := corev1.ProtocolTCP
												return &protocol
											}(),
										},
										{
											Port: intstrPtr(intstr.FromInt(80)),
											Protocol: func() *corev1.Protocol {
												protocol := corev1.ProtocolTCP
												return &protocol
											}(),
										},
									},
								},
							},
						},
					},
				},
			},
			expectedUnsafeServices: []string{},
		},
		{
			name: "LoadBalancer service with externalTrafficPolicy=Cluster with a selector and targetport=0 and an allow all ingress policy with a matching selector and different ports",
			namespaces: &corev1.NamespaceList{
				Items: []corev1.Namespace{
					{ObjectMeta: metav1.ObjectMeta{Name: "namespace1"}},
				},
			},
			servicesByNamespace: map[string][]*corev1.Service{
				"namespace1": {
					{
						ObjectMeta: metav1.ObjectMeta{Name: "service-with-selector-and-ports"},
						Spec: corev1.ServiceSpec{
							Type:     corev1.ServiceTypeLoadBalancer,
							Selector: map[string]string{"app": "test"},
							Ports: []corev1.ServicePort{
								{
									Port:       80,
									Protocol:   corev1.ProtocolTCP,
									TargetPort: intstr.FromInt(0),
								},
							},
							ExternalTrafficPolicy: corev1.ServiceExternalTrafficPolicyTypeCluster,
						},
					},
				},
			},
			policiesByNamespace: map[string][]*networkingv1.NetworkPolicy{
				"namespace1": {
					{
						ObjectMeta: metav1.ObjectMeta{Name: "allow-all-ingress-policy-with-selector-and-ports"},
						Spec: networkingv1.NetworkPolicySpec{
							PodSelector: metav1.LabelSelector{
								MatchLabels: map[string]string{"app": "test"},
							},
							PolicyTypes: []networkingv1.PolicyType{"Ingress"},
							Ingress: []networkingv1.NetworkPolicyIngressRule{
								{
									Ports: []networkingv1.NetworkPolicyPort{
										{
											Port: intstrPtr(intstr.FromInt(80)),
											Protocol: func() *corev1.Protocol {
												protocol := corev1.ProtocolTCP
												return &protocol
											}(),
										},
									},
								},
							},
						},
					},
				},
			},
			expectedUnsafeServices: []string{"namespace1/service-with-selector-and-ports"},
		},
		{
			name: "LoadBalancer service with externalTrafficPolicy=Cluster with a selector and targetport=0 and an allow all ingress policy with a matching selector and ports=0",
			namespaces: &corev1.NamespaceList{
				Items: []corev1.Namespace{
					{ObjectMeta: metav1.ObjectMeta{Name: "namespace1"}},
				},
			},
			servicesByNamespace: map[string][]*corev1.Service{
				"namespace1": {
					{
						ObjectMeta: metav1.ObjectMeta{Name: "service-with-selector-and-ports"},
						Spec: corev1.ServiceSpec{
							Type:     corev1.ServiceTypeLoadBalancer,
							Selector: map[string]string{"app": "test"},
							Ports: []corev1.ServicePort{
								{
									Port:       80,
									Protocol:   corev1.ProtocolTCP,
									TargetPort: intstr.FromInt(0),
								},
							},
							ExternalTrafficPolicy: corev1.ServiceExternalTrafficPolicyTypeCluster,
						},
					},
				},
			},
			policiesByNamespace: map[string][]*networkingv1.NetworkPolicy{
				"namespace1": {
					{
						ObjectMeta: metav1.ObjectMeta{Name: "allow-all-ingress-policy-with-selector-and-ports"},
						Spec: networkingv1.NetworkPolicySpec{
							PodSelector: metav1.LabelSelector{
								MatchLabels: map[string]string{"app": "test"},
							},
							PolicyTypes: []networkingv1.PolicyType{"Ingress"},
							Ingress: []networkingv1.NetworkPolicyIngressRule{
								{
									Ports: []networkingv1.NetworkPolicyPort{
										{
											Port: intstrPtr(intstr.FromInt(0)),
											Protocol: func() *corev1.Protocol {
												protocol := corev1.ProtocolTCP
												return &protocol
											}(),
										},
									},
								},
							},
						},
					},
				},
			},
			expectedUnsafeServices: []string{"namespace1/service-with-selector-and-ports"},
		},
		{
			name: "LoadBalancer service with externalTrafficPolicy=Cluster with a selector and an ingress policy with a matching selector and ports and pod/namespace selectors",
			namespaces: &corev1.NamespaceList{
				Items: []corev1.Namespace{
					{ObjectMeta: metav1.ObjectMeta{Name: "namespace1"}},
				},
			},
			servicesByNamespace: map[string][]*corev1.Service{
				"namespace1": {
					{
						ObjectMeta: metav1.ObjectMeta{Name: "service-with-selector-and-ports"},
						Spec: corev1.ServiceSpec{
							Type:     corev1.ServiceTypeLoadBalancer,
							Selector: map[string]string{"app": "test"},
							Ports: []corev1.ServicePort{
								{
									Port:       80,
									Protocol:   corev1.ProtocolTCP,
									TargetPort: intstr.FromInt(80),
								},
							},
							ExternalTrafficPolicy: corev1.ServiceExternalTrafficPolicyTypeCluster,
						},
					},
				},
			},
			policiesByNamespace: map[string][]*networkingv1.NetworkPolicy{
				"namespace1": {
					{
						ObjectMeta: metav1.ObjectMeta{Name: "allow-all-ingress-policy-with-selector-and-ports"},
						Spec: networkingv1.NetworkPolicySpec{
							PodSelector: metav1.LabelSelector{
								MatchLabels: map[string]string{"app": "test"},
							},
							PolicyTypes: []networkingv1.PolicyType{"Ingress"},
							Ingress: []networkingv1.NetworkPolicyIngressRule{
								{
									From: []networkingv1.NetworkPolicyPeer{
										{
											PodSelector: &metav1.LabelSelector{
												MatchLabels: map[string]string{"app": "test"},
											},
											NamespaceSelector: &metav1.LabelSelector{
												MatchLabels: map[string]string{"app": "test"},
											},
										},
									},
									Ports: []networkingv1.NetworkPolicyPort{
										{
											Port: intstrPtr(intstr.FromInt(80)),
											Protocol: func() *corev1.Protocol {
												protocol := corev1.ProtocolTCP
												return &protocol
											}(),
										},
									},
								},
							},
						},
					},
				},
			},
			expectedUnsafeServices: []string{"namespace1/service-with-selector-and-ports"},
		},
		// Scenarios where there are LoadBalancer or NodePort services with externalTrafficPolicy=Cluster and there are multiple namespaces
		{
			name: "LoadBalancer or NodePort services with externalTrafficPolicy=Cluster and allow all ingress policies with matching label and ports in multiple namespaces",
			namespaces: &corev1.NamespaceList{
				Items: []corev1.Namespace{
					{ObjectMeta: metav1.ObjectMeta{Name: "namespace1"}},
					{ObjectMeta: metav1.ObjectMeta{Name: "namespace2"}},
					{ObjectMeta: metav1.ObjectMeta{Name: "namespace3"}},
				},
			},
			servicesByNamespace: map[string][]*corev1.Service{
				"namespace1": {
					{
						ObjectMeta: metav1.ObjectMeta{Name: "service-with-selector"},
						Spec: corev1.ServiceSpec{
							Type:                  corev1.ServiceTypeLoadBalancer,
							Selector:              map[string]string{"app": "test"},
							ExternalTrafficPolicy: corev1.ServiceExternalTrafficPolicyTypeCluster,
						},
					},
				},
				"namespace2": {
					{
						ObjectMeta: metav1.ObjectMeta{Name: "service-with-selector-and-ports"},
						Spec: corev1.ServiceSpec{
							Type:     corev1.ServiceTypeNodePort,
							Selector: map[string]string{"app": "test"},
							Ports: []corev1.ServicePort{
								{
									Port:       80,
									Protocol:   corev1.ProtocolTCP,
									TargetPort: intstr.FromInt(80),
								},
							},
							ExternalTrafficPolicy: corev1.ServiceExternalTrafficPolicyTypeCluster,
						},
					},
				},
				"namespace3": {
					{
						ObjectMeta: metav1.ObjectMeta{Name: "service-with-selector-and-ports"},
						Spec: corev1.ServiceSpec{
							Type:     corev1.ServiceTypeLoadBalancer,
							Selector: map[string]string{"app": "test"},
							Ports: []corev1.ServicePort{
								{
									Port:       80,
									Protocol:   corev1.ProtocolTCP,
									TargetPort: intstr.FromInt(80),
								},
							},
							ExternalTrafficPolicy: corev1.ServiceExternalTrafficPolicyTypeCluster,
						},
					},
				},
			},
			policiesByNamespace: map[string][]*networkingv1.NetworkPolicy{
				"namespace1": {
					{
						ObjectMeta: metav1.ObjectMeta{Name: "allow-all-ingress-policy-with-selector"},
						Spec: networkingv1.NetworkPolicySpec{
							PodSelector: metav1.LabelSelector{
								MatchLabels: map[string]string{"app": "test"},
							},
							PolicyTypes: []networkingv1.PolicyType{"Ingress"},
							Ingress: []networkingv1.NetworkPolicyIngressRule{
								{},
							},
						},
					},
				},
				"namespace2": {
					{
						ObjectMeta: metav1.ObjectMeta{Name: "allow-all-ingress-policy-with-selector-and-ports"},
						Spec: networkingv1.NetworkPolicySpec{
							PodSelector: metav1.LabelSelector{
								MatchLabels: map[string]string{"app": "test"},
							},
							PolicyTypes: []networkingv1.PolicyType{"Ingress"},
							Ingress: []networkingv1.NetworkPolicyIngressRule{
								{
									Ports: []networkingv1.NetworkPolicyPort{
										{
											Port: intstrPtr(intstr.FromInt(80)),
											Protocol: func() *corev1.Protocol {
												protocol := corev1.ProtocolTCP
												return &protocol
											}(),
										},
									},
								},
							},
						},
					},
				},
				"namespace3": {
					{
						ObjectMeta: metav1.ObjectMeta{Name: "allow-all-ingress-policy-with-selector-and-ports"},
						Spec: networkingv1.NetworkPolicySpec{
							PodSelector: metav1.LabelSelector{
								MatchLabels: map[string]string{"app": "test"},
							},
							PolicyTypes: []networkingv1.PolicyType{"Ingress"},
							Ingress: []networkingv1.NetworkPolicyIngressRule{
								{
									Ports: []networkingv1.NetworkPolicyPort{
										{
											Protocol: func() *corev1.Protocol {
												protocol := corev1.ProtocolTCP
												return &protocol
											}(),
										},
									},
								},
							},
						},
					},
				},
			},
			expectedUnsafeServices: []string{},
		},
		{
			name: "LoadBalancer or NodePort services with externalTrafficPolicy=Cluster and allow all ingress policies without matching label and ports in multiple namespaces",
			namespaces: &corev1.NamespaceList{
				Items: []corev1.Namespace{
					{ObjectMeta: metav1.ObjectMeta{Name: "namespace1"}},
					{ObjectMeta: metav1.ObjectMeta{Name: "namespace2"}},
					{ObjectMeta: metav1.ObjectMeta{Name: "namespace3"}},
				},
			},
			servicesByNamespace: map[string][]*corev1.Service{
				"namespace1": {
					{
						ObjectMeta: metav1.ObjectMeta{Name: "service-with-selector"},
						Spec: corev1.ServiceSpec{
							Type:                  corev1.ServiceTypeLoadBalancer,
							Selector:              map[string]string{"app": "test2"},
							ExternalTrafficPolicy: corev1.ServiceExternalTrafficPolicyTypeCluster,
						},
					},
				},
				"namespace2": {
					{
						ObjectMeta: metav1.ObjectMeta{Name: "service-with-selector-and-ports"},
						Spec: corev1.ServiceSpec{
							Type:     corev1.ServiceTypeNodePort,
							Selector: map[string]string{"app": "test"},
							Ports: []corev1.ServicePort{
								{
									Port:       80,
									Protocol:   corev1.ProtocolTCP,
									TargetPort: intstr.FromInt(80),
								},
								{
									Port:       90,
									Protocol:   corev1.ProtocolTCP,
									TargetPort: intstr.FromInt(90),
								},
							},
							ExternalTrafficPolicy: corev1.ServiceExternalTrafficPolicyTypeCluster,
						},
					},
				},
				"namespace3": {
					{
						ObjectMeta: metav1.ObjectMeta{Name: "service-with-selector-and-ports"},
						Spec: corev1.ServiceSpec{
							Type:     corev1.ServiceTypeLoadBalancer,
							Selector: map[string]string{"app": "test"},
							Ports: []corev1.ServicePort{
								{
									Port:       80,
									Protocol:   corev1.ProtocolTCP,
									TargetPort: intstr.FromInt(80),
								},
							},
							ExternalTrafficPolicy: corev1.ServiceExternalTrafficPolicyTypeCluster,
						},
					},
				},
			},
			policiesByNamespace: map[string][]*networkingv1.NetworkPolicy{
				"namespace1": {
					{
						ObjectMeta: metav1.ObjectMeta{Name: "allow-all-ingress-policy-with-selector"},
						Spec: networkingv1.NetworkPolicySpec{
							PodSelector: metav1.LabelSelector{
								MatchLabels: map[string]string{"app": "test"},
							},
							PolicyTypes: []networkingv1.PolicyType{"Ingress"},
							Ingress: []networkingv1.NetworkPolicyIngressRule{
								{},
							},
						},
					},
				},
				"namespace2": {
					{
						ObjectMeta: metav1.ObjectMeta{Name: "allow-all-ingress-policy-with-selector-and-ports"},
						Spec: networkingv1.NetworkPolicySpec{
							PodSelector: metav1.LabelSelector{
								MatchLabels: map[string]string{"app": "test"},
							},
							PolicyTypes: []networkingv1.PolicyType{"Ingress"},
							Ingress: []networkingv1.NetworkPolicyIngressRule{
								{
									Ports: []networkingv1.NetworkPolicyPort{
										{
											Port: intstrPtr(intstr.FromInt(80)),
											Protocol: func() *corev1.Protocol {
												protocol := corev1.ProtocolTCP
												return &protocol
											}(),
										},
									},
								},
							},
						},
					},
				},
				"namespace3": {
					{
						ObjectMeta: metav1.ObjectMeta{Name: "allow-all-ingress-policy-with-selector-and-ports"},
						Spec: networkingv1.NetworkPolicySpec{
							PodSelector: metav1.LabelSelector{
								MatchLabels: map[string]string{"app": "test"},
							},
							PolicyTypes: []networkingv1.PolicyType{"Ingress"},
							Ingress: []networkingv1.NetworkPolicyIngressRule{
								{
									Ports: []networkingv1.NetworkPolicyPort{
										{
											Protocol: func() *corev1.Protocol {
												protocol := corev1.ProtocolUDP
												return &protocol
											}(),
										},
									},
								},
							},
						},
					},
				},
			},
			expectedUnsafeServices: []string{"namespace1/service-with-selector", "namespace2/service-with-selector-and-ports", "namespace3/service-with-selector-and-ports"},
		},
		{
			name: "LoadBalancer or NodePort services with externalTrafficPolicy=Cluster and allow all ingress policies with some matching label and ports in multiple namespaces",
			namespaces: &corev1.NamespaceList{
				Items: []corev1.Namespace{
					{ObjectMeta: metav1.ObjectMeta{Name: "namespace1"}},
					{ObjectMeta: metav1.ObjectMeta{Name: "namespace2"}},
					{ObjectMeta: metav1.ObjectMeta{Name: "namespace3"}},
				},
			},
			servicesByNamespace: map[string][]*corev1.Service{
				"namespace1": {
					{
						ObjectMeta: metav1.ObjectMeta{Name: "service-with-selector-match"},
						Spec: corev1.ServiceSpec{
							Type:                  corev1.ServiceTypeLoadBalancer,
							Selector:              map[string]string{"app": "test"},
							ExternalTrafficPolicy: corev1.ServiceExternalTrafficPolicyTypeCluster,
						},
					},
					{
						ObjectMeta: metav1.ObjectMeta{Name: "service-with-selector-no-match"},
						Spec: corev1.ServiceSpec{
							Type:                  corev1.ServiceTypeLoadBalancer,
							Selector:              map[string]string{"app": "test2"},
							ExternalTrafficPolicy: corev1.ServiceExternalTrafficPolicyTypeCluster,
						},
					},
				},
				"namespace2": {
					{
						ObjectMeta: metav1.ObjectMeta{Name: "service-with-selector-and-ports-match"},
						Spec: corev1.ServiceSpec{
							Type:     corev1.ServiceTypeNodePort,
							Selector: map[string]string{"app": "test"},
							Ports: []corev1.ServicePort{
								{
									Port:       80,
									Protocol:   corev1.ProtocolTCP,
									TargetPort: intstr.FromInt(80),
								},
							},
							ExternalTrafficPolicy: corev1.ServiceExternalTrafficPolicyTypeCluster,
						},
					},
					{
						ObjectMeta: metav1.ObjectMeta{Name: "service-with-selector-and-ports-no-match"},
						Spec: corev1.ServiceSpec{
							Type:     corev1.ServiceTypeNodePort,
							Selector: map[string]string{"app": "test"},
							Ports: []corev1.ServicePort{
								{
									Port:       80,
									Protocol:   corev1.ProtocolTCP,
									TargetPort: intstr.FromInt(80),
								},
								{
									Port:       90,
									Protocol:   corev1.ProtocolTCP,
									TargetPort: intstr.FromInt(90),
								},
							},
							ExternalTrafficPolicy: corev1.ServiceExternalTrafficPolicyTypeCluster,
						},
					},
				},
				"namespace3": {
					{
						ObjectMeta: metav1.ObjectMeta{Name: "service-with-selector-and-ports-match"},
						Spec: corev1.ServiceSpec{
							Type:     corev1.ServiceTypeLoadBalancer,
							Selector: map[string]string{"app": "test"},
							Ports: []corev1.ServicePort{
								{
									Port:       80,
									Protocol:   corev1.ProtocolUDP,
									TargetPort: intstr.FromInt(80),
								},
							},
							ExternalTrafficPolicy: corev1.ServiceExternalTrafficPolicyTypeCluster,
						},
					},
					{
						ObjectMeta: metav1.ObjectMeta{Name: "service-with-selector-and-ports-no-match"},
						Spec: corev1.ServiceSpec{
							Type:     corev1.ServiceTypeLoadBalancer,
							Selector: map[string]string{"app": "test"},
							Ports: []corev1.ServicePort{
								{
									Port:       80,
									Protocol:   corev1.ProtocolTCP,
									TargetPort: intstr.FromInt(80),
								},
							},
							ExternalTrafficPolicy: corev1.ServiceExternalTrafficPolicyTypeCluster,
						},
					},
				},
			},
			policiesByNamespace: map[string][]*networkingv1.NetworkPolicy{
				"namespace1": {
					{
						ObjectMeta: metav1.ObjectMeta{Name: "allow-all-ingress-policy-with-selector"},
						Spec: networkingv1.NetworkPolicySpec{
							PodSelector: metav1.LabelSelector{
								MatchLabels: map[string]string{"app": "test"},
							},
							PolicyTypes: []networkingv1.PolicyType{"Ingress"},
							Ingress: []networkingv1.NetworkPolicyIngressRule{
								{},
							},
						},
					},
				},
				"namespace2": {
					{
						ObjectMeta: metav1.ObjectMeta{Name: "allow-all-ingress-policy-with-selector-and-ports"},
						Spec: networkingv1.NetworkPolicySpec{
							PodSelector: metav1.LabelSelector{
								MatchLabels: map[string]string{"app": "test"},
							},
							PolicyTypes: []networkingv1.PolicyType{"Ingress"},
							Ingress: []networkingv1.NetworkPolicyIngressRule{
								{
									Ports: []networkingv1.NetworkPolicyPort{
										{
											Port: intstrPtr(intstr.FromInt(80)),
											Protocol: func() *corev1.Protocol {
												protocol := corev1.ProtocolTCP
												return &protocol
											}(),
										},
									},
								},
							},
						},
					},
				},
				"namespace3": {
					{
						ObjectMeta: metav1.ObjectMeta{Name: "allow-all-ingress-policy-with-selector-and-ports"},
						Spec: networkingv1.NetworkPolicySpec{
							PodSelector: metav1.LabelSelector{
								MatchLabels: map[string]string{"app": "test"},
							},
							PolicyTypes: []networkingv1.PolicyType{"Ingress"},
							Ingress: []networkingv1.NetworkPolicyIngressRule{
								{
									Ports: []networkingv1.NetworkPolicyPort{
										{
											Protocol: func() *corev1.Protocol {
												protocol := corev1.ProtocolUDP
												return &protocol
											}(),
										},
									},
								},
							},
						},
					},
				},
			},
			expectedUnsafeServices: []string{"namespace1/service-with-selector-no-match", "namespace2/service-with-selector-and-ports-no-match", "namespace3/service-with-selector-and-ports-no-match"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			unsafeServices := getUnsafeExternalTrafficPolicyClusterServices(tt.namespaces, tt.servicesByNamespace, tt.policiesByNamespace)
			if !equal(unsafeServices, tt.expectedUnsafeServices) {
				t.Errorf("expected unsafe services %v, got %v", tt.expectedUnsafeServices, unsafeServices)
			}
		})
	}
}

// Helper to test the list output of functions
func equal(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	m := make(map[string]bool)
	for _, v := range a {
		m[v] = true
	}
	for _, v := range b {
		if !m[v] {
			return false
		}
	}
	return true
}

// Helper function to create a pointer to an intstr.IntOrString
func intstrPtr(i intstr.IntOrString) *intstr.IntOrString {
	return &i
}

// Helper function to create a pointer to an int32
func int32Ptr(i int32) *int32 {
	return &i
}
