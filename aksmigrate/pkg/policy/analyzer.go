package policy

import (
	"fmt"
	"net"
	"strings"
	"time"

	corev1 "k8s.io/api/core/v1"
	networkingv1 "k8s.io/api/networking/v1"
	"k8s.io/apimachinery/pkg/util/intstr"

	"github.com/azure/aksmigrate/pkg/types"
)

// Analyzer scans NetworkPolicies and cluster resources for Cilium migration incompatibilities.
type Analyzer struct {
	resources      *types.ClusterResources
	k8sVersion     string // e.g. "1.29" to determine Cilium version
	ciliumVersion  string // e.g. "1.14.19"
}

// NewAnalyzer creates a new policy analyzer.
func NewAnalyzer(resources *types.ClusterResources, k8sVersion string) *Analyzer {
	return &Analyzer{
		resources:     resources,
		k8sVersion:    k8sVersion,
		ciliumVersion: ciliumVersionForK8s(k8sVersion),
	}
}

// ciliumVersionForK8s maps Kubernetes versions to the minimum Cilium version on AKS.
func ciliumVersionForK8s(k8sVersion string) string {
	switch {
	case strings.HasPrefix(k8sVersion, "1.33"):
		return "1.17.0"
	case strings.HasPrefix(k8sVersion, "1.32"):
		return "1.17.0"
	case strings.HasPrefix(k8sVersion, "1.31"):
		return "1.16.6"
	case strings.HasPrefix(k8sVersion, "1.30"):
		return "1.14.19"
	case strings.HasPrefix(k8sVersion, "1.29"):
		return "1.14.19"
	default:
		return "1.14.19"
	}
}

// Analyze runs all detection rules and returns an AuditReport.
func (a *Analyzer) Analyze() *types.AuditReport {
	report := &types.AuditReport{
		Timestamp:     time.Now().UTC().Format(time.RFC3339),
		TotalPolicies: len(a.resources.NetworkPolicies),
	}

	for i := range a.resources.NetworkPolicies {
		np := &a.resources.NetworkPolicies[i]
		a.checkIPBlockCatchAll(np, report)
		a.checkNamedPorts(np, report)
		a.checkEndPort(np, report)
		a.checkImplicitLocalNodeEgress(np, report)
		a.checkLBIngressEnforcement(np, report)
	}

	// Cluster-wide checks (not per-policy)
	a.checkHostNetworkPods(report)
	a.checkKubeProxyRemoval(report)
	a.checkIdentityExhaustion(report)
	a.checkServiceMesh(report)

	// Compute summary
	for _, f := range report.Findings {
		switch f.Severity {
		case types.SeverityFail:
			report.Summary.FailCount++
		case types.SeverityWarn:
			report.Summary.WarnCount++
		case types.SeverityPass:
			report.Summary.PassCount++
		case types.SeverityInfo:
			report.Summary.InfoCount++
		}
	}

	return report
}

