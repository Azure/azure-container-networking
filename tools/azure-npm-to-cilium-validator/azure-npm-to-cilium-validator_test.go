package main

import (
	"testing"

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
			expectedIngressEndportPolicies: []string{"namespace1/ingress-and-egress-endport-policy"},
			expectedEgressEndportPolicies:  []string{"namespace1/egress-endport-policy", "namespace1/ingress-and-egress-endport-policy"},
		},
		{
			name: "Multiple polices in multiple namespaces with ingress or egress endport or no endport",
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
			expectedIngressEndportPolicies: []string{"namespace1/ingress-and-egress-endport-policy", "namespace2/ingress-endport-policy"},
			expectedEgressEndportPolicies:  []string{"namespace1/egress-endport-policy", "namespace1/ingress-and-egress-endport-policy"},
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
			name: "Multiple namespaces and policies",
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
				},
			},
			expectedIngressCIDRPolicies: []string{"namespace1/ingress-cidr-policy"},
			expectedEgressCIDRPolicies:  []string{"namespace2/egress-cidr-policy"},
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
			expectedEgressPolicies: []string{},
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
			name: "Multiple namespaces and policies",
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
				},
			},
			expectedEgressPolicies: []string{"namespace1/egress-to-policy", "namespace2/egress-ports-policy"},
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
