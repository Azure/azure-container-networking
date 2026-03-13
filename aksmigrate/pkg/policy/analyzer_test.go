package policy

import (
	"testing"

	corev1 "k8s.io/api/core/v1"
	networkingv1 "k8s.io/api/networking/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"

	"github.com/azure/aksmigrate/pkg/types"
	"github.com/azure/aksmigrate/pkg/utils"
)

func TestCheckIPBlockCatchAll_BroadCIDR(t *testing.T) {
	resources := &types.ClusterResources{
		NetworkPolicies: []networkingv1.NetworkPolicy{
			{
				ObjectMeta: metav1.ObjectMeta{Name: "catch-all-egress", Namespace: "default"},
				Spec: networkingv1.NetworkPolicySpec{
					PodSelector: metav1.LabelSelector{},
					PolicyTypes: []networkingv1.PolicyType{networkingv1.PolicyTypeEgress},
					Egress: []networkingv1.NetworkPolicyEgressRule{
						{
							To: []networkingv1.NetworkPolicyPeer{
								{
									IPBlock: &networkingv1.IPBlock{
										CIDR: "0.0.0.0/0",
									},
								},
							},
						},
					},
				},
			},
		},
	}

	analyzer := NewAnalyzer(resources, "1.29")
	report := analyzer.Analyze()

	found := false
	for _, f := range report.Findings {
		if f.RuleID == types.RuleIPBlockCatchAll && f.Severity == types.SeverityFail {
			found = true
			break
		}
	}
	if !found {
		t.Error("expected FAIL finding for ipBlock catch-all 0.0.0.0/0, but none found")
	}
}

func TestCheckIPBlockCatchAll_WithSelectors_NoProblem(t *testing.T) {
	resources := &types.ClusterResources{
		NetworkPolicies: []networkingv1.NetworkPolicy{
			{
				ObjectMeta: metav1.ObjectMeta{Name: "safe-policy", Namespace: "default"},
				Spec: networkingv1.NetworkPolicySpec{
					PodSelector: metav1.LabelSelector{},
					PolicyTypes: []networkingv1.PolicyType{networkingv1.PolicyTypeEgress},
					Egress: []networkingv1.NetworkPolicyEgressRule{
						{
							To: []networkingv1.NetworkPolicyPeer{
								{
									IPBlock: &networkingv1.IPBlock{CIDR: "0.0.0.0/0"},
								},
								{
									NamespaceSelector: &metav1.LabelSelector{},
								},
							},
						},
					},
				},
			},
		},
	}

	analyzer := NewAnalyzer(resources, "1.29")
	report := analyzer.Analyze()

	for _, f := range report.Findings {
		if f.RuleID == types.RuleIPBlockCatchAll {
			t.Errorf("expected no ipBlock finding for policy with selectors, but found: %s", f.Description)
		}
	}
}

func TestCheckNamedPorts_Detected(t *testing.T) {
	httpPort := intstr.FromString("http-api")
	resources := &types.ClusterResources{
		NetworkPolicies: []networkingv1.NetworkPolicy{
			{
				ObjectMeta: metav1.ObjectMeta{Name: "named-port-policy", Namespace: "default"},
				Spec: networkingv1.NetworkPolicySpec{
					PodSelector: metav1.LabelSelector{},
					PolicyTypes: []networkingv1.PolicyType{networkingv1.PolicyTypeIngress},
					Ingress: []networkingv1.NetworkPolicyIngressRule{
						{
							Ports: []networkingv1.NetworkPolicyPort{
								{Port: &httpPort},
							},
						},
					},
				},
			},
		},
	}

	analyzer := NewAnalyzer(resources, "1.29")
	report := analyzer.Analyze()

	found := false
	for _, f := range report.Findings {
		if f.RuleID == types.RuleNamedPorts {
			found = true
			break
		}
	}
	if !found {
		t.Error("expected finding for named port usage, but none found")
	}
}

func TestCheckNamedPorts_ConflictingMappings(t *testing.T) {
	httpPort := intstr.FromString("http-api")
	resources := &types.ClusterResources{
		NetworkPolicies: []networkingv1.NetworkPolicy{
			{
				ObjectMeta: metav1.ObjectMeta{Name: "named-port-policy", Namespace: "default"},
				Spec: networkingv1.NetworkPolicySpec{
					PodSelector: metav1.LabelSelector{},
					PolicyTypes: []networkingv1.PolicyType{networkingv1.PolicyTypeIngress},
					Ingress: []networkingv1.NetworkPolicyIngressRule{
						{
							Ports: []networkingv1.NetworkPolicyPort{
								{Port: &httpPort},
							},
						},
					},
				},
			},
		},
		Pods: []corev1.Pod{
			{
				ObjectMeta: metav1.ObjectMeta{Name: "pod-1", Namespace: "default"},
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{
						{Ports: []corev1.ContainerPort{{Name: "http-api", ContainerPort: 8080}}},
					},
				},
			},
			{
				ObjectMeta: metav1.ObjectMeta{Name: "pod-2", Namespace: "default"},
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{
						{Ports: []corev1.ContainerPort{{Name: "http-api", ContainerPort: 3000}}},
					},
				},
			},
		},
	}

	analyzer := NewAnalyzer(resources, "1.29")
	report := analyzer.Analyze()

	found := false
	for _, f := range report.Findings {
		if f.RuleID == types.RuleNamedPorts && f.Severity == types.SeverityFail {
			found = true
			break
		}
	}
	if !found {
		t.Error("expected FAIL finding for conflicting named port mappings, but none found")
	}
}