// checkIPBlockCatchAll detects ipBlock rules with broad CIDRs (e.g. 0.0.0.0/0)
// that will behave differently under Cilium's identity-based enforcement.
func (a *Analyzer) checkIPBlockCatchAll(np *networkingv1.NetworkPolicy, report *types.AuditReport) {
	// Check egress rules
	for i, egressRule := range np.Spec.Egress {
		for j, peer := range egressRule.To {
			if peer.IPBlock == nil {
				continue
			}
			if isBroadCIDR(peer.IPBlock.CIDR) {
				// Check if there are also selector-based peers that would cover pods
				hasSelectors := hasSelectorPeers(egressRule.To)
				if !hasSelectors {
					report.Findings = append(report.Findings, types.Finding{
						RuleID:      types.RuleIPBlockCatchAll,
						Severity:    types.SeverityFail,
						PolicyName:  np.Name,
						Namespace:   np.Namespace,
						Description: fmt.Sprintf("Egress rule[%d].to[%d] uses ipBlock CIDR %q without pod/namespace selectors. Under Cilium, this will NOT match pod or node IPs — only external IPs. Pod-to-pod and pod-to-node egress will be blocked.", i, j, peer.IPBlock.CIDR),
						Remediation: "Add namespaceSelector: {} and podSelector: {} peers alongside the ipBlock. Create a CiliumNetworkPolicy with toEntities: [host, remote-node] for node access.",
						AffectedFields: []string{
							fmt.Sprintf("spec.egress[%d].to[%d].ipBlock.cidr", i, j),
						},
					})
				}
			}
		}
	}

	// Check ingress rules
	for i, ingressRule := range np.Spec.Ingress {
		for j, peer := range ingressRule.From {
			if peer.IPBlock == nil {
				continue
			}
			if isBroadCIDR(peer.IPBlock.CIDR) {
				hasSelectors := hasSelectorPeers(convertFromPeersToToPeers(ingressRule.From))
				if !hasSelectors {
					report.Findings = append(report.Findings, types.Finding{
						RuleID:      types.RuleIPBlockCatchAll,
						Severity:    types.SeverityFail,
						PolicyName:  np.Name,
						Namespace:   np.Namespace,
						Description: fmt.Sprintf("Ingress rule[%d].from[%d] uses ipBlock CIDR %q without pod/namespace selectors. Under Cilium, this will NOT match pod or node source IPs.", i, j, peer.IPBlock.CIDR),
						Remediation: "Add namespaceSelector: {} and podSelector: {} peers alongside the ipBlock to allow traffic from cluster-internal sources.",
						AffectedFields: []string{
							fmt.Sprintf("spec.ingress[%d].from[%d].ipBlock.cidr", i, j),
						},
					})
				}
			}
		}
	}
}

// checkNamedPorts detects usage of named ports that may not work correctly under Cilium.
func (a *Analyzer) checkNamedPorts(np *networkingv1.NetworkPolicy, report *types.AuditReport) {
	allPorts := collectAllPorts(np)
	for _, portInfo := range allPorts {
		if portInfo.Port != nil && portInfo.Port.Type == intstr.String {
			// Check if this named port maps to different numeric values across pods
			portName := portInfo.Port.StrVal
			conflicting := a.findConflictingPortMappings(np, portName)

			severity := types.SeverityWarn
			desc := fmt.Sprintf("Policy uses named port %q. Named ports may not enforce correctly under Cilium when the same port name maps to different port numbers across pods (Cilium issue #30003).", portName)

			if conflicting {
				severity = types.SeverityFail
				desc = fmt.Sprintf("Policy uses named port %q which maps to DIFFERENT numeric values across target pods. This WILL cause incorrect policy enforcement under Cilium.", portName)
			}

			report.Findings = append(report.Findings, types.Finding{
				RuleID:      types.RuleNamedPorts,
				Severity:    severity,
				PolicyName:  np.Name,
				Namespace:   np.Namespace,
				Description: desc,
				Remediation: fmt.Sprintf("Replace named port %q with its numeric value before migration.", portName),
				AffectedFields: []string{portInfo.Location},
			})
		}
	}
}

