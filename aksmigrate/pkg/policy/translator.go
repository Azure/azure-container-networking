package policy

import (
	"fmt"
	"strings"

	corev1 "k8s.io/api/core/v1"
	networkingv1 "k8s.io/api/networking/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"

	"github.com/azure/aksmigrate/pkg/types"
)

// Translator generates patched NetworkPolicies and CiliumNetworkPolicies
// to maintain behavioral equivalence after migration from NPM to Cilium.
type Translator struct {
	resources     *types.ClusterResources
	k8sVersion    string
	ciliumVersion string
}

// NewTranslator creates a new policy translator.
func NewTranslator(resources *types.ClusterResources, k8sVersion string) *Translator {
	return &Translator{
		resources:     resources,
		k8sVersion:    k8sVersion,
		ciliumVersion: ciliumVersionForK8s(k8sVersion),
	}
}

// Translate processes all NetworkPolicies and generates the necessary patches
// and supplementary CiliumNetworkPolicies.
func (t *Translator) Translate() *types.TranslationOutput {
	output := &types.TranslationOutput{}

	namespacesNeedingHostEgress := make(map[string]bool)

	for i := range t.resources.NetworkPolicies {
		np := t.resources.NetworkPolicies[i].DeepCopy()
		original := t.resources.NetworkPolicies[i].DeepCopy()
		var changes []string

		// Fix 1: Patch ipBlock catch-all rules
		ipBlockChanges := t.patchIPBlockCatchAll(np)
		changes = append(changes, ipBlockChanges...)

		// Fix 2: Replace named ports with numeric values
		namedPortChanges, mappings := t.replaceNamedPorts(np)
		changes = append(changes, namedPortChanges...)
		output.RemovedNamedPorts = append(output.RemovedNamedPorts, mappings...)

		// Fix 3: Expand endPort ranges if Cilium version doesn't support them
		endPortChanges := t.expandEndPort(np)
		changes = append(changes, endPortChanges...)

		// Track namespaces that need host egress CiliumNetworkPolicy
		hasEgress := false
		for _, pt := range np.Spec.PolicyTypes {
			if pt == networkingv1.PolicyTypeEgress {
				hasEgress = true
				break
			}
		}
		if hasEgress {
			namespacesNeedingHostEgress[np.Namespace] = true
		}

		if len(changes) > 0 {
			output.PatchedPolicies = append(output.PatchedPolicies, types.PatchedPolicy{
				Original: original,
				Patched:  np,
				Changes:  changes,
			})
		}
	}

	// Generate CiliumNetworkPolicies for host/remote-node egress
	for ns := range namespacesNeedingHostEgress {
		output.CiliumPolicies = append(output.CiliumPolicies, t.generateHostEgressPolicy(ns))
	}

	// Generate CiliumNetworkPolicies for LB ingress allow
	lbPolicies := t.generateLBIngressPolicies()
	output.CiliumPolicies = append(output.CiliumPolicies, lbPolicies...)

	return output
}

// patchIPBlockCatchAll adds namespaceSelector/podSelector peers alongside broad ipBlock rules.
func (t *Translator) patchIPBlockCatchAll(np *networkingv1.NetworkPolicy) []string {
	var changes []string

	for i := range np.Spec.Egress {
		rule := &np.Spec.Egress[i]
		needsSelectors := false
		for _, peer := range rule.To {
			if peer.IPBlock != nil && isBroadCIDR(peer.IPBlock.CIDR) {
				if !hasSelectorPeers(rule.To) {
					needsSelectors = true
					break
				}
			}
		}
		if needsSelectors {
			rule.To = append(rule.To,
				networkingv1.NetworkPolicyPeer{
					NamespaceSelector: &metav1.LabelSelector{},
				},
			)
			changes = append(changes, fmt.Sprintf("egress[%d]: added namespaceSelector: {} to cover pod-to-pod traffic alongside ipBlock", i))
		}
	}

	for i := range np.Spec.Ingress {
		rule := &np.Spec.Ingress[i]
		needsSelectors := false
		for _, peer := range rule.From {
			if peer.IPBlock != nil && isBroadCIDR(peer.IPBlock.CIDR) {
				if !hasSelectorPeers(rule.From) {
					needsSelectors = true
					break
				}
			}
		}
		if needsSelectors {
			rule.From = append(rule.From,
				networkingv1.NetworkPolicyPeer{
					NamespaceSelector: &metav1.LabelSelector{},
				},
			)
			changes = append(changes, fmt.Sprintf("ingress[%d]: added namespaceSelector: {} to cover cluster-internal traffic alongside ipBlock", i))
		}
	}

	return changes
}

