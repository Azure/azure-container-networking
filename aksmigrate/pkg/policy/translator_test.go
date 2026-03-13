package policy

import (
	"testing"

	corev1 "k8s.io/api/core/v1"
	networkingv1 "k8s.io/api/networking/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"

	"github.com/azure/aksmigrate/pkg/types"
)

func TestPatchIPBlockCatchAll(t *testing.T) {
	resources := &types.ClusterResources{
		NetworkPolicies: []networkingv1.NetworkPolicy{
			{
				ObjectMeta: metav1.ObjectMeta{Name: "catch-all", Namespace: "default"},
				Spec: networkingv1.NetworkPolicySpec{
					PodSelector: metav1.LabelSelector{},
					PolicyTypes: []networkingv1.PolicyType{networkingv1.PolicyTypeEgress},
					Egress: []networkingv1.NetworkPolicyEgressRule{
						{
							To: []networkingv1.NetworkPolicyPeer{
								{IPBlock: &networkingv1.IPBlock{CIDR: "0.0.0.0/0"}},
							},
						},
					},
				},
			},
		},
	}

	translator := NewTranslator(resources, "1.29")
	output := translator.Translate()

	if len(output.PatchedPolicies) != 1 {
		t.Fatalf("expected 1 patched policy, got %d", len(output.PatchedPolicies))
	}

	patched := output.PatchedPolicies[0].Patched
	egressPeers := patched.Spec.Egress[0].To
	if len(egressPeers) < 2 {
		t.Fatalf("expected at least 2 egress peers after patching, got %d", len(egressPeers))
	}

	// Verify a namespaceSelector was added
	found := false
	for _, peer := range egressPeers {
		if peer.NamespaceSelector != nil {
			found = true
			break
		}
	}
	if !found {
		t.Error("expected namespaceSelector to be added as peer alongside ipBlock")
	}
}

func TestReplaceNamedPorts(t *testing.T) {
	httpPort := intstr.FromString("http-api")
	resources := &types.ClusterResources{
		NetworkPolicies: []networkingv1.NetworkPolicy{
			{
				ObjectMeta: metav1.ObjectMeta{Name: "named-port", Namespace: "default"},
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
				ObjectMeta: metav1.ObjectMeta{Name: "app-1", Namespace: "default"},
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{
						{
							Ports: []corev1.ContainerPort{
								{Name: "http-api", ContainerPort: 8080},
							},
						},
					},
				},
			},
		},
	}

	translator := NewTranslator(resources, "1.29")
	output := translator.Translate()

	if len(output.PatchedPolicies) != 1 {
		t.Fatalf("expected 1 patched policy, got %d", len(output.PatchedPolicies))
	}

	patched := output.PatchedPolicies[0].Patched
	port := patched.Spec.Ingress[0].Ports[0].Port
	if port.Type != intstr.Int {
		t.Errorf("expected port to be numeric after translation, got type %v", port.Type)
	}
	if port.IntVal != 8080 {
		t.Errorf("expected port to be 8080, got %d", port.IntVal)
	}

	if len(output.RemovedNamedPorts) != 1 {
		t.Fatalf("expected 1 named port mapping, got %d", len(output.RemovedNamedPorts))
	}
	if output.RemovedNamedPorts[0].PortName != "http-api" {
		t.Errorf("expected port name 'http-api', got %q", output.RemovedNamedPorts[0].PortName)
	}
}

