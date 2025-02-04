package main

import (
	"bytes"
	"fmt"
	"log"
	"strings"
	"testing"

	corev1 "k8s.io/api/core/v1"
	networkingv1 "k8s.io/api/networking/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	intstr "k8s.io/apimachinery/pkg/util/intstr"
)

// Test function for checkEndportNetworkPolicies
func TestCheckEndportNetworkPolicies(t *testing.T) {
	tests := []struct {
		name                string
		policiesByNamespace map[string][]networkingv1.NetworkPolicy
		expectedResult      bool
		expectedLogs        []string
	}{
		{
			name: "Egress endPort policy present",
			policiesByNamespace: map[string][]networkingv1.NetworkPolicy{
				"default": {
					{
						ObjectMeta: metav1.ObjectMeta{Name: "policy1"},
						Spec: networkingv1.NetworkPolicySpec{
							Egress: []networkingv1.NetworkPolicyEgressRule{
								{
									Ports: []networkingv1.NetworkPolicyPort{
										{
											EndPort: int32Ptr(8080),
										},
									},
								},
							},
						},
					},
				},
			},
			expectedResult: true,
			expectedLogs: []string{
				"NetworkPolicy with endPort",
				"❌ Found NetworkPolicy: \033[31mpolicy1\033[0m with egress endPort field in namespace: \033[31mdefault\033[0m\n",
			},
		},
		{
			name: "Ingress endPort policy present",
			policiesByNamespace: map[string][]networkingv1.NetworkPolicy{
				"default": {
					{
						ObjectMeta: metav1.ObjectMeta{Name: "policy2"},
						Spec: networkingv1.NetworkPolicySpec{
							Ingress: []networkingv1.NetworkPolicyIngressRule{
								{
									Ports: []networkingv1.NetworkPolicyPort{
										{
											EndPort: int32Ptr(8080),
										},
									},
								},
							},
						},
					},
				},
			},
			expectedResult: true,
			expectedLogs: []string{
				"NetworkPolicy with endPort",
				"❌ Found NetworkPolicy: \033[31mpolicy2\033[0m with ingress endPort field in namespace: \033[31mdefault\033[0m\n",
			},
		},
		{
			name: "No endPort policy present",
			policiesByNamespace: map[string][]networkingv1.NetworkPolicy{
				"default": {
					{
						ObjectMeta: metav1.ObjectMeta{Name: "policy3"},
						Spec: networkingv1.NetworkPolicySpec{
							Egress: []networkingv1.NetworkPolicyEgressRule{
								{
									Ports: []networkingv1.NetworkPolicyPort{
										{
											Port: &intstr.IntOrString{IntVal: 80},
										},
									},
								},
							},
						},
					},
				},
			},
			expectedResult: false,
			expectedLogs: []string{
				"NetworkPolicy with endPort",
				"✅",
			},
		},
		{
			name: "Empty policies",
			policiesByNamespace: map[string][]networkingv1.NetworkPolicy{
				"default": {},
			},
			expectedResult: false,
			expectedLogs: []string{
				"NetworkPolicy with endPort",
				"✅",
			},
		},
	}

	runTestWithLogs(t, tests, checkEndportNetworkPolicies)
}