// replaceNamedPorts resolves named ports to their numeric values by inspecting pod specs.
func (t *Translator) replaceNamedPorts(np *networkingv1.NetworkPolicy) ([]string, []types.NamedPortMapping) {
	var changes []string
	var mappings []types.NamedPortMapping

	portMap := t.buildPortNameMap(np)

	for i := range np.Spec.Ingress {
		for j := range np.Spec.Ingress[i].Ports {
			p := &np.Spec.Ingress[i].Ports[j]
			if p.Port != nil && p.Port.Type == intstr.String {
				if num, ok := portMap[p.Port.StrVal]; ok {
					proto := "TCP"
					if p.Protocol != nil {
						proto = string(*p.Protocol)
					}
					mappings = append(mappings, types.NamedPortMapping{
						PolicyName: np.Name,
						Namespace:  np.Namespace,
						PortName:   p.Port.StrVal,
						PortNumber: num,
						Protocol:   proto,
					})
					changes = append(changes, fmt.Sprintf("ingress[%d].ports[%d]: replaced named port %q with %d", i, j, p.Port.StrVal, num))
					numPort := intstr.FromInt32(num)
					p.Port = &numPort
				}
			}
		}
	}

	for i := range np.Spec.Egress {
		for j := range np.Spec.Egress[i].Ports {
			p := &np.Spec.Egress[i].Ports[j]
			if p.Port != nil && p.Port.Type == intstr.String {
				if num, ok := portMap[p.Port.StrVal]; ok {
					proto := "TCP"
					if p.Protocol != nil {
						proto = string(*p.Protocol)
					}
					mappings = append(mappings, types.NamedPortMapping{
						PolicyName: np.Name,
						Namespace:  np.Namespace,
						PortName:   p.Port.StrVal,
						PortNumber: num,
						Protocol:   proto,
					})
					changes = append(changes, fmt.Sprintf("egress[%d].ports[%d]: replaced named port %q with %d", i, j, p.Port.StrVal, num))
					numPort := intstr.FromInt32(num)
					p.Port = &numPort
				}
			}
		}
	}

	return changes, mappings
}

// expandEndPort replaces port ranges (port + endPort) with individual port entries
// when the target Cilium version doesn't support endPort.
func (t *Translator) expandEndPort(np *networkingv1.NetworkPolicy) []string {
	if supportsEndPort(t.ciliumVersion) {
		return nil
	}

	var changes []string

	for i := range np.Spec.Egress {
		rule := &np.Spec.Egress[i]
		var newPorts []networkingv1.NetworkPolicyPort
		for j, p := range rule.Ports {
			if p.EndPort != nil && p.Port != nil {
				startPort := p.Port.IntVal
				endPort := *p.EndPort
				// Only expand reasonable ranges (up to 100 ports)
				if endPort-startPort <= 100 {
					for port := startPort; port <= endPort; port++ {
						np := networkingv1.NetworkPolicyPort{
							Protocol: p.Protocol,
							Port:     portPtr(int32(port)),
						}
						newPorts = append(newPorts, np)
					}
					changes = append(changes, fmt.Sprintf("egress[%d].ports[%d]: expanded port range %d-%d into %d individual entries", i, j, startPort, endPort, endPort-startPort+1))
				} else {
					// Range too large, keep as-is and warn
					newPorts = append(newPorts, p)
					changes = append(changes, fmt.Sprintf("egress[%d].ports[%d]: port range %d-%d too large to expand (>100 ports), kept as-is", i, j, startPort, endPort))
				}
			} else {
				newPorts = append(newPorts, p)
			}
		}
		rule.Ports = newPorts
	}

	for i := range np.Spec.Ingress {
		rule := &np.Spec.Ingress[i]
		var newPorts []networkingv1.NetworkPolicyPort
		for j, p := range rule.Ports {
			if p.EndPort != nil && p.Port != nil {
				startPort := p.Port.IntVal
				endPort := *p.EndPort
				if endPort-startPort <= 100 {
					for port := startPort; port <= endPort; port++ {
						np := networkingv1.NetworkPolicyPort{
							Protocol: p.Protocol,
							Port:     portPtr(int32(port)),
						}
						newPorts = append(newPorts, np)
					}
					changes = append(changes, fmt.Sprintf("ingress[%d].ports[%d]: expanded port range %d-%d into %d individual entries", i, j, startPort, endPort, endPort-startPort+1))
				} else {
					newPorts = append(newPorts, p)
					changes = append(changes, fmt.Sprintf("ingress[%d].ports[%d]: port range %d-%d too large to expand, kept as-is", i, j, startPort, endPort))
				}
			} else {
				newPorts = append(newPorts, p)
			}
		}
		rule.Ports = newPorts
	}

	return changes
}