// checkEndPort detects usage of endPort which requires Cilium >= 1.17.
func (a *Analyzer) checkEndPort(np *networkingv1.NetworkPolicy, report *types.AuditReport) {
	for i, egressRule := range np.Spec.Egress {
		for j, port := range egressRule.Ports {
			if port.EndPort != nil {
				if !supportsEndPort(a.ciliumVersion) {
					report.Findings = append(report.Findings, types.Finding{
						RuleID:      types.RuleEndPort,
						Severity:    types.SeverityFail,
						PolicyName:  np.Name,
						Namespace:   np.Namespace,
						Description: fmt.Sprintf("Egress rule[%d].ports[%d] uses endPort=%d, but your cluster's Cilium version (%s) does not support endPort (requires >= 1.17).", i, j, *port.EndPort, a.ciliumVersion),
						Remediation: "Either upgrade to Kubernetes 1.32+ (which ships with Cilium 1.17) or expand the port range into individual port entries.",
						AffectedFields: []string{
							fmt.Sprintf("spec.egress[%d].ports[%d].endPort", i, j),
						},
					})
				}
			}
		}
	}
	for i, ingressRule := range np.Spec.Ingress {
		for j, port := range ingressRule.Ports {
			if port.EndPort != nil {
				if !supportsEndPort(a.ciliumVersion) {
					report.Findings = append(report.Findings, types.Finding{
						RuleID:      types.RuleEndPort,
						Severity:    types.SeverityFail,
						PolicyName:  np.Name,
						Namespace:   np.Namespace,
						Description: fmt.Sprintf("Ingress rule[%d].ports[%d] uses endPort=%d, but your cluster's Cilium version (%s) does not support endPort (requires >= 1.17).", i, j, *port.EndPort, a.ciliumVersion),
						Remediation: "Either upgrade to Kubernetes 1.32+ (which ships with Cilium 1.17) or expand the port range into individual port entries.",
						AffectedFields: []string{
							fmt.Sprintf("spec.ingress[%d].ports[%d].endPort", i, j),
						},
					})
				}
			}
		}
	}
}

// checkImplicitLocalNodeEgress detects egress-restricted pods that rely on
// NPM's implicit allow for local node traffic.
func (a *Analyzer) checkImplicitLocalNodeEgress(np *networkingv1.NetworkPolicy, report *types.AuditReport) {
	hasEgress := false
	for _, pt := range np.Spec.PolicyTypes {
		if pt == networkingv1.PolicyTypeEgress {
			hasEgress = true
			break
		}
	}
	if !hasEgress {
		return
	}

	// If there are egress rules, check if any explicitly allow host/node traffic
	// Since K8s NetworkPolicy can't express "allow to host entity", any egress-restricted
	// pod is at risk of losing implicit local node access.
	report.Findings = append(report.Findings, types.Finding{
		RuleID:      types.RuleImplicitLocalNodeEgress,
		Severity:    types.SeverityWarn,
		PolicyName:  np.Name,
		Namespace:   np.Namespace,
		Description: "Policy restricts egress traffic. Under NPM, egress to the local node IP is implicitly allowed. Under Cilium, this is blocked unless explicitly allowed via CiliumNetworkPolicy.",
		Remediation: "After migration, create a CiliumNetworkPolicy with toEntities: [host] to restore implicit local node egress access for affected pods.",
	})
}

// checkLBIngressEnforcement detects ingress policies on pods that back
// LoadBalancer or NodePort services. Under Cilium, these policies WILL be enforced
// on LB traffic, unlike NPM.
func (a *Analyzer) checkLBIngressEnforcement(np *networkingv1.NetworkPolicy, report *types.AuditReport) {
	hasIngress := false
	for _, pt := range np.Spec.PolicyTypes {
		if pt == networkingv1.PolicyTypeIngress {
			hasIngress = true
			break
		}
	}
	if !hasIngress {
		return
	}

	// Check if this policy selects pods that back a LoadBalancer or NodePort service
	for _, svc := range a.resources.Services {
		if svc.Namespace != np.Namespace {
			continue
		}
		if svc.Spec.Type != corev1.ServiceTypeLoadBalancer && svc.Spec.Type != corev1.ServiceTypeNodePort {
			continue
		}

		// Check if the NetworkPolicy's podSelector matches the service's selector
		if selectorsOverlap(np.Spec.PodSelector.MatchLabels, svc.Spec.Selector) {
			etp := corev1.ServiceExternalTrafficPolicyCluster
			if svc.Spec.ExternalTrafficPolicy != "" {
				etp = svc.Spec.ExternalTrafficPolicy
			}

			if etp == corev1.ServiceExternalTrafficPolicyCluster {
				report.Findings = append(report.Findings, types.Finding{
					RuleID:   types.RuleLBIngressEnforcement,
					Severity: types.SeverityFail,
					PolicyName: np.Name,
					Namespace:  np.Namespace,
					Description: fmt.Sprintf("Policy applies ingress rules to pods backing %s service %q (externalTrafficPolicy=Cluster). Under NPM, LoadBalancer/NodePort traffic bypasses ingress policy. Under Cilium, it IS enforced. A deny-all ingress will block LB traffic.", svc.Spec.Type, svc.Name),
					Remediation: fmt.Sprintf("Add explicit ingress allow rules for traffic arriving via service %q, or switch to externalTrafficPolicy=Local.", svc.Name),
				})
			}
		}
	}
}