// Test function for checkCIDRNetworkPolicies
func TestCheckCIDRNetworkPolicies(t *testing.T) {
	tests := []struct {
		name                string
		policiesByNamespace map[string][]networkingv1.NetworkPolicy
		expectedResult      bool
		expectedLogs        []string
	}{
		{
			name: "Ingress CIDR policy present",
			policiesByNamespace: map[string][]networkingv1.NetworkPolicy{
				"default": {
					{
						ObjectMeta: metav1.ObjectMeta{Name: "policy1"},
						Spec: networkingv1.NetworkPolicySpec{
							Ingress: []networkingv1.NetworkPolicyIngressRule{
								{
									From: []networkingv1.NetworkPolicyPeer{
										{
											IPBlock: &networkingv1.IPBlock{
												CIDR: "192.168.0.0/16",
											},
										},
									},
								},
							},
						},
					},
				},
			},
			expectedResult: true,
			expectedLogs: []string{
				"NetworkPolicy with cidr",
				"❌ Found NetworkPolicy: \033[31mpolicy1\033[0m with ingress cidr field in namespace: \033[31mdefault\033[0m",
			},
		},
		{
			name: "Egress CIDR policy present",
			policiesByNamespace: map[string][]networkingv1.NetworkPolicy{
				"default": {
					{
						ObjectMeta: metav1.ObjectMeta{Name: "policy2"},
						Spec: networkingv1.NetworkPolicySpec{
							Egress: []networkingv1.NetworkPolicyEgressRule{
								{
									To: []networkingv1.NetworkPolicyPeer{
										{
											IPBlock: &networkingv1.IPBlock{
												CIDR: "192.168.0.0/16",
											},
										},
									},
								},
							},
						},
					},
				},
			},
			expectedResult: true,
			expectedLogs: []string{
				"NetworkPolicy with cidr",
				"❌ Found NetworkPolicy: \033[31mpolicy2\033[0m with egress cidr field in namespace: \033[31mdefault\033[0m",
			},
		},
		{
			name: "No CIDR policy present",
			policiesByNamespace: map[string][]networkingv1.NetworkPolicy{
				"default": {
					{
						ObjectMeta: metav1.ObjectMeta{Name: "policy3"},
						Spec: networkingv1.NetworkPolicySpec{
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
			expectedResult: false,
			expectedLogs: []string{
				"NetworkPolicy with cidr",
				"✅",
			},
		},
		{
			name: "Empty policies",
			policiesByNamespace: map[string][]networkingv1.NetworkPolicy{
				"default": {},
			},
			expectedResult: false,
			expectedLogs: []string{
				"NetworkPolicy with cidr",
				"✅",
			},
		},
	}

	runTestWithLogs(t, tests, checkCIDRNetworkPolicies)
}

// Test function for checkForEgressPolicies
func TestCheckForEgressPolicies(t *testing.T) {
	tests := []struct {
		name                string
		policiesByNamespace map[string][]networkingv1.NetworkPolicy
		expectedResult      bool
		expectedLogs        []string
	}{
		{
			name: "Egress policy present",
			policiesByNamespace: map[string][]networkingv1.NetworkPolicy{
				"default": {
					{
						ObjectMeta: metav1.ObjectMeta{Name: "policy1"},
						Spec: networkingv1.NetworkPolicySpec{
							Egress: []networkingv1.NetworkPolicyEgressRule{
								{
									To: []networkingv1.NetworkPolicyPeer{
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
			expectedResult: true,
			expectedLogs: []string{
				"NetworkPolicy with egress",
				"(Not allow all egress)",
				"❌ Found NetworkPolicy: \033[31mpolicy1\033[0m with egress field (non-allow all) in namespace: \033[31mdefault\033[0m",
			},
		},
		{
			name: "Allow all egress policy present",
			policiesByNamespace: map[string][]networkingv1.NetworkPolicy{
				"default": {
					{
						ObjectMeta: metav1.ObjectMeta{Name: "policy2"},
						Spec: networkingv1.NetworkPolicySpec{
							Egress: []networkingv1.NetworkPolicyEgressRule{},
						},
					},
				},
			},
			expectedResult: false,
			expectedLogs: []string{
				"NetworkPolicy with egress",
				"✅",
			},
		},
		{
			name: "No egress policy present",
			policiesByNamespace: map[string][]networkingv1.NetworkPolicy{
				"default": {
					{
						ObjectMeta: metav1.ObjectMeta{Name: "policy3"},
						Spec: networkingv1.NetworkPolicySpec{
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
			expectedResult: false,
			expectedLogs: []string{
				"NetworkPolicy with egress",
				"✅",
			},
		},
		{
			name: "Empty policies",
			policiesByNamespace: map[string][]networkingv1.NetworkPolicy{
				"default": {},
			},
			expectedResult: false,
			expectedLogs: []string{
				"NetworkPolicy with egress",
				"✅",
			},
		},
	}

	runTestWithLogs(t, tests, checkForEgressPolicies)
}

// Test function for checkExternalTrafficPolicyServices
func TestCheckExternalTrafficPolicyServices(t *testing.T) {
	namespaces := &corev1.NamespaceList{
		Items: []corev1.Namespace{
			{
				ObjectMeta: metav1.ObjectMeta{
					Name: "default",
				},
			},
		},
	}

	servicesByNamespace := map[string][]corev1.Service{
		"default": {
			{
				Spec: corev1.ServiceSpec{
					Type:                  corev1.ServiceTypeLoadBalancer,
					ExternalTrafficPolicy: corev1.ServiceExternalTrafficPolicyTypeCluster,
				},
			},
		},
	}

	policiesByNamespace := map[string][]networkingv1.NetworkPolicy{}

	result := checkExternalTrafficPolicyServices(namespaces, servicesByNamespace, policiesByNamespace)
	fmt.Println(result)
	// if !result {
	// 	t.Errorf("Expected true, got %v", result)
	// }
}

// Helper function to run tests and verify logs
func runTestWithLogs(t *testing.T, tests []struct {
	name                string
	policiesByNamespace map[string][]networkingv1.NetworkPolicy
	expectedResult      bool
	expectedLogs        []string
}, testFunc func(map[string][]networkingv1.NetworkPolicy) bool) {
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Capture the logs
			var buf bytes.Buffer
			log.SetOutput(&buf)
			defer log.SetOutput(nil)

			result := testFunc(tt.policiesByNamespace)
			if result != tt.expectedResult {
				t.Errorf("Expected %v, got %v", tt.expectedResult, result)
			}

			// Verify logs
			logOutput := buf.String()
			for _, expectedLog := range tt.expectedLogs {
				if !strings.Contains(logOutput, expectedLog) {
					t.Errorf("Expected log containing %q, but not found", expectedLog)
				}
			}
		})
	}
}

// Helper function to create a pointer to an int32
func int32Ptr(i int32) *int32 {
	return &i
}