// generateHostEgressPolicy creates a CiliumNetworkPolicy that allows egress to host and remote-node entities.
func (t *Translator) generateHostEgressPolicy(namespace string) types.CiliumPolicy {
	return types.CiliumPolicy{
		Name:      "allow-host-egress",
		Namespace: namespace,
		Reason:    "Restores implicit local node egress access that NPM provided. Cilium blocks egress to host/remote-node unless explicitly allowed.",
		Spec: map[string]interface{}{
			"endpointSelector": map[string]interface{}{},
			"egress": []map[string]interface{}{
				{
					"toEntities": []string{"host", "remote-node"},
				},
			},
		},
	}
}

// generateLBIngressPolicies creates CiliumNetworkPolicies for pods behind LoadBalancer/NodePort
// services that have ingress restrictions.
func (t *Translator) generateLBIngressPolicies() []types.CiliumPolicy {
	var policies []types.CiliumPolicy

	for _, np := range t.resources.NetworkPolicies {
		hasIngress := false
		for _, pt := range np.Spec.PolicyTypes {
			if pt == networkingv1.PolicyTypeIngress {
				hasIngress = true
				break
			}
		}
		if !hasIngress {
			continue
		}

		for _, svc := range t.resources.Services {
			if svc.Namespace != np.Namespace {
				continue
			}
			if svc.Spec.Type != corev1.ServiceTypeLoadBalancer && svc.Spec.Type != corev1.ServiceTypeNodePort {
				continue
			}
			if !selectorsOverlap(np.Spec.PodSelector.MatchLabels, svc.Spec.Selector) {
				continue
			}

			etp := corev1.ServiceExternalTrafficPolicyCluster
			if svc.Spec.ExternalTrafficPolicy != "" {
				etp = svc.Spec.ExternalTrafficPolicy
			}

			if etp == corev1.ServiceExternalTrafficPolicyCluster {
				// Build port list from service
				var ports []map[string]interface{}
				for _, sp := range svc.Spec.Ports {
					portEntry := map[string]interface{}{
						"port":     fmt.Sprintf("%d", sp.Port),
						"protocol": string(sp.Protocol),
					}
					ports = append(ports, portEntry)
				}

				policyName := fmt.Sprintf("allow-lb-ingress-%s", svc.Name)
				// Truncate if too long
				if len(policyName) > 63 {
					policyName = policyName[:63]
				}

				ciliumPolicy := types.CiliumPolicy{
					Name:      policyName,
					Namespace: np.Namespace,
					Reason:    fmt.Sprintf("Allows ingress from world entity to pods behind %s service %q. Under NPM, LB traffic bypassed ingress policies. Under Cilium, it is enforced.", svc.Spec.Type, svc.Name),
					Spec: map[string]interface{}{
						"endpointSelector": map[string]interface{}{
							"matchLabels": svc.Spec.Selector,
						},
						"ingress": []map[string]interface{}{
							{
								"fromEntities": []string{"world"},
								"toPorts": []map[string]interface{}{
									{"ports": ports},
								},
							},
						},
					},
				}
				policies = append(policies, ciliumPolicy)
			}
		}
	}

	return policies
}