// checkHostNetworkPods detects pods using hostNetwork that have NetworkPolicies targeting them.
func (a *Analyzer) checkHostNetworkPods(report *types.AuditReport) {
	hostNetPods := make(map[string][]string) // namespace -> []podName
	for _, pod := range a.resources.Pods {
		if pod.Spec.HostNetwork {
			hostNetPods[pod.Namespace] = append(hostNetPods[pod.Namespace], pod.Name)
		}
	}

	if len(hostNetPods) == 0 {
		return
	}

	for _, np := range a.resources.NetworkPolicies {
		pods, ok := hostNetPods[np.Namespace]
		if !ok {
			continue
		}

		// Check if the policy could match any host-networked pods
		for _, podName := range pods {
			pod := findPod(a.resources.Pods, np.Namespace, podName)
			if pod == nil {
				continue
			}
			if matchesSelector(np.Spec.PodSelector.MatchLabels, pod.Labels) {
				report.Findings = append(report.Findings, types.Finding{
					RuleID:      types.RuleHostNetworkPods,
					Severity:    types.SeverityWarn,
					PolicyName:  np.Name,
					Namespace:   np.Namespace,
					Description: fmt.Sprintf("Policy selects host-networked pod %q. Under Cilium, network policies are NOT enforced on pods with hostNetwork=true.", podName),
					Remediation: "Consider using node-level firewall rules or NSGs instead of NetworkPolicy for host-networked pods.",
				})
			}
		}
	}
}

// checkKubeProxyRemoval adds an info finding about kube-proxy removal.
func (a *Analyzer) checkKubeProxyRemoval(report *types.AuditReport) {
	report.Findings = append(report.Findings, types.Finding{
		RuleID:      types.RuleKubeProxyRemoval,
		Severity:    types.SeverityInfo,
		PolicyName:  "(cluster-wide)",
		Namespace:   "",
		Description: "Migration to Cilium will remove kube-proxy. Cilium takes over all service routing via eBPF. Any tooling, monitoring, or custom scripts that depend on kube-proxy iptables rules will break.",
		Remediation: "Audit for any dependencies on kube-proxy iptables chains (e.g., iptables -t nat -L parsing, kube-proxy metrics). Update monitoring dashboards to use Cilium/Hubble metrics.",
	})
}

// checkIdentityExhaustion detects high-churn labels that could exhaust Cilium's 65K identity limit.
func (a *Analyzer) checkIdentityExhaustion(report *types.AuditReport) {
	highChurnLabels := []string{
		"spark-app-name", "spark-app-selector", "spark-role",
		"job-name", "controller-uid", "batch.kubernetes.io/job-name",
		"statefulset.kubernetes.io/pod-name",
	}

	labelCounts := make(map[string]int)
	for _, pod := range a.resources.Pods {
		for label := range pod.Labels {
			for _, hcl := range highChurnLabels {
				if label == hcl {
					labelCounts[label]++
				}
			}
		}
	}

	// Count unique label value combinations
	uniqueIdentities := countUniqueIdentities(a.resources.Pods)

	for label, count := range labelCounts {
		if count > 50 {
			report.Findings = append(report.Findings, types.Finding{
				RuleID:      types.RuleIdentityExhaustion,
				Severity:    types.SeverityWarn,
				PolicyName:  "(cluster-wide)",
				Namespace:   "",
				Description: fmt.Sprintf("High-churn label %q found on %d pods. Cilium assigns a unique identity per label-set combination (limit: 65,535). High-churn labels can cause identity exhaustion.", label, count),
				Remediation: fmt.Sprintf("Exclude label %q from Cilium identity computation via the Cilium configmap (requires Azure support for managed clusters).", label),
			})
		}
	}

	if uniqueIdentities > 50000 {
		report.Findings = append(report.Findings, types.Finding{
			RuleID:      types.RuleIdentityExhaustion,
			Severity:    types.SeverityFail,
			PolicyName:  "(cluster-wide)",
			Namespace:   "",
			Description: fmt.Sprintf("Estimated unique identity count (%d) is dangerously close to Cilium's 65,535 limit. Identity exhaustion will prevent new pods from being scheduled correctly.", uniqueIdentities),
			Remediation: "Review pod labels for high-cardinality values and exclude them from identity computation. Contact Azure support for Cilium configmap modifications.",
		})
	}
}