func TestCheckEndPort_OldCiliumVersion(t *testing.T) {
	port := intstr.FromInt32(30000)
	endPort := int32(32767)
	resources := &types.ClusterResources{
		NetworkPolicies: []networkingv1.NetworkPolicy{
			{
				ObjectMeta: metav1.ObjectMeta{Name: "endport-policy", Namespace: "default"},
				Spec: networkingv1.NetworkPolicySpec{
					PodSelector: metav1.LabelSelector{},
					PolicyTypes: []networkingv1.PolicyType{networkingv1.PolicyTypeIngress},
					Ingress: []networkingv1.NetworkPolicyIngressRule{
						{
							Ports: []networkingv1.NetworkPolicyPort{
								{Port: &port, EndPort: &endPort},
							},
						},
					},
				},
			},
		},
	}

	// K8s 1.29 maps to Cilium 1.14.19 which does NOT support endPort
	analyzer := NewAnalyzer(resources, "1.29")
	report := analyzer.Analyze()

	found := false
	for _, f := range report.Findings {
		if f.RuleID == types.RuleEndPort && f.Severity == types.SeverityFail {
			found = true
			break
		}
	}
	if !found {
		t.Error("expected FAIL finding for endPort on Cilium < 1.17, but none found")
	}
}

func TestCheckEndPort_NewCiliumVersion_NoProblem(t *testing.T) {
	port := intstr.FromInt32(30000)
	endPort := int32(32767)
	resources := &types.ClusterResources{
		NetworkPolicies: []networkingv1.NetworkPolicy{
			{
				ObjectMeta: metav1.ObjectMeta{Name: "endport-policy", Namespace: "default"},
				Spec: networkingv1.NetworkPolicySpec{
					PodSelector: metav1.LabelSelector{},
					PolicyTypes: []networkingv1.PolicyType{networkingv1.PolicyTypeIngress},
					Ingress: []networkingv1.NetworkPolicyIngressRule{
						{
							Ports: []networkingv1.NetworkPolicyPort{
								{Port: &port, EndPort: &endPort},
							},
						},
					},
				},
			},
		},
	}

	// K8s 1.32 maps to Cilium 1.17.0 which supports endPort
	analyzer := NewAnalyzer(resources, "1.32")
	report := analyzer.Analyze()

	for _, f := range report.Findings {
		if f.RuleID == types.RuleEndPort {
			t.Errorf("expected no endPort finding for Cilium 1.17+, but found: %s", f.Description)
		}
	}
}

func TestCheckImplicitLocalNodeEgress(t *testing.T) {
	resources := &types.ClusterResources{
		NetworkPolicies: []networkingv1.NetworkPolicy{
			{
				ObjectMeta: metav1.ObjectMeta{Name: "egress-policy", Namespace: "default"},
				Spec: networkingv1.NetworkPolicySpec{
					PodSelector: metav1.LabelSelector{},
					PolicyTypes: []networkingv1.PolicyType{networkingv1.PolicyTypeEgress},
					Egress: []networkingv1.NetworkPolicyEgressRule{
						{
							To: []networkingv1.NetworkPolicyPeer{
								{PodSelector: &metav1.LabelSelector{MatchLabels: map[string]string{"app": "db"}}},
							},
						},
					},
				},
			},
		},
	}

	analyzer := NewAnalyzer(resources, "1.29")
	report := analyzer.Analyze()

	found := false
	for _, f := range report.Findings {
		if f.RuleID == types.RuleImplicitLocalNodeEgress {
			found = true
			break
		}
	}
	if !found {
		t.Error("expected finding for implicit local node egress, but none found")
	}
}

func TestCheckLBIngressEnforcement(t *testing.T) {
	resources := &types.ClusterResources{
		NetworkPolicies: []networkingv1.NetworkPolicy{
			{
				ObjectMeta: metav1.ObjectMeta{Name: "deny-all-ingress", Namespace: "default"},
				Spec: networkingv1.NetworkPolicySpec{
					PodSelector: metav1.LabelSelector{},
					PolicyTypes: []networkingv1.PolicyType{networkingv1.PolicyTypeIngress},
				},
			},
		},
		Services: []corev1.Service{
			{
				ObjectMeta: metav1.ObjectMeta{Name: "my-lb", Namespace: "default"},
				Spec: corev1.ServiceSpec{
					Type:     corev1.ServiceTypeLoadBalancer,
					Selector: map[string]string{"app": "web"},
					Ports: []corev1.ServicePort{
						{Port: 80, Protocol: corev1.ProtocolTCP},
					},
					ExternalTrafficPolicy: corev1.ServiceExternalTrafficPolicyCluster,
				},
			},
		},
	}

	analyzer := NewAnalyzer(resources, "1.29")
	report := analyzer.Analyze()

	found := false
	for _, f := range report.Findings {
		if f.RuleID == types.RuleLBIngressEnforcement && f.Severity == types.SeverityFail {
			found = true
			break
		}
	}
	if !found {
		t.Error("expected FAIL finding for LB ingress enforcement, but none found")
	}
}