// buildPortNameMap builds a map of named port -> numeric port by inspecting pods matching
// the policy's selector in the same namespace.
func (t *Translator) buildPortNameMap(np *networkingv1.NetworkPolicy) map[string]int32 {
	portMap := make(map[string]int32)
	for _, pod := range t.resources.Pods {
		if pod.Namespace != np.Namespace {
			continue
		}
		if !matchesSelector(np.Spec.PodSelector.MatchLabels, pod.Labels) {
			continue
		}
		for _, c := range pod.Spec.Containers {
			for _, cp := range c.Ports {
				if cp.Name != "" {
					portMap[cp.Name] = cp.ContainerPort
				}
			}
		}
	}
	return portMap
}

func portPtr(p int32) *intstr.IntOrString {
	v := intstr.FromInt32(p)
	return &v
}

// RenderCiliumPolicyYAML renders a CiliumPolicy as YAML string.
func RenderCiliumPolicyYAML(cp types.CiliumPolicy) string {
	var sb strings.Builder
	sb.WriteString("apiVersion: \"cilium.io/v2\"\n")
	sb.WriteString("kind: CiliumNetworkPolicy\n")
	sb.WriteString("metadata:\n")
	sb.WriteString(fmt.Sprintf("  name: %s\n", cp.Name))
	sb.WriteString(fmt.Sprintf("  namespace: %s\n", cp.Namespace))
	sb.WriteString(fmt.Sprintf("  # Reason: %s\n", cp.Reason))
	sb.WriteString("spec:\n")
	renderMapYAML(&sb, cp.Spec, 2)
	return sb.String()
}

// renderMapYAML recursively renders a map as YAML.
func renderMapYAML(sb *strings.Builder, m map[string]interface{}, indent int) {
	prefix := strings.Repeat(" ", indent)
	for k, v := range m {
		switch val := v.(type) {
		case string:
			sb.WriteString(fmt.Sprintf("%s%s: %s\n", prefix, k, val))
		case int, int32, int64, float64:
			sb.WriteString(fmt.Sprintf("%s%s: %v\n", prefix, k, val))
		case map[string]interface{}:
			if len(val) == 0 {
				sb.WriteString(fmt.Sprintf("%s%s: {}\n", prefix, k))
			} else {
				sb.WriteString(fmt.Sprintf("%s%s:\n", prefix, k))
				renderMapYAML(sb, val, indent+2)
			}
		case map[string]string:
			if len(val) == 0 {
				sb.WriteString(fmt.Sprintf("%s%s: {}\n", prefix, k))
			} else {
				sb.WriteString(fmt.Sprintf("%s%s:\n", prefix, k))
				for mk, mv := range val {
					sb.WriteString(fmt.Sprintf("%s  %s: %s\n", prefix, mk, mv))
				}
			}
		case []string:
			sb.WriteString(fmt.Sprintf("%s%s:\n", prefix, k))
			for _, s := range val {
				sb.WriteString(fmt.Sprintf("%s  - %s\n", prefix, s))
			}
		case []map[string]interface{}:
			sb.WriteString(fmt.Sprintf("%s%s:\n", prefix, k))
			for _, item := range val {
				first := true
				for ik, iv := range item {
					if first {
						sb.WriteString(fmt.Sprintf("%s  - %s:", prefix, ik))
						first = false
					} else {
						sb.WriteString(fmt.Sprintf("%s    %s:", prefix, ik))
					}
					switch ivv := iv.(type) {
					case string:
						sb.WriteString(fmt.Sprintf(" %s\n", ivv))
					case []string:
						sb.WriteString("\n")
						for _, s := range ivv {
							sb.WriteString(fmt.Sprintf("%s      - %s\n", prefix, s))
						}
					case []map[string]interface{}:
						sb.WriteString("\n")
						for _, im := range ivv {
							sb.WriteString(fmt.Sprintf("%s      - ", prefix))
							firstInner := true
							for iik, iiv := range im {
								if firstInner {
									sb.WriteString(fmt.Sprintf("%s: %v\n", iik, iiv))
									firstInner = false
								} else {
									sb.WriteString(fmt.Sprintf("%s        %s: %v\n", prefix, iik, iiv))
								}
							}
						}
					default:
						sb.WriteString(fmt.Sprintf(" %v\n", ivv))
					}
				}
			}
		default:
			sb.WriteString(fmt.Sprintf("%s%s: %v\n", prefix, k, v))
		}
	}
}