// checkServiceMesh detects Istio or Linkerd sidecars.
func (a *Analyzer) checkServiceMesh(report *types.AuditReport) {
	istioCount := 0
	linkerdCount := 0

	for _, pod := range a.resources.Pods {
		for _, c := range pod.Spec.Containers {
			if c.Name == "istio-proxy" || strings.Contains(c.Image, "istio") {
				istioCount++
			}
			if c.Name == "linkerd-proxy" || strings.Contains(c.Image, "linkerd") {
				linkerdCount++
			}
		}
		for _, ic := range pod.Spec.InitContainers {
			if ic.Name == "istio-init" || strings.Contains(ic.Image, "istio") {
				istioCount++
			}
			if ic.Name == "linkerd-init" || strings.Contains(ic.Image, "linkerd") {
				linkerdCount++
			}
		}
	}

	if istioCount > 0 {
		report.Findings = append(report.Findings, types.Finding{
			RuleID:      types.RuleServiceMeshDetected,
			Severity:    types.SeverityWarn,
			PolicyName:  "(cluster-wide)",
			Namespace:   "",
			Description: fmt.Sprintf("Istio service mesh detected (%d sidecar instances). Cilium removes kube-proxy which Istio depends on for ClusterIP resolution. Requires Istio >= 1.16 for compatibility.", istioCount),
			Remediation: "Test Istio connectivity in a staging cluster before migration. Verify mTLS, traffic routing, and authorization policies work with Cilium's eBPF data plane.",
		})
	}

	if linkerdCount > 0 {
		report.Findings = append(report.Findings, types.Finding{
			RuleID:      types.RuleServiceMeshDetected,
			Severity:    types.SeverityWarn,
			PolicyName:  "(cluster-wide)",
			Namespace:   "",
			Description: fmt.Sprintf("Linkerd service mesh detected (%d proxy instances). Cilium removes kube-proxy. Linkerd is generally compatible but should be validated.", linkerdCount),
			Remediation: "Test Linkerd connectivity in a staging cluster before migration.",
		})
	}
}

// --- Helper functions ---

// isBroadCIDR returns true if the CIDR covers a very large range (e.g., 0.0.0.0/0 or 10.0.0.0/8).
func isBroadCIDR(cidr string) bool {
	_, ipNet, err := net.ParseCIDR(cidr)
	if err != nil {
		return false
	}
	ones, bits := ipNet.Mask.Size()
	// Consider anything /16 or broader as "broad" for ipBlock warning purposes
	return ones <= 16 && bits == 32
}

// hasSelectorPeers returns true if the peer list contains at least one selector-based peer.
func hasSelectorPeers(peers []networkingv1.NetworkPolicyPeer) bool {
	for _, p := range peers {
		if p.PodSelector != nil || p.NamespaceSelector != nil {
			return true
		}
	}
	return false
}

// convertFromPeersToToPeers converts ingress "from" peers to the same type as egress "to" peers
// (they're the same type, this is just for type compatibility).
func convertFromPeersToToPeers(from []networkingv1.NetworkPolicyPeer) []networkingv1.NetworkPolicyPeer {
	return from
}

type portInfo struct {
	Port     *intstr.IntOrString
	Location string
}

