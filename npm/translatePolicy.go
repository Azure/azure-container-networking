// Copyright 2018 Microsoft. All rights reserved.
// MIT License
package npm

import (
	"github.com/Azure/azure-container-networking/log"
	"github.com/Azure/azure-container-networking/npm/iptm"
	"github.com/Azure/azure-container-networking/npm/util"
	networkingv1 "k8s.io/api/networking/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

type portsInfo struct {
	protocol string
	port     string
}

func craftPartialIptEntrySpecFromPort(portRule networkingv1.NetworkPolicyPort, sPortOrDPortFlag string) []string {
	partialSpec := []string{}
	if portRule.Protocol != nil {
		partialSpec = append(
			partialSpec,
			util.IptablesProtFlag,
			string(*portRule.Protocol),
		)
	}

	if portRule.Port != nil {
		partialSpec = append(
			partialSpec,
			sPortOrDPortFlag,
			portRule.Port.String(),
		)
	}

	return partialSpec
}

func craftPartialIptablesCommentFromPort(portRule networkingv1.NetworkPolicyPort, sPortOrDPortFlag string) string {
	partialComment := ""
	if portRule.Protocol != nil {
		partialComment += string(*portRule.Protocol)
		if portRule.Port != nil {
			partialComment += "-"
		}
	}

	if portRule.Port != nil {
		partialComment += "PORT-"
		partialComment += portRule.Port.String()		
	}

	if portRule.Protocol != nil || portRule.Port != nil {
		partialComment += "-OF-"
	}

	return partialComment
}

func craftPartialIptEntrySpecFromOpAndLabel(op, label, srcOrDstFlag string, isNamespaceSelector bool) []string {
	if isNamespaceSelector {
		label = "ns-" + label
	}
	partialSpec := []string{
		util.IptablesModuleFlag,
		util.IptablesSetModuleFlag,
		op,
		util.IptablesMatchSetFlag,
		util.GetHashedName(label),
		srcOrDstFlag,
	}

	return util.DropEmptyFields(partialSpec)
}

func craftPartialIptEntrySpecFromOpsAndLabels(ns string, ops, labels []string, srcOrDstFlag string, isNamespaceSelector bool) []string {
	var spec []string
	
	if len(ops) == 1 && len(labels) == 1 {
		if ops[0] == "" && labels[0] == "" {
			if !isNamespaceSelector {
				// This is an empty podSelector,
				// selecting all the pods within its namespace.
				spec = []string{
					util.IptablesModuleFlag,
					util.IptablesSetModuleFlag,
					util.IptablesMatchSetFlag,
					util.GetHashedName("ns-" + ns),
					srcOrDstFlag,
				}
			} else {
				// This is an empty namespaceSelector,
				// selecting all namespaces.
				spec = []string{
					util.IptablesModuleFlag,
					util.IptablesSetModuleFlag,
					util.IptablesMatchSetFlag,
					util.GetHashedName(util.KubeAllNamespacesFlag),
					srcOrDstFlag,
				}
			}

			return spec
		}
	}

	for i, _ := range ops {
		spec = append(spec, craftPartialIptEntrySpecFromOpAndLabel(ops[i], labels[i], srcOrDstFlag, isNamespaceSelector)...)
	}

	return spec
}

func craftPartialIptEntrySpecFromSelector(ns string, selector *metav1.LabelSelector, srcOrDstFlag string, isNamespaceSelector bool) []string {
	labelsWithOps, _, _ := parseSelector(selector)
	ops, labels := GetOperatorsAndLabels(labelsWithOps)
	return craftPartialIptEntrySpecFromOpsAndLabels(ns, ops, labels, srcOrDstFlag, isNamespaceSelector)
}

func craftPartialIptablesCommentFromSelector(ns string, selector *metav1.LabelSelector, isNamespaceSelector bool) string {
	if selector == nil {
		return "none"
	}

	if len(selector.MatchExpressions) == 0 && len(selector.MatchLabels) == 0 {
		if isNamespaceSelector {
			return util.KubeAllNamespacesFlag
		}

		return "ns-" + ns
	}

	labelsWithOps, _, _ := parseSelector(selector)
	ops, labelsWithoutOps := GetOperatorsAndLabels(labelsWithOps)

	var comment, prefix string
	if isNamespaceSelector {
		prefix = "ns-"
	}

	for i, _ := range labelsWithoutOps {
		comment += prefix + ops[i] + labelsWithoutOps[i]
		comment += "-AND-"
	}
	
	return comment[:len(comment)-len("-AND-")]
}

func translateIngress(ns string, targetSelector metav1.LabelSelector, rules []networkingv1.NetworkPolicyIngressRule) ([]string, []string, []*iptm.IptEntry) {
	var (
		sets  []string // ipsets with type: net:hash
		lists []string // ipsets with type: list:set
		entries         []*iptm.IptEntry
	)

	log.Printf("started parsing ingress rule")

	labelsWithOps, _, _ := parseSelector(&targetSelector)
	ops, labels := GetOperatorsAndLabels(labelsWithOps)
	if len(ops) == 1 && len(labels) == 1 {
		if ops[0] == "" && labels[0] == "" {
			// targetSelector is empty. Select all pods within the namespace
			labels[0] = "ns-" + ns
		}
	}
	sets = append(sets, labels...)
	
	targetSelectorIptEntrySpec := craftPartialIptEntrySpecFromOpsAndLabels(ns, ops, labels, util.IptablesDstFlag, false)
	targetSelectorComment := craftPartialIptablesCommentFromSelector(ns, &targetSelector, false)
	
	for _, rule := range rules {
		allowExternal, portRuleExists, fromRuleExists := false, false, false

		if len(rule.Ports) > 0 {
			portRuleExists = true
		}

		if rule.From != nil {
			if len(rule.From) == 0 {
				fromRuleExists = true
				allowExternal = true
			}

			for _, fromRule := range rule.From {
				if fromRule.PodSelector != nil ||
					fromRule.NamespaceSelector != nil ||
					fromRule.IPBlock != nil {
					fromRuleExists = true
					break
				}
			}
		}

		if !portRuleExists && !fromRuleExists && !allowExternal {
			entry := &iptm.IptEntry{
				Chain: util.IptablesAzureIngressPortChain,
			}
			entry.Specs = append(
				entry.Specs,
				util.IptablesModuleFlag,
				util.IptablesSetModuleFlag,
				util.IptablesMatchSetFlag,
				util.GetHashedName(util.KubeAllNamespacesFlag),
				util.IptablesSrcFlag,
			)
			entry.Specs = append(entry.Specs, targetSelectorIptEntrySpec...)
			entry.Specs = append(
				entry.Specs,
				util.IptablesJumpFlag,
				util.IptablesAccept,
				util.IptablesModuleFlag,
				util.IptablesCommentModuleFlag,
				util.IptablesCommentFlag,
				"ALLOW-ALL-TO-" + targetSelectorComment + 
				"-FROM-" +util.KubeAllNamespacesFlag,
			)

			entries = append(entries, entry)
			lists = append(lists, util.KubeAllNamespacesFlag)
			continue
		}

		// Only Ports rules exist
		if portRuleExists && !fromRuleExists && !allowExternal {
			for _, portRule := range rule.Ports {
				entry := &iptm.IptEntry{
					Chain: util.IptablesAzureIngressPortChain,
					Specs: craftPartialIptEntrySpecFromPort(portRule, util.IptablesDstPortFlag),
				}
				entry.Specs = append(entry.Specs, targetSelectorIptEntrySpec...)
				entry.Specs = append(
					entry.Specs,
					util.IptablesJumpFlag,
					util.IptablesAccept,
					util.IptablesModuleFlag,
					util.IptablesCommentModuleFlag,
					util.IptablesCommentFlag,
					"ALLOW-ALL-TO-" +
					craftPartialIptablesCommentFromPort(portRule, util.IptablesDstPortFlag) +
					targetSelectorComment,
				)
				entries = append(entries, entry)
			}
			continue
		}

		// fromRuleExists
		if portRuleExists {
			for _, portRule := range rule.Ports {
				entry := &iptm.IptEntry{
					Chain: util.IptablesAzureIngressPortChain,
					Specs: craftPartialIptEntrySpecFromPort(portRule, util.IptablesDstPortFlag),
				}
				entry.Specs = append(entry.Specs, targetSelectorIptEntrySpec...)
				entry.Specs = append(
					entry.Specs,
					util.IptablesJumpFlag,
					util.IptablesAzureIngressFromChain,
					util.IptablesModuleFlag,
					util.IptablesCommentModuleFlag,
					util.IptablesCommentFlag,
					"ALLOW-ALL-TO-" +
					craftPartialIptablesCommentFromPort(portRule, util.IptablesDstPortFlag) +
					targetSelectorComment +
					"-TO-JUMP-TO-" + util.IptablesAzureIngressFromChain,
				)
				entries = append(entries, entry)
			}
		} else {
			entry := &iptm.IptEntry{
				Chain: util.IptablesAzureIngressPortChain,
			}
			entry.Specs = append(entry.Specs, targetSelectorIptEntrySpec...)
			entry.Specs = append(
				entry.Specs,
				util.IptablesJumpFlag,
				util.IptablesAzureIngressFromChain,
				util.IptablesModuleFlag,
				util.IptablesCommentModuleFlag,
				util.IptablesCommentFlag,
				"ALLOW-ALL-TO-" +
				targetSelectorComment +
				"-TO-JUMP-TO-" + util.IptablesAzureIngressFromChain,
			)
			entries = append(entries, entry)
		}

		if allowExternal {
			entry := &iptm.IptEntry{
				Chain: util.IptablesAzureIngressFromChain,
			}
			entry.Specs = append(entry.Specs, targetSelectorIptEntrySpec...)
			entry.Specs = append(
				entry.Specs,
				util.IptablesJumpFlag,
				util.IptablesAccept,
				util.IptablesModuleFlag,
				util.IptablesCommentModuleFlag,
				util.IptablesCommentFlag,
				"ALLOW-ALL-TO-" +
				targetSelectorComment,
			)
			entries = append(entries, entry)

			continue
		}

		for _, fromRule := range rule.From {
			// Handle IPBlock field of NetworkPolicyPeer
			if fromRule.IPBlock != nil {
				if len(fromRule.IPBlock.CIDR) > 0 {
					cidrEntry := &iptm.IptEntry{
						Chain: util.IptablesAzureIngressFromChain,
					}
					cidrEntry.Specs	= append(
						cidrEntry.Specs,
						util.IptablesSFlag,
						fromRule.IPBlock.CIDR,
					)
					cidrEntry.Specs	= append(cidrEntry.Specs, targetSelectorIptEntrySpec...)
					cidrEntry.Specs	= append(
						cidrEntry.Specs,
						util.IptablesJumpFlag,
						util.IptablesAccept,
						util.IptablesModuleFlag,
						util.IptablesCommentModuleFlag,
						util.IptablesCommentFlag,
						"ALLOW-" + fromRule.IPBlock.CIDR +
						"-TO-" + targetSelectorComment,
					)													
					entries = append(entries, cidrEntry)
				}
				if len(fromRule.IPBlock.Except) > 0 {
					for _, except := range fromRule.IPBlock.Except {
						exceptEntry := &iptm.IptEntry{
							Chain: util.IptablesAzureIngressFromChain,
						}
						exceptEntry.Specs	= append(
							exceptEntry.Specs,
							util.IptablesSFlag,
							except,
						)
						exceptEntry.Specs = append(exceptEntry.Specs, targetSelectorIptEntrySpec...)
						exceptEntry.Specs = append(
							exceptEntry.Specs,
							util.IptablesJumpFlag,
							util.IptablesDrop,
							util.IptablesModuleFlag,
							util.IptablesCommentModuleFlag,
							util.IptablesCommentFlag,
							"DROP-" + except +
							"-TO-" + targetSelectorComment,
						)
						entries = append(entries, exceptEntry)
					}
				}
				continue
			}

			// Handle podSelector and namespaceSelector.
			// For PodSelector, use hash:net in ipset.
			// For NamespaceSelector, use set:list in ipset.
			if fromRule.PodSelector == nil && fromRule.NamespaceSelector == nil {
				continue
			}

			if fromRule.PodSelector == nil && fromRule.NamespaceSelector != nil {
				nsLabelsWithOps, _, _ := parseSelector(fromRule.NamespaceSelector)
				_, nsLabelsWithoutOps := GetOperatorsAndLabels(nsLabelsWithOps)
				if len(nsLabelsWithoutOps) == 1 && nsLabelsWithoutOps[0] == "" {
					// Empty namespaceSelector. This selects all namespaces
					nsLabelsWithoutOps[0] = util.KubeAllNamespacesFlag
				} else {
					for i, _ := range nsLabelsWithoutOps {
						// Add namespaces prefix to distinguish namespace ipset lists and pod ipsets
						nsLabelsWithoutOps[i] = "ns-" + nsLabelsWithoutOps[i]
					}
				}
				lists = append(lists, nsLabelsWithoutOps...)

				entry := &iptm.IptEntry{
					Chain: util.IptablesAzureIngressFromChain,
				}
				entry.Specs = append(
					entry.Specs, 
					craftPartialIptEntrySpecFromSelector(
						ns,
						fromRule.NamespaceSelector, 
						util.IptablesSrcFlag,
						true,
					)...,
				)
				entry.Specs = append(entry.Specs, targetSelectorIptEntrySpec...)
				entry.Specs = append(
					entry.Specs,
					util.IptablesJumpFlag,
					util.IptablesAccept,
					util.IptablesModuleFlag,
					util.IptablesCommentModuleFlag,
					util.IptablesCommentFlag,
					"ALLOW-" + craftPartialIptablesCommentFromSelector(ns, fromRule.NamespaceSelector, true) +
					"-TO-" + targetSelectorComment,
				)
				entries = append(entries, entry)
				continue
			}

			if fromRule.PodSelector != nil && fromRule.NamespaceSelector == nil {
				podLabelsWithOps, _, _ := parseSelector(fromRule.PodSelector)
				_, podLabelsWithoutOps := GetOperatorsAndLabels(podLabelsWithOps)
				if len(podLabelsWithoutOps) == 1 {
					if podLabelsWithoutOps[0] == "" {
						podLabelsWithoutOps[0] = "ns-" + ns
					}
				}
				sets = append(sets, podLabelsWithoutOps...)

				entry := &iptm.IptEntry{
					Chain: util.IptablesAzureIngressFromChain,
				}
				entry.Specs = append(
					entry.Specs, 
					craftPartialIptEntrySpecFromSelector(
						ns,
						fromRule.PodSelector, 
						util.IptablesSrcFlag,
						false,
					)...,
				)
				entry.Specs = append(entry.Specs, targetSelectorIptEntrySpec...)
				entry.Specs = append(
					entry.Specs,
					util.IptablesJumpFlag,
					util.IptablesAccept,
					util.IptablesModuleFlag,
					util.IptablesCommentModuleFlag,
					util.IptablesCommentFlag,
					"ALLOW-" + craftPartialIptablesCommentFromSelector(ns, fromRule.PodSelector, false) +
					"-TO-" + targetSelectorComment,
				)
				entries = append(entries, entry)
				continue
			}

			// fromRule has both namespaceSelector and podSelector set.
			// We should match the selected pods in the selected namespaces.
			// This allows traffic from podSelector intersects namespaceSelector
			// This is only supported in kubernetes version >= 1.11
			if !util.IsNewNwPolicyVerFlag {
				continue
			}

			nsLabelsWithOps, _, _ := parseSelector(fromRule.NamespaceSelector)
			_, nsLabelsWithoutOps := GetOperatorsAndLabels(nsLabelsWithOps)
			// Add namespaces prefix to distinguish namespace ipsets and pod ipsets
			for i, _ := range nsLabelsWithoutOps {
				nsLabelsWithoutOps[i] = "ns-" + nsLabelsWithoutOps[i]
			}
			lists = append(lists, nsLabelsWithoutOps...)

			podLabelsWithOps, _, _ := parseSelector(fromRule.PodSelector)
			_, podLabelsWithoutOps := GetOperatorsAndLabels(podLabelsWithOps)
			sets = append(sets, podLabelsWithoutOps...)

			entry := &iptm.IptEntry{
				Chain: util.IptablesAzureIngressFromChain,
			}
			entry.Specs = append(
				entry.Specs, 
				craftPartialIptEntrySpecFromSelector(
					ns,
					fromRule.NamespaceSelector, 
					util.IptablesSrcFlag,
					true,
				)...,
			)
			entry.Specs = append(
				entry.Specs, 
				craftPartialIptEntrySpecFromSelector(
					ns,
					fromRule.PodSelector, 
					util.IptablesSrcFlag,
					false,
				)...,
			)
			entry.Specs = append(entry.Specs, targetSelectorIptEntrySpec...)
			entry.Specs = append(
				entry.Specs,
				util.IptablesJumpFlag,
				util.IptablesAccept,
				util.IptablesModuleFlag,
				util.IptablesCommentModuleFlag,
				util.IptablesCommentFlag,
				"ALLOW-" + craftPartialIptablesCommentFromSelector(ns, fromRule.NamespaceSelector, true) +
				"-AND-" + craftPartialIptablesCommentFromSelector(ns, fromRule.PodSelector, false) +
				"-TO-" + targetSelectorComment,
			)
			entries = append(entries, entry)
		}
	}

	log.Printf("finished parsing ingress rule")
	return util.DropEmptyFields(sets), util.DropEmptyFields(lists), entries
}

func translateEgress(ns string, targetSelector metav1.LabelSelector, rules []networkingv1.NetworkPolicyEgressRule) ([]string, []string, []*iptm.IptEntry) {
	var (
		sets  []string // ipsets with type: net:hash
		lists []string // ipsets with type: list:set
		entries         []*iptm.IptEntry
	)

	log.Printf("started parsing egress rule")

	labelsWithOps, _, _ := parseSelector(&targetSelector)
	ops, labels := GetOperatorsAndLabels(labelsWithOps)
	if len(ops) == 1 && len(labels) == 1 {
		if ops[0] == "" && labels[0] == "" {
			// targetSelector is empty. Select all pods within the namespace
			labels[0] = "ns-" + ns
		}
	}
	sets = append(sets, labels...)
	targetSelectorIptEntrySpec := craftPartialIptEntrySpecFromOpsAndLabels(ns, ops, labels, util.IptablesSrcFlag, false)
	targetSelectorComment := craftPartialIptablesCommentFromSelector(ns, &targetSelector, false)
	for _, rule := range rules {
		allowExternal, portRuleExists, toRuleExists := false, false, false
		
		if len(rule.Ports) > 0 {
			portRuleExists = true
		}

		if rule.To != nil {
			if len(rule.To) == 0 {
				toRuleExists = true
				allowExternal = true
			}

			for _, toRule := range rule.To {
				if toRule.PodSelector != nil ||
					toRule.NamespaceSelector != nil ||
					toRule.IPBlock != nil {
					toRuleExists = true
					break
				}
			}
		}

		if !portRuleExists && !toRuleExists && !allowExternal {
			entry := &iptm.IptEntry{
				Chain: util.IptablesAzureEgressPortChain,
				Specs: targetSelectorIptEntrySpec,
			}
			entry.Specs = append(
				entry.Specs,
				util.IptablesModuleFlag,
				util.IptablesSetModuleFlag,
				util.IptablesMatchSetFlag,
				util.GetHashedName(util.KubeAllNamespacesFlag),
				util.IptablesDstFlag,
				util.IptablesJumpFlag,
				util.IptablesAccept,
				util.IptablesModuleFlag,
				util.IptablesCommentModuleFlag,
				util.IptablesCommentFlag,
				"ALLOW-ALL-FROM-" + targetSelectorComment + 
				"-TO-" + util.KubeAllNamespacesFlag,
			)

			entries = append(entries, entry)
			lists = append(lists, util.KubeAllNamespacesFlag)
			continue
		}

		// Only Ports rules exist
		if portRuleExists && !toRuleExists && !allowExternal {
			for _, portRule := range rule.Ports {
				entry := &iptm.IptEntry{
					Chain: util.IptablesAzureEgressPortChain,
					Specs: craftPartialIptEntrySpecFromPort(portRule, util.IptablesDstPortFlag),
				}
				entry.Specs = append(entry.Specs, targetSelectorIptEntrySpec...)
				entry.Specs = append(
					entry.Specs,
					util.IptablesJumpFlag,
					util.IptablesAccept,
					util.IptablesModuleFlag,
					util.IptablesCommentModuleFlag,
					util.IptablesCommentFlag,
					"ALLOW-ALL-FROM-" +
					craftPartialIptablesCommentFromPort(portRule, util.IptablesDstPortFlag) +
					targetSelectorComment,
				)
				entries = append(entries, entry)
			}
			continue
		}

		// toRuleExists
		if portRuleExists {
			for _, portRule := range rule.Ports {
				entry := &iptm.IptEntry{
					Chain: util.IptablesAzureEgressPortChain,
					Specs: craftPartialIptEntrySpecFromPort(portRule, util.IptablesDstPortFlag),
				}
				entry.Specs = append(entry.Specs, targetSelectorIptEntrySpec...)
				entry.Specs = append(
					entry.Specs,
					util.IptablesJumpFlag,
					util.IptablesAzureEgressToChain,
					util.IptablesModuleFlag,
					util.IptablesCommentModuleFlag,
					util.IptablesCommentFlag,
					"ALLOW-ALL-FROM-" +
					craftPartialIptablesCommentFromPort(portRule, util.IptablesDstPortFlag) +
					targetSelectorComment +
					"-TO-JUMP-TO-" + util.IptablesAzureEgressToChain,
				)
				entries = append(entries, entry)
			}
		} else {
			entry := &iptm.IptEntry{
				Chain: util.IptablesAzureEgressPortChain,
			}
			entry.Specs = append(entry.Specs, targetSelectorIptEntrySpec...)
			entry.Specs = append(
				entry.Specs,
				util.IptablesJumpFlag,
				util.IptablesAzureEgressToChain,
				util.IptablesModuleFlag,
				util.IptablesCommentModuleFlag,
				util.IptablesCommentFlag,
				"ALLOW-ALL-FROM-" +
				targetSelectorComment +
				"-TO-JUMP-TO-" + util.IptablesAzureEgressToChain,
			)
			entries = append(entries, entry)
		}

		if allowExternal {
			entry := &iptm.IptEntry{
				Chain: util.IptablesAzureEgressToChain,
			}
			entry.Specs = append(entry.Specs, targetSelectorIptEntrySpec...)
			entry.Specs = append(
				entry.Specs,
				util.IptablesJumpFlag,
				util.IptablesAccept,
				util.IptablesModuleFlag,
				util.IptablesCommentModuleFlag,
				util.IptablesCommentFlag,
				"ALLOW-ALL-FROM-" +
				targetSelectorComment,
			)
			entries = append(entries, entry)

			continue
		}

		for _, toRule := range rule.To {
			// Handle IPBlock field of NetworkPolicyPeer
			if toRule.IPBlock != nil {
				if len(toRule.IPBlock.CIDR) > 0 {
					cidrEntry := &iptm.IptEntry{
						Chain: util.IptablesAzureEgressToChain,
						Specs: targetSelectorIptEntrySpec,
					}
					cidrEntry.Specs	= append(
						cidrEntry.Specs,
						util.IptablesDFlag,
						toRule.IPBlock.CIDR,
					)
					cidrEntry.Specs	= append(
						cidrEntry.Specs,
						util.IptablesJumpFlag,
						util.IptablesAccept,
						util.IptablesModuleFlag,
						util.IptablesCommentModuleFlag,
						util.IptablesCommentFlag,
						"ALLOW-" + toRule.IPBlock.CIDR +
						"-FROM-" + targetSelectorComment,
					)
					entries = append(entries, cidrEntry)
				}
				if len(toRule.IPBlock.Except) > 0 {
					for _, except := range toRule.IPBlock.Except {
						exceptEntry := &iptm.IptEntry{
							Chain: util.IptablesAzureEgressToChain,
							Specs: targetSelectorIptEntrySpec,
						}
						exceptEntry.Specs	= append(
							exceptEntry.Specs,
							util.IptablesDFlag,
							except,
						)
						exceptEntry.Specs = append(
							exceptEntry.Specs,
							util.IptablesJumpFlag,
							util.IptablesDrop,
							util.IptablesModuleFlag,
							util.IptablesCommentModuleFlag,
							util.IptablesCommentFlag,
							"DROP-" + except +
							"-FROM-" + targetSelectorComment,
						)
						entries = append(entries, exceptEntry)
					}
				}
				continue
			}

			// Handle podSelector and namespaceSelector.
			// For PodSelector, use hash:net in ipset.
			// For NamespaceSelector, use set:list in ipset.
			if toRule.PodSelector == nil && toRule.NamespaceSelector == nil {
				continue
			}

			if toRule.PodSelector == nil && toRule.NamespaceSelector != nil {
				nsLabelsWithOps, _, _ := parseSelector(toRule.NamespaceSelector)
				_, nsLabelsWithoutOps := GetOperatorsAndLabels(nsLabelsWithOps)
				if len(nsLabelsWithoutOps) == 1 && nsLabelsWithoutOps[0] == "" {
					// Empty namespaceSelector. This selects all namespaces
					nsLabelsWithoutOps[0] = util.KubeAllNamespacesFlag
				} else {
					for i, _ := range nsLabelsWithoutOps {
						// Add namespaces prefix to distinguish namespace ipset lists and pod ipsets
						nsLabelsWithoutOps[i] = "ns-" + nsLabelsWithoutOps[i]
					}
				}
				lists = append(lists, nsLabelsWithoutOps...)

				entry := &iptm.IptEntry{
					Chain: util.IptablesAzureEgressToChain,
					Specs: targetSelectorIptEntrySpec,
				}
				entry.Specs = append(
					entry.Specs, 
					craftPartialIptEntrySpecFromSelector(
						ns,
						toRule.NamespaceSelector, 
						util.IptablesDstFlag,
						true,
					)...,
				)
				entry.Specs = append(
					entry.Specs,
					util.IptablesJumpFlag,
					util.IptablesAccept,
					util.IptablesModuleFlag,
					util.IptablesCommentModuleFlag,
					util.IptablesCommentFlag,
					"ALLOW-" + targetSelectorComment +
					"-TO-" + craftPartialIptablesCommentFromSelector(ns, toRule.NamespaceSelector, true),
				)
				entries = append(entries, entry)
				continue
			}

			if toRule.PodSelector != nil && toRule.NamespaceSelector == nil {
				podLabelsWithOps, _, _ := parseSelector(toRule.PodSelector)
				_, podLabelsWithoutOps := GetOperatorsAndLabels(podLabelsWithOps)
				if len(podLabelsWithoutOps) == 1 {
					if podLabelsWithoutOps[0] == "" {
						podLabelsWithoutOps[0] = "ns-" + ns
					}
				}
				sets = append(sets, podLabelsWithoutOps...)

				entry := &iptm.IptEntry{
					Chain: util.IptablesAzureEgressToChain,
					Specs: targetSelectorIptEntrySpec,
				}
				entry.Specs = append(
					entry.Specs, 
					craftPartialIptEntrySpecFromSelector(
						ns,
						toRule.PodSelector, 
						util.IptablesDstFlag,
						false,
					)...,
				)
				entry.Specs = append(
					entry.Specs,
					util.IptablesJumpFlag,
					util.IptablesAccept,
					util.IptablesModuleFlag,
					util.IptablesCommentModuleFlag,
					util.IptablesCommentFlag,
					"ALLOW-" + targetSelectorComment +
					"-TO-" + craftPartialIptablesCommentFromSelector(ns, toRule.PodSelector, false),
				)
				entries = append(entries, entry)
				continue
			}

			// toRule has both namespaceSelector and podSelector set.
			// We should match the selected pods in the selected namespaces.
			// This allows traffic from podSelector intersects namespaceSelector
			// This is only supported in kubernetes version >= 1.11
			if !util.IsNewNwPolicyVerFlag {
				continue
			}
			
			nsLabelsWithOps, _, _ := parseSelector(toRule.NamespaceSelector)
			_, nsLabelsWithoutOps := GetOperatorsAndLabels(nsLabelsWithOps)
			// Add namespaces prefix to distinguish namespace ipsets and pod ipsets
			for i, _ := range nsLabelsWithoutOps {
				nsLabelsWithoutOps[i] = "ns-" + nsLabelsWithoutOps[i]
			}
			lists = append(lists, nsLabelsWithoutOps...)

			podLabelsWithOps, _, _ := parseSelector(toRule.PodSelector)
			_, podLabelsWithoutOps := GetOperatorsAndLabels(podLabelsWithOps)
			sets = append(sets, podLabelsWithoutOps...)

			entry := &iptm.IptEntry{
				Chain: util.IptablesAzureEgressToChain,
				Specs: targetSelectorIptEntrySpec,
			}
			entry.Specs = append(
				entry.Specs, 
				craftPartialIptEntrySpecFromSelector(
					ns,
					toRule.NamespaceSelector, 
					util.IptablesDstFlag,
					true,
				)...,
			)
			entry.Specs = append(
				entry.Specs, 
				craftPartialIptEntrySpecFromSelector(
					ns,
					toRule.PodSelector, 
					util.IptablesDstFlag,
					false,
				)...,
			)
			entry.Specs = append(
				entry.Specs,
				util.IptablesJumpFlag,
				util.IptablesAccept,
				util.IptablesModuleFlag,
				util.IptablesCommentModuleFlag,
				util.IptablesCommentFlag,
				"ALLOW-" + targetSelectorComment +
				"-TO-" + craftPartialIptablesCommentFromSelector(ns, toRule.NamespaceSelector, true) +
				"-AND-" + craftPartialIptablesCommentFromSelector(ns, toRule.PodSelector, false),
			)
			entries = append(entries, entry)
		}
	}

	log.Printf("finished parsing egress rule")
	return util.DropEmptyFields(sets), util.DropEmptyFields(lists), entries
}

// Drop all non-whitelisted packets.
func getDefaultDropEntries(ns string, targetSelector metav1.LabelSelector) []*iptm.IptEntry {
	var entries []*iptm.IptEntry

	labelsWithOps, _, _ := parseSelector(&targetSelector)
	ops, labels := GetOperatorsAndLabels(labelsWithOps)
	if len(ops) == 1 && len(labels) == 1 {
		if ops[0] == "" && labels[0] == "" {
			// targetSelector is empty. Select all pods within the namespace
			labels[0] = "ns-" + ns
		}
	}

	targetSelectorIngressIptEntrySpec := craftPartialIptEntrySpecFromOpsAndLabels(ns, ops, labels, util.IptablesDstFlag, false)
	targetSelectorEgressIptEntrySpec := craftPartialIptEntrySpecFromOpsAndLabels(ns, ops, labels, util.IptablesSrcFlag, false)
	targetSelectorComment := craftPartialIptablesCommentFromSelector(ns, &targetSelector, false)

	entry := &iptm.IptEntry{
		Chain: util.IptablesAzureTargetSetsChain,
		Specs: targetSelectorIngressIptEntrySpec,
	}
	entry.Specs = append(
		entry.Specs,
		util.IptablesJumpFlag,
		util.IptablesDrop,
		util.IptablesModuleFlag,
		util.IptablesCommentModuleFlag,
		util.IptablesCommentFlag,
		"DROP-ALL-TO-" + targetSelectorComment,
	)
	entries = append(entries, entry)

	entry = &iptm.IptEntry{
		Chain: util.IptablesAzureTargetSetsChain,
		Specs: targetSelectorEgressIptEntrySpec,
	}
	entry.Specs = append(
		entry.Specs,
		util.IptablesJumpFlag,
		util.IptablesDrop,
		util.IptablesModuleFlag,
		util.IptablesCommentModuleFlag,
		util.IptablesCommentFlag,
		"DROP-ALL-FROM-" + targetSelectorComment,
	)
	entries = append(entries, entry)

	return entries
}

// Allow traffic from/to kube-system pods
func getAllowKubeSystemEntries(ns string, targetSelector metav1.LabelSelector) []*iptm.IptEntry {
	var entries []*iptm.IptEntry
	hashedKubeSystemSet := util.GetHashedName("ns-" + util.KubeSystemFlag)
	targetSelectorComment := craftPartialIptablesCommentFromSelector(ns, &targetSelector, false)
	allowKubeSystemIngress := &iptm.IptEntry{
		Chain: util.IptablesAzureKubeSystemChain,
		Specs: []string{
			util.IptablesModuleFlag,
			util.IptablesSetModuleFlag,
			util.IptablesMatchSetFlag,
			hashedKubeSystemSet,
			util.IptablesSrcFlag,
			util.IptablesJumpFlag,
			util.IptablesAccept,
			util.IptablesModuleFlag,
			util.IptablesCommentModuleFlag,
			util.IptablesCommentFlag,
			"ALLOW-" + "ns-" + util.KubeSystemFlag + 
			"-TO-" + targetSelectorComment,
		},
	}
	entries = append(entries, allowKubeSystemIngress)

	allowKubeSystemEgress := &iptm.IptEntry{
		Chain: util.IptablesAzureKubeSystemChain,
		Specs: []string{
			util.IptablesModuleFlag,
			util.IptablesSetModuleFlag,
			util.IptablesMatchSetFlag,
			hashedKubeSystemSet,
			util.IptablesDstFlag,
			util.IptablesJumpFlag,
			util.IptablesAccept,
			util.IptablesModuleFlag,
			util.IptablesCommentModuleFlag,
			util.IptablesCommentFlag,
			"ALLOW-" + targetSelectorComment +
			"-TO-" + "ns-" + util.KubeSystemFlag,
		},
	}
	entries = append(entries, allowKubeSystemEgress)

	return entries
}

// translatePolicy translates network policy object into a set of iptables rules.
// input:
// kubernetes network policy project
// output:
// 1. ipset set names generated from all podSelectors
// 2. ipset list names generated from all namespaceSelectors
// 3. iptables entries generated from the input network policy object.
func translatePolicy(npObj *networkingv1.NetworkPolicy) ([]string, []string, []*iptm.IptEntry) {
	var (
		resultSets  []string
		resultLists []string
		entries     []*iptm.IptEntry
	)

	log.Printf("Translating network policy:\n %+v", npObj)

	defer func() {
		log.Printf("Finished translatePolicy")
		log.Printf("sets: %v", resultSets)
		log.Printf("lists: %v", resultLists)
		log.Printf("entries: ")
		for _, entry := range entries {
			log.Printf("entry: %+v", entry)
		}	
	}()

	npNs := npObj.ObjectMeta.Namespace
	// Allow kube-system pods
	entries = append(entries, getAllowKubeSystemEntries(npNs, npObj.Spec.PodSelector)...)

	if len(npObj.Spec.PolicyTypes) == 0 {
		ingressSets, ingressLists, ingressEntries := translateIngress(npNs, npObj.Spec.PodSelector, npObj.Spec.Ingress)
		resultSets = append(resultSets, ingressSets...)
		resultLists = append(resultLists, ingressLists...)
		entries = append(entries, ingressEntries...)

		egressSets, egressLists, egressEntries := translateEgress(npNs, npObj.Spec.PodSelector, npObj.Spec.Egress)
		resultSets = append(resultSets, egressSets...)
		resultLists = append(resultLists, egressLists...)
		entries = append(entries, egressEntries...)

		entries = append(entries, getDefaultDropEntries(npNs, npObj.Spec.PodSelector)...)
		
		return util.UniqueStrSlice(resultSets), util.UniqueStrSlice(resultLists), entries
	}

	for _, ptype := range npObj.Spec.PolicyTypes {
		if ptype == networkingv1.PolicyTypeIngress {
			ingressSets, ingressLists, ingressEntries := translateIngress(npNs, npObj.Spec.PodSelector, npObj.Spec.Ingress)
			resultSets = append(resultSets, ingressSets...)
			resultLists = append(resultLists, ingressLists...)
			entries = append(entries, ingressEntries...)
		}

		if ptype == networkingv1.PolicyTypeEgress {
			egressSets, egressLists, egressEntries := translateEgress(npNs, npObj.Spec.PodSelector, npObj.Spec.Egress)
			resultSets = append(resultSets, egressSets...)
			resultLists = append(resultLists, egressLists...)
			entries = append(entries, egressEntries...)
		}
	}

	entries = append(entries, getDefaultDropEntries(npNs, npObj.Spec.PodSelector)...)

	resultSets, resultLists = util.UniqueStrSlice(resultSets), util.UniqueStrSlice(resultLists)

	return resultSets, resultLists, entries
}
