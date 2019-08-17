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

func craftPartialIptEntrySpecFromOpsAndLabels(ops, labels []string, srcOrDstFlag string, isNamespaceSelector bool) []string {
	var spec []string

	for i, _ := range ops {
		spec = append(spec, craftPartialIptEntrySpecFromOpAndLabel(ops[i], labels[i], srcOrDstFlag, isNamespaceSelector)...)
	}

	return spec
}

func craftPartialIptEntrySpecFromSelector(selector *metav1.LabelSelector, srcOrDstFlag string, isNamespaceSelector bool) []string {
	labelsWithOps, _, _ := parseSelector(selector)
	ops, labels := GetOperatorsAndLabels(labelsWithOps)
	return craftPartialIptEntrySpecFromOpsAndLabels(ops, labels, srcOrDstFlag, isNamespaceSelector)
}

func craftPartialIptablesCommentFromSelector(selector *metav1.LabelSelector, isNamespaceSelector bool) string {
	if selector == nil {
		return "none"
	}

	if len(selector.MatchExpressions) == 0 && len(selector.MatchLabels) == 0 {
		return util.KubeAllNamespacesFlag
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
		portRuleExists    = false
		fromRuleExists    = false
		protPortPairSlice []*portsInfo
		sets  []string // ipsets with type: net:hash
		lists []string // ipsets with type: list:set
		entries         []*iptm.IptEntry
	)

	log.Printf("started parsing ingress rule")

	labelsWithOps, _, _ := parseSelector(&targetSelector)
	ops, labels := GetOperatorsAndLabels(labelsWithOps)
	// targetSelector is empty. Select all pods within the namespace
	if len(ops) == 0 && len(labels) == 0 {
		ops = append(ops, "")
		labels = append(labels, "ns-" + ns)
	}
	sets = append(sets, labels...)
	targetSelectorIptEntrySpec := craftPartialIptEntrySpecFromOpsAndLabels(ops, labels, util.IptablesDstFlag, false)
	targetSelectorComment := craftPartialIptablesCommentFromSelector(&targetSelector, false)
	
	for _, rule := range rules {
		// parse Ports field
		for _, portRule := range rule.Ports {
			protPortPairSlice = append(
				protPortPairSlice,
				&portsInfo{
					protocol: string(*portRule.Protocol),
					port:     portRule.Port.String(),
				},
			)
			portRuleExists = true
		}

		if rule.From != nil {
			for _, fromRule := range rule.From {
				if fromRule.PodSelector != nil ||
					fromRule.NamespaceSelector != nil ||
					fromRule.IPBlock != nil {
					fromRuleExists = true
				}
			}
		}

		if !portRuleExists && !fromRuleExists {
			entry := &iptm.IptEntry{
				Chain: util.IptablesAzureIngressPortChain,
				Specs: targetSelectorIptEntrySpec,
			}
			entry.Specs = append(
				entry.Specs,
				util.IptablesJumpFlag,
				util.IptablesAccept,
				util.IptablesModuleFlag,
				util.IptablesCommentModuleFlag,
				util.IptablesCommentFlag,
				"ALLOW-ALL-TO-" + targetSelectorComment,
			)

			entries = append(entries, entry)
			lists = append(lists, util.KubeAllNamespacesFlag)
			continue
		}

		// Only Ports rules exist
		if !fromRuleExists {
			for _, protPortPair := range protPortPairSlice {
				entry := &iptm.IptEntry{
					Chain: util.IptablesAzureIngressPortChain,
					Specs: []string{
						util.IptablesProtFlag,
						protPortPair.protocol,
						util.IptablesDstPortFlag,
						protPortPair.port,
					},
				}
				entry.Specs = append(entry.Specs, targetSelectorIptEntrySpec...)
				entry.Specs = append(
					entry.Specs,
					util.IptablesJumpFlag,
					util.IptablesAccept,
					util.IptablesModuleFlag,
					util.IptablesCommentModuleFlag,
					util.IptablesCommentFlag,
					"ALLOW-ALL-TO-" + protPortPair.port + "-PORT-OF-" +
					targetSelectorComment,
				)
				entries = append(entries, entry)
			}
			continue
		}

		if portRuleExists {
			for _, protPortPair := range protPortPairSlice {
				entry := &iptm.IptEntry{
					Chain: util.IptablesAzureIngressPortChain,
					Specs: []string{
						util.IptablesProtFlag,
						protPortPair.protocol,
						util.IptablesDstPortFlag,
						protPortPair.port,
					},
				}
				entry.Specs = append(entry.Specs, targetSelectorIptEntrySpec...)
				entry.Specs = append(
					entry.Specs,
					util.IptablesJumpFlag,
					util.IptablesAzureIngressFromChain,
					util.IptablesModuleFlag,
					util.IptablesCommentModuleFlag,
					util.IptablesCommentFlag,
					"ALLOW-ALL-TO-" + protPortPair.port + "-PORT-OF-" +
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
				// Add namespaces prefix to distinguish namespace ipsets and pod ipsets
				for i, _ := range nsLabelsWithoutOps {
					nsLabelsWithoutOps[i] = "ns-" + nsLabelsWithoutOps[i]
				}
				lists = append(lists, nsLabelsWithoutOps...)

				entry := &iptm.IptEntry{
					Chain: util.IptablesAzureIngressFromChain,
				}
				entry.Specs = append(
					entry.Specs, 
					craftPartialIptEntrySpecFromSelector(
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
					"ALLOW-" + craftPartialIptablesCommentFromSelector(fromRule.NamespaceSelector, true) +
					"-TO-" + targetSelectorComment,
				)
				entries = append(entries, entry)
				continue
			}

			if fromRule.PodSelector != nil && fromRule.NamespaceSelector == nil {
				podLabelsWithOps, _, _ := parseSelector(fromRule.PodSelector)
				_, podLabelsWithoutOps := GetOperatorsAndLabels(podLabelsWithOps)
				sets = append(sets, podLabelsWithoutOps...)

				entry := &iptm.IptEntry{
					Chain: util.IptablesAzureIngressFromChain,
				}
				entry.Specs = append(
					entry.Specs, 
					craftPartialIptEntrySpecFromSelector(
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
					"ALLOW-" + craftPartialIptablesCommentFromSelector(fromRule.PodSelector, false) +
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
					fromRule.NamespaceSelector, 
					util.IptablesSrcFlag,
					true,
				)...,
			)
			entry.Specs = append(
				entry.Specs, 
				craftPartialIptEntrySpecFromSelector(
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
				"ALLOW-" + craftPartialIptablesCommentFromSelector(fromRule.NamespaceSelector, true) +
				"-AND-" + craftPartialIptablesCommentFromSelector(fromRule.PodSelector, false) +
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
		portRuleExists    = false
		toRuleExists    = false
		protPortPairSlice []*portsInfo
		sets  []string // ipsets with type: net:hash
		lists []string // ipsets with type: list:set
		entries         []*iptm.IptEntry
	)

	log.Printf("started parsing egress rule")

	labelsWithOps, _, _ := parseSelector(&targetSelector)
	ops, labels := GetOperatorsAndLabels(labelsWithOps)
	// targetSelector is empty. Select all pods within the namespace
	if len(ops) == 0 && len(labels) == 0 {
		ops = append(ops, "")
		labels = append(labels, "ns-" + ns)
	}
	sets = append(sets, labels...)
	targetSelectorIptEntrySpec := craftPartialIptEntrySpecFromOpsAndLabels(ops, labels, util.IptablesSrcFlag, false)
	targetSelectorComment := craftPartialIptablesCommentFromSelector(&targetSelector, false)
	for _, rule := range rules {
		// parse Ports field
		for _, portRule := range rule.Ports {
			protPortPairSlice = append(
				protPortPairSlice,
				&portsInfo{
					protocol: string(*portRule.Protocol),
					port:     portRule.Port.String(),
				},
			)
			portRuleExists = true
		}

		if rule.To != nil {
			for _, toRule := range rule.To {
				if toRule.PodSelector != nil ||
					toRule.NamespaceSelector != nil ||
					toRule.IPBlock != nil {
					toRuleExists = true
				}
			}
		}

		if !portRuleExists && !toRuleExists {
			entry := &iptm.IptEntry{
				Chain: util.IptablesAzureEgressPortChain,
				Specs: targetSelectorIptEntrySpec,
			}
			entry.Specs = append(
				entry.Specs,
				util.IptablesJumpFlag,
				util.IptablesAccept,
				util.IptablesModuleFlag,
				util.IptablesCommentModuleFlag,
				util.IptablesCommentFlag,
				"ALLOW-ALL-FROM-" + targetSelectorComment,
			)

			entries = append(entries, entry)
			lists = append(lists, util.KubeAllNamespacesFlag)
			continue
		}

		// Only Ports rules exist
		if !toRuleExists {
			for _, protPortPair := range protPortPairSlice {
				entry := &iptm.IptEntry{
					Chain: util.IptablesAzureEgressPortChain,
					Specs: []string{
						util.IptablesProtFlag,
						protPortPair.protocol,
						util.IptablesDstPortFlag,
						protPortPair.port,
					},
				}
				entry.Specs = append(entry.Specs, targetSelectorIptEntrySpec...)
				entry.Specs = append(
					entry.Specs,
					util.IptablesJumpFlag,
					util.IptablesAccept,
					util.IptablesModuleFlag,
					util.IptablesCommentModuleFlag,
					util.IptablesCommentFlag,
					"ALLOW-ALL-FROM-" + protPortPair.port + "-PORT-OF-" +
					targetSelectorComment,
				)
				entries = append(entries, entry)
			}
			continue
		}

		if portRuleExists {
			for _, protPortPair := range protPortPairSlice {
				entry := &iptm.IptEntry{
					Chain: util.IptablesAzureEgressPortChain,
					Specs: []string{
						util.IptablesProtFlag,
						protPortPair.protocol,
						util.IptablesDstPortFlag,
						protPortPair.port,
					},
				}
				entry.Specs = append(entry.Specs, targetSelectorIptEntrySpec...)
				entry.Specs = append(
					entry.Specs,
					util.IptablesJumpFlag,
					util.IptablesAzureEgressToChain,
					util.IptablesModuleFlag,
					util.IptablesCommentModuleFlag,
					util.IptablesCommentFlag,
					"ALLOW-ALL-FROM-" + protPortPair.port + "-PORT-OF-" +
					targetSelectorComment +
					"-TO-JUMP-TO-" + util.IptablesAzureEgressToChain,
				)
				entries = append(entries, entry)
			}
		} else {
			entry := &iptm.IptEntry{
				Chain: util.IptablesAzureEgressToChain,
			}
			entry.Specs = append(entry.Specs, targetSelectorIptEntrySpec...)
			entry.Specs = append(
				entry.Specs,
				util.IptablesJumpFlag,
				util.IptablesAzureEgressToChain,
				util.IptablesModuleFlag,
				util.IptablesCommentModuleFlag,
				util.IptablesCommentFlag,
				"ALLOW-ALL-TO-" +
				targetSelectorComment +
				"-TO-JUMP-TO-" + util.IptablesAzureEgressToChain,
			)
			entries = append(entries, entry)
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
				// Add namespaces prefix to distinguish namespace ipsets and pod ipsets
				for i, _ := range nsLabelsWithoutOps {
					nsLabelsWithoutOps[i] = "ns-" + nsLabelsWithoutOps[i]
				}
				lists = append(lists, nsLabelsWithoutOps...)

				entry := &iptm.IptEntry{
					Chain: util.IptablesAzureEgressToChain,
					Specs: targetSelectorIptEntrySpec,
				}
				entry.Specs = append(
					entry.Specs, 
					craftPartialIptEntrySpecFromSelector(
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
					"-TO-" + craftPartialIptablesCommentFromSelector(toRule.NamespaceSelector, true),
				)
				entries = append(entries, entry)
				continue
			}

			if toRule.PodSelector != nil && toRule.NamespaceSelector == nil {
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
					"-TO-" + craftPartialIptablesCommentFromSelector(toRule.PodSelector, false),
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
					toRule.NamespaceSelector, 
					util.IptablesDstFlag,
					true,
				)...,
			)
			entry.Specs = append(
				entry.Specs, 
				craftPartialIptEntrySpecFromSelector(
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
				"-TO-" + craftPartialIptablesCommentFromSelector(toRule.NamespaceSelector, true) +
				"-AND-" + craftPartialIptablesCommentFromSelector(toRule.PodSelector, false),
			)
			entries = append(entries, entry)
		}
	}

	log.Printf("finished parsing egress rule")
	return util.DropEmptyFields(sets), util.DropEmptyFields(lists), entries
}

// Allow traffic from/to kube-system pods
func getAllowKubeSystemEntries(ns string, targetSelector metav1.LabelSelector) []*iptm.IptEntry {
	var entries []*iptm.IptEntry
	hashedKubeSystemSet := util.GetHashedName("ns-" + util.KubeSystemFlag)
	targetSelectorComment := craftPartialIptablesCommentFromSelector(&targetSelector, false)
	allowKubeSystemIngress := &iptm.IptEntry{
		Chain: util.IptablesAzureChain,
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
		Chain: util.IptablesAzureChain,
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

	return util.UniqueStrSlice(resultSets), util.UniqueStrSlice(resultLists), entries
}