func TestCheckHostNetworkPods(t *testing.T) {
	resources := &types.ClusterResources{
		NetworkPolicies: []networkingv1.NetworkPolicy{
			{
				ObjectMeta: metav1.ObjectMeta{Name: "monitor-policy", Namespace: "monitoring"},
				Spec: networkingv1.NetworkPolicySpec{
					PodSelector: metav1.LabelSelector{
						MatchLabels: map[string]string{"app": "node-exporter"},
					},
					PolicyTypes: []networkingv1.PolicyType{networkingv1.PolicyTypeIngress},
				},
			},
		},
		Pods: []corev1.Pod{
			{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "node-exporter-abc",
					Namespace: "monitoring",
					Labels:    map[string]string{"app": "node-exporter"},
				},
				Spec: corev1.PodSpec{
					HostNetwork: true,
					Containers:  []corev1.Container{{Name: "exporter", Image: "prom/node-exporter"}},
				},
			},
		},
	}

	analyzer := NewAnalyzer(resources, "1.29")
	report := analyzer.Analyze()

	found := false
	for _, f := range report.Findings {
		if f.RuleID == types.RuleHostNetworkPods {
			found = true
			break
		}
	}
	if !found {
		t.Error("expected finding for host-networked pod with NetworkPolicy, but none found")
	}
}

func TestCheckServiceMesh_Istio(t *testing.T) {
	resources := &types.ClusterResources{
		Pods: []corev1.Pod{
			{
				ObjectMeta: metav1.ObjectMeta{Name: "app-1", Namespace: "default"},
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{
						{Name: "app", Image: "myapp:latest"},
						{Name: "istio-proxy", Image: "docker.io/istio/proxyv2:1.20.0"},
					},
					InitContainers: []corev1.Container{
						{Name: "istio-init", Image: "docker.io/istio/proxyv2:1.20.0"},
					},
				},
			},
		},
	}

	analyzer := NewAnalyzer(resources, "1.29")
	report := analyzer.Analyze()

	found := false
	for _, f := range report.Findings {
		if f.RuleID == types.RuleServiceMeshDetected {
			found = true
			break
		}
	}
	if !found {
		t.Error("expected finding for Istio service mesh detection, but none found")
	}
}

func TestIsBroadCIDR(t *testing.T) {
	tests := []struct {
		cidr     string
		expected bool
	}{
		{"0.0.0.0/0", true},
		{"10.0.0.0/8", true},
		{"172.16.0.0/12", true},
		{"192.168.0.0/16", true},
		{"10.0.0.0/24", false},
		{"192.168.1.0/24", false},
		{"10.0.1.5/32", false},
	}

	for _, tt := range tests {
		t.Run(tt.cidr, func(t *testing.T) {
			result := isBroadCIDR(tt.cidr)
			if result != tt.expected {
				t.Errorf("isBroadCIDR(%q) = %v, want %v", tt.cidr, result, tt.expected)
			}
		})
	}
}

func TestSupportsEndPort(t *testing.T) {
	tests := []struct {
		version  string
		expected bool
	}{
		{"1.14.19", false},
		{"1.16.6", false},
		{"1.17.0", true},
		{"1.18.0", true},
	}

	for _, tt := range tests {
		t.Run(tt.version, func(t *testing.T) {
			result := supportsEndPort(tt.version)
			if result != tt.expected {
				t.Errorf("supportsEndPort(%q) = %v, want %v", tt.version, result, tt.expected)
			}
		})
	}
}

func TestFullAuditFromDirectory(t *testing.T) {
	// This test loads from the test/policies directory and verifies we get findings
	resources, err := loadTestResources()
	if err != nil {
		t.Skipf("skipping integration test: %v", err)
	}

	analyzer := NewAnalyzer(resources, "1.29")
	report := analyzer.Analyze()

	if report.TotalPolicies == 0 {
		t.Error("expected at least one policy to be loaded from test fixtures")
	}

	if report.Summary.FailCount == 0 {
		t.Error("expected at least one FAIL finding from test fixtures")
	}

	// Verify we always get the kube-proxy removal info
	foundKubeProxy := false
	for _, f := range report.Findings {
		if f.RuleID == types.RuleKubeProxyRemoval {
			foundKubeProxy = true
			break
		}
	}
	if !foundKubeProxy {
		t.Error("expected kube-proxy removal info finding")
	}
}

func loadTestResources() (*types.ClusterResources, error) {
	return utils.LoadFromDirectory("../../test/policies")
}