// collectAllPorts returns all ports from all rules in a NetworkPolicy with their locations.
func collectAllPorts(np *networkingv1.NetworkPolicy) []portInfo {
	var ports []portInfo
	for i, rule := range np.Spec.Ingress {
		for j, p := range rule.Ports {
			if p.Port != nil {
				ports = append(ports, portInfo{
					Port:     p.Port,
					Location: fmt.Sprintf("spec.ingress[%d].ports[%d].port", i, j),
				})
			}
		}
	}
	for i, rule := range np.Spec.Egress {
		for j, p := range rule.Ports {
			if p.Port != nil {
				ports = append(ports, portInfo{
					Port:     p.Port,
					Location: fmt.Sprintf("spec.egress[%d].ports[%d].port", i, j),
				})
			}
		}
	}
	return ports
}

// findConflictingPortMappings checks if a named port resolves to different numeric values
// across pods that the policy selects.
func (a *Analyzer) findConflictingPortMappings(np *networkingv1.NetworkPolicy, portName string) bool {
	var portNumbers []int32
	for _, pod := range a.resources.Pods {
		if pod.Namespace != np.Namespace {
			continue
		}
		if !matchesSelector(np.Spec.PodSelector.MatchLabels, pod.Labels) {
			continue
		}
		for _, c := range pod.Spec.Containers {
			for _, cp := range c.Ports {
				if cp.Name == portName {
					portNumbers = append(portNumbers, cp.ContainerPort)
				}
			}
		}
	}

	if len(portNumbers) <= 1 {
		return false
	}
	first := portNumbers[0]
	for _, pn := range portNumbers[1:] {
		if pn != first {
			return true
		}
	}
	return false
}

// supportsEndPort returns true if the Cilium version supports the endPort field.
func supportsEndPort(ciliumVersion string) bool {
	parts := strings.Split(ciliumVersion, ".")
	if len(parts) < 2 {
		return false
	}
	// endPort requires Cilium >= 1.17
	major := parts[0]
	minor := parts[1]
	if major == "1" {
		switch {
		case minor >= "17":
			return true
		default:
			return false
		}
	}
	return false
}

// selectorsOverlap checks if the policy's match labels are a subset of the service's selector.
func selectorsOverlap(policyLabels, serviceSelector map[string]string) bool {
	if len(policyLabels) == 0 {
		// Empty selector matches all pods
		return true
	}
	for k, v := range policyLabels {
		if sv, ok := serviceSelector[k]; !ok || sv != v {
			return false
		}
	}
	return true
}

// matchesSelector checks if pod labels satisfy a label selector.
func matchesSelector(selectorLabels, podLabels map[string]string) bool {
	if len(selectorLabels) == 0 {
		return true // empty selector matches all
	}
	for k, v := range selectorLabels {
		if pv, ok := podLabels[k]; !ok || pv != v {
			return false
		}
	}
	return true
}

// findPod finds a pod by namespace and name.
func findPod(pods []corev1.Pod, namespace, name string) *corev1.Pod {
	for i := range pods {
		if pods[i].Namespace == namespace && pods[i].Name == name {
			return &pods[i]
		}
	}
	return nil
}

// countUniqueIdentities estimates the number of unique Cilium identities based on
// unique label set combinations across all pods.
func countUniqueIdentities(pods []corev1.Pod) int {
	seen := make(map[string]bool)
	for _, pod := range pods {
		key := labelSetKey(pod.Labels)
		seen[key] = true
	}
	return len(seen)
}

// labelSetKey creates a deterministic string key from a label map.
func labelSetKey(labels map[string]string) string {
	if len(labels) == 0 {
		return ""
	}
	pairs := make([]string, 0, len(labels))
	for k, v := range labels {
		pairs = append(pairs, k+"="+v)
	}
	// Simple sort for determinism
	for i := 0; i < len(pairs); i++ {
		for j := i + 1; j < len(pairs); j++ {
			if pairs[i] > pairs[j] {
				pairs[i], pairs[j] = pairs[j], pairs[i]
			}
		}
	}
	return strings.Join(pairs, ",")
}