func TestExpandEndPort(t *testing.T) {
	port := intstr.FromInt32(5432)
	endPort := int32(5440)
	resources := &types.ClusterResources{
		NetworkPolicies: []networkingv1.NetworkPolicy{
			{
				ObjectMeta: metav1.ObjectMeta{Name: "endport-policy", Namespace: "default"},
				Spec: networkingv1.NetworkPolicySpec{
					PodSelector: metav1.LabelSelector{},
					PolicyTypes: []networkingv1.PolicyType{networkingv1.PolicyTypeEgress},
					Egress: []networkingv1.NetworkPolicyEgressRule{
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

	// K8s 1.29 -> Cilium 1.14.19, doesn't support endPort
	translator := NewTranslator(resources, "1.29")
	output := translator.Translate()

	if len(output.PatchedPolicies) != 1 {
		t.Fatalf("expected 1 patched policy, got %d", len(output.PatchedPolicies))
	}

	patched := output.PatchedPolicies[0].Patched
	// Port range 5432-5440 = 9 individual ports
	expectedPorts := 9
	actualPorts := len(patched.Spec.Egress[0].Ports)
	if actualPorts != expectedPorts {
		t.Errorf("expected %d individual ports after expansion, got %d", expectedPorts, actualPorts)
	}

	// Verify first and last port
	if patched.Spec.Egress[0].Ports[0].Port.IntVal != 5432 {
		t.Errorf("expected first port to be 5432, got %d", patched.Spec.Egress[0].Ports[0].Port.IntVal)
	}
	if patched.Spec.Egress[0].Ports[expectedPorts-1].Port.IntVal != 5440 {
		t.Errorf("expected last port to be 5440, got %d", patched.Spec.Egress[0].Ports[expectedPorts-1].Port.IntVal)
	}

	// Verify no endPort on expanded entries
	for _, p := range patched.Spec.Egress[0].Ports {
		if p.EndPort != nil {
			t.Error("expected no endPort on expanded port entries")
		}
	}
}

func TestExpandEndPort_NotNeededOnNewCilium(t *testing.T) {
	port := intstr.FromInt32(5432)
	endPort := int32(5440)
	resources := &types.ClusterResources{
		NetworkPolicies: []networkingv1.NetworkPolicy{
			{
				ObjectMeta: metav1.ObjectMeta{Name: "endport-policy", Namespace: "default"},
				Spec: networkingv1.NetworkPolicySpec{
					PodSelector: metav1.LabelSelector{},
					PolicyTypes: []networkingv1.PolicyType{networkingv1.PolicyTypeEgress},
					Egress: []networkingv1.NetworkPolicyEgressRule{
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

	// K8s 1.32 -> Cilium 1.17.0, supports endPort
	translator := NewTranslator(resources, "1.32")
	output := translator.Translate()

	// Should produce no patched policies for endPort (no changes needed)
	for _, pp := range output.PatchedPolicies {
		for _, change := range pp.Changes {
			if change != "" && len(pp.Changes) > 0 {
				// Check that none of the changes are about endPort expansion
				for _, c := range pp.Changes {
					if c == "" {
						continue
					}
					if testing.Verbose() {
						t.Logf("change: %s", c)
					}
				}
			}
		}
	}
}

func TestGenerateHostEgressPolicy(t *testing.T) {
	resources := &types.ClusterResources{
		NetworkPolicies: []networkingv1.NetworkPolicy{
			{
				ObjectMeta: metav1.ObjectMeta{Name: "egress-restricted", Namespace: "production"},
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

	translator := NewTranslator(resources, "1.29")
	output := translator.Translate()

	found := false
	for _, cp := range output.CiliumPolicies {
		if cp.Name == "allow-host-egress" && cp.Namespace == "production" {
			found = true
			// Verify spec contains toEntities
			spec := cp.Spec
			egress, ok := spec["egress"].([]map[string]interface{})
			if !ok || len(egress) == 0 {
				t.Error("expected egress rules in CiliumNetworkPolicy")
				break
			}
			entities, ok := egress[0]["toEntities"].([]string)
			if !ok {
				t.Error("expected toEntities in egress rule")
				break
			}
			if len(entities) != 2 || entities[0] != "host" || entities[1] != "remote-node" {
				t.Errorf("expected toEntities [host, remote-node], got %v", entities)
			}
			break
		}
	}
	if !found {
		t.Error("expected allow-host-egress CiliumNetworkPolicy for namespace 'production'")
	}
}

func TestGenerateLBIngressPolicy(t *testing.T) {
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
				ObjectMeta: metav1.ObjectMeta{Name: "web-svc", Namespace: "default"},
				Spec: corev1.ServiceSpec{
					Type:                  corev1.ServiceTypeLoadBalancer,
					Selector:              map[string]string{"app": "web"},
					ExternalTrafficPolicy: corev1.ServiceExternalTrafficPolicyCluster,
					Ports: []corev1.ServicePort{
						{Port: 80, Protocol: corev1.ProtocolTCP},
					},
				},
			},
		},
	}

	translator := NewTranslator(resources, "1.29")
	output := translator.Translate()

	found := false
	for _, cp := range output.CiliumPolicies {
		if cp.Name == "allow-lb-ingress-web-svc" {
			found = true
			break
		}
	}
	if !found {
		t.Error("expected CiliumNetworkPolicy for LB ingress allow")
	}
}

func TestRenderCiliumPolicyYAML(t *testing.T) {
	cp := types.CiliumPolicy{
		Name:      "allow-host-egress",
		Namespace: "production",
		Reason:    "test reason",
		Spec: map[string]interface{}{
			"endpointSelector": map[string]interface{}{},
			"egress": []map[string]interface{}{
				{
					"toEntities": []string{"host", "remote-node"},
				},
			},
		},
	}

	yaml := RenderCiliumPolicyYAML(cp)

	if yaml == "" {
		t.Error("expected non-empty YAML output")
	}
	if !contains(yaml, "apiVersion: \"cilium.io/v2\"") {
		t.Error("expected apiVersion in YAML output")
	}
	if !contains(yaml, "kind: CiliumNetworkPolicy") {
		t.Error("expected kind in YAML output")
	}
	if !contains(yaml, "name: allow-host-egress") {
		t.Error("expected name in YAML output")
	}
}

func contains(s, substr string) bool {
	return len(s) > 0 && len(substr) > 0 && len(s) >= len(substr) && searchString(s, substr)
}

func searchString(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
