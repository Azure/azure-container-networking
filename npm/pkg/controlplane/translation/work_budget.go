package translation

import (
	"fmt"

	networkingv1 "k8s.io/api/networking/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
)

// Count copied ACL matches and generated members together before materialization.
const maxTotalPolicyMatches = maxTotalSelectorMatches

type policyWorkBudget struct {
	acls    int
	matches int
}

func validateFullPolicyWork(policy *networkingv1.NetworkPolicy) error {
	selectedMatches, selectedMembers, err := podSelectorWork(&policy.Spec.PodSelector)
	if err != nil {
		return err
	}
	if selectedMatches >= maxSelectorMatches {
		return fmt.Errorf("selected pod selector expands into %d matches including its namespace anchor, past the %d limit: %w",
			selectedMatches+1, maxSelectorMatches, ErrTooManySelectorMatches)
	}
	budget := policyWorkBudget{matches: selectedMatches + selectedMembers + 1}
	if budget.matches > maxTotalPolicyMatches {
		return fmt.Errorf("selected pod members exceed the %d policy match limit: %w", maxTotalPolicyMatches, ErrTooManyPolicyMatches)
	}
	for _, direction := range policy.Spec.PolicyTypes {
		if direction == networkingv1.PolicyTypeIngress {
			if isAllowAllToIngress(policy.Spec.Ingress) {
				if err := budget.reserve(1, 0, nil, 0); err != nil {
					return err
				}
				continue
			}
			for _, rule := range policy.Spec.Ingress {
				if err := budget.rule(rule.Ports, rule.From); err != nil {
					return err
				}
			}
		} else {
			if isAllowAllToEgress(policy.Spec.Egress) {
				if err := budget.reserve(1, 0, nil, 0); err != nil {
					return err
				}
				continue
			}
			for _, rule := range policy.Spec.Egress {
				if err := budget.rule(rule.Ports, rule.To); err != nil {
					return err
				}
			}
		}
		if err := budget.reserve(1, 0, nil, 0); err != nil {
			return err
		}
	}
	return nil
}

func (budget *policyWorkBudget) rule(ports []networkingv1.NetworkPolicyPort, peers []networkingv1.NetworkPolicyPeer) error {
	allowExternal, portRuleExists, peerRuleExists := ruleExists(ports, peers)
	if portRuleExists && (!peerRuleExists || allowExternal) {
		if err := budget.reserve(1, 0, ports, 0); err != nil {
			return err
		}
	}
	for _, peer := range peers {
		if peer.IPBlock != nil {
			if peer.IPBlock.CIDR != "" {
				// A parent may split into two members; exclusions add at most one each.
				if err := budget.reserve(1, 1, ports, len(peer.IPBlock.Except)+2); err != nil {
					return err
				}
			}
			continue
		}
		if peer.PodSelector == nil && peer.NamespaceSelector == nil {
			continue
		}
		podMatches, podMembers, err := podSelectorWork(peer.PodSelector)
		if err != nil {
			return err
		}
		branches, namespaceMatches := 1, 1
		if peer.NamespaceSelector != nil {
			branches, namespaceMatches, err = namespaceSelectorWork(peer.NamespaceSelector)
			if err != nil {
				return err
			}
			for _, requirement := range peer.NamespaceSelector.MatchExpressions {
				if unsupportedOpsInWindows(requirement.Operator) {
					return ErrUnsupportedNegativeMatch
				}
			}
		}
		if err := budget.reserve(branches, namespaceMatches+podMatches, ports, podMembers); err != nil {
			return err
		}
	}
	return nil
}

func (budget *policyWorkBudget) reserve(branches, matches int, ports []networkingv1.NetworkPolicyPort, members int) error {
	portCount := len(ports)
	if portCount == 0 {
		portCount = 1
	}
	if portCount > (maxACLsPerPolicy-budget.acls)/branches {
		return fmt.Errorf("policy exceeds the %d rule limit: %w", maxACLsPerPolicy, ErrTooManyACLs)
	}
	acls := branches * portCount
	namedPorts := 0
	for _, port := range ports {
		if port.Port != nil && port.Port.Type == intstr.String {
			namedPorts++
		}
	}
	perACLMatches := matches
	if namedPorts > 0 {
		perACLMatches++
	}
	if perACLMatches > maxSelectorMatches {
		return fmt.Errorf("peer expands into %d matches per ACL, past the %d limit: %w",
			perACLMatches, maxSelectorMatches, ErrTooManySelectorMatches)
	}
	remaining := maxTotalPolicyMatches - budget.matches
	if members > remaining || matches > (remaining-members)/acls {
		return fmt.Errorf("replicated matches exceed the %d policy match limit: %w", maxTotalPolicyMatches, ErrTooManyPolicyMatches)
	}
	work := members + matches*acls
	if namedPorts > (remaining-work)/branches {
		return fmt.Errorf("named-port matches exceed the %d policy match limit: %w", maxTotalPolicyMatches, ErrTooManyPolicyMatches)
	}
	budget.acls += acls
	budget.matches += work + namedPorts*branches
	return nil
}

func podSelectorWork(selector *metav1.LabelSelector) (matches, members int, err error) {
	if selector == nil {
		return 0, 0, nil
	}
	matches = len(selector.MatchLabels) + len(selector.MatchExpressions)
	if matches > maxSelectorMatches {
		return 0, 0, fmt.Errorf("pod selector has %d matches, past the %d limit: %w",
			matches, maxSelectorMatches, ErrTooManySelectorMatches)
	}
	for _, requirement := range selector.MatchExpressions {
		if unsupportedOpsInWindows(requirement.Operator) {
			return 0, 0, ErrUnsupportedNegativeMatch
		}
		if len(requirement.Values) > maxSelectorMatches {
			return 0, 0, fmt.Errorf("pod requirement %q has %d values, past the %d limit: %w",
				requirement.Key, len(requirement.Values), maxSelectorMatches, ErrTooManySelectorMatches)
		}
		if len(requirement.Values) > 1 {
			if len(requirement.Values) > maxTotalPolicyMatches-members {
				return 0, 0, fmt.Errorf("pod selector members exceed the %d policy match limit: %w", maxTotalPolicyMatches, ErrTooManyPolicyMatches)
			}
			members += len(requirement.Values)
		}
	}
	return matches, members, nil
}
