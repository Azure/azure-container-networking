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

func craftPartialIptEntrySpecFromOpAndLabel(op, label, srcOrDstFlag string) []string {
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

func craftPartialIptEntrySpecFromOpsAndLabels(ops, labels []string, srcOrDstFlag string) []string {
	var spec []string

	for i, _ := range ops {
		spec = append(spec, craftPartialIptEntrySpecFromOpAndLabel(ops[i], labels[i], srcOrDstFlag)...)
	}

	return spec
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

	labels, _, _ := ParseSelector(&targetSelector)
	for i := range labels {
		op, label := util.GetOperatorAndLabel(labels[i])
		sets = append(sets, label)
		log.Printf("Parsing iptables for label %s", label)

		hashedLabelName := util.GetHashedName(label)
		for _, rule := range rules {
			// parse Ports field
			for _, portRule := range rule.Ports {
				protPortPairSlice = append(protPortPairSlice,
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
					Chain: util.IptablesAzureIngressFromChain,
					Specs: []string{
						util.IptablesModuleFlag,
						util.IptablesSetModuleFlag,
						op,
						util.IptablesMatchSetFlag,
						hashedLabelName,
						util.IptablesDstFlag,
						util.IptablesJumpFlag,
						util.IptablesAccept,
						util.IptablesModuleFlag,
						util.IptablesCommentModuleFlag,
						util.IptablesCommentFlag,
						"allow-all-to-" + op + label,
					},
				}
				entries = append(entries, entry)
				continue
			}

			if !fromRuleExists {
				for _, protPortPair := range protPortPairSlice {
					entry := &iptm.IptEntry{
						Chain: util.IptablesAzureIngressPortChain,
						Specs: []string{
							util.IptablesProtFlag,
							protPortPair.protocol,
							util.IptablesDstPortFlag,
							protPortPair.port,
							util.IptablesModuleFlag,
							util.IptablesSetModuleFlag,
							op,
							util.IptablesMatchSetFlag,
							hashedLabelName,
							util.IptablesDstFlag,
							util.IptablesJumpFlag,
							util.IptablesAccept,
							util.IptablesModuleFlag,
							util.IptablesCommentModuleFlag,
							util.IptablesCommentFlag,
							"allow-to-" + protPortPair.port + "-port-of-" +
							op + label,
						},
					}
					entries = append(entries, entry)
				}
				continue
			}

			if !portRuleExists {
				for _, fromRule := range rule.From {
					// Handle IPBlock field of NetworkPolicyPeer
					if fromRule.IPBlock != nil {
						if len(fromRule.IPBlock.CIDR) > 0 {
							cidrEntry := &iptm.IptEntry{
								Chain: util.IptablesAzureIngressFromChain,
								Specs: []string{
									util.IptablesModuleFlag,
									util.IptablesSetModuleFlag,
									op,
									util.IptablesMatchSetFlag,
									hashedLabelName,
									util.IptablesDstFlag,
									util.IptablesSFlag,
									fromRule.IPBlock.CIDR,
									util.IptablesJumpFlag,
									util.IptablesAccept,
									util.IptablesModuleFlag,
									util.IptablesCommentModuleFlag,
									util.IptablesCommentFlag,
									"allow-" + fromRule.IPBlock.CIDR +
									"-to-" + op + label,
								},
							}														
							entries = append(entries, cidrEntry)
						}

						if len(fromRule.IPBlock.Except) > 0 {
							for _, except := range fromRule.IPBlock.Except {
								entry := &iptm.IptEntry{
									Chain: util.IptablesAzureIngressFromChain,
									Specs: []string{
										util.IptablesModuleFlag,
										util.IptablesSetModuleFlag,
										op,
										util.IptablesMatchSetFlag,
										hashedLabelName,
										util.IptablesDstFlag,
										util.IptablesSFlag,
										except,
										util.IptablesJumpFlag,
										util.IptablesDrop,
										util.IptablesModuleFlag,
										util.IptablesCommentModuleFlag,
										util.IptablesCommentFlag,
										"block-" + except +
										"-to-" + op + label,
									},
								}
								entries = append(entries, entry)
							}
						}
					}

					// Handle podSelector and namespaceSelector.
					// For PodSelector, use hash:net in ipset.
					// For NamespaceSelector, use set:list in ipset.
					if fromRule.PodSelector == nil && fromRule.NamespaceSelector == nil {
						continue
					}

					if fromRule.PodSelector == nil && fromRule.NamespaceSelector != nil {
						selectorLabels, _, _ := ParseSelector(fromRule.NamespaceSelector)
						lists = append(lists, selectorLabels...)
						for _, sLabel := range selectorLabels {
							selectorOp, selectorLabel := util.GetOperatorAndLabel(sLabel)
							hashedSelectorLabelName := util.GetHashedName(selectorLabel)						
							entry := &iptm.IptEntry{
								Chain: util.IptablesAzureIngressFromChain,
								Specs: []string{
									util.IptablesModuleFlag,
									util.IptablesSetModuleFlag,
									selectorOp,
									util.IptablesMatchSetFlag,
									hashedSelectorLabelName,
									util.IptablesSrcFlag,
									util.IptablesModuleFlag,
									util.IptablesSetModuleFlag,
									op,
									util.IptablesMatchSetFlag,
									hashedLabelName,
									util.IptablesDstFlag,
									util.IptablesJumpFlag,
									util.IptablesAccept,
									util.IptablesModuleFlag,
									util.IptablesCommentModuleFlag,
									util.IptablesCommentFlag,
									"allow-" + selectorOp + selectorLabel +
									"-to-" + op + label,
								},
							}
							entries = append(entries, entry)
						}
						continue
					}

					if fromRule.PodSelector != nil && fromRule.NamespaceSelector == nil {
						selectorLabels, _, _ := ParseSelector(fromRule.NamespaceSelector)
						sets = append(sets, selectorLabels...)
						for _, sLabel := range selectorLabels {
							selectorOp, selectorLabel := util.GetOperatorAndLabel(sLabel)
							hashedSelectorLabelName := util.GetHashedName(selectorLabel)
							entry := &iptm.IptEntry{
								Chain: util.IptablesAzureIngressFromChain,
								Specs: []string{
									util.IptablesModuleFlag,
									util.IptablesSetModuleFlag,
									selectorOp,
									util.IptablesMatchSetFlag,
									hashedSelectorLabelName,
									util.IptablesSrcFlag,
									util.IptablesModuleFlag,
									util.IptablesSetModuleFlag,
									op,
									util.IptablesMatchSetFlag,
									hashedLabelName,
									util.IptablesDstFlag,
									util.IptablesJumpFlag,
									util.IptablesAccept,
									util.IptablesModuleFlag,
									util.IptablesCommentModuleFlag,
									util.IptablesCommentFlag,
									"allow-" + selectorOp + selectorLabel +
									"-to-" + op + label,
								},
							}
							entries = append(entries, entry)
							continue
						}
					}

					// fromRule has both namespaceSelector and podSelector set.
					// We should match the selected pods in the selected namespaces.
					// This allows traffic from podSelector intersects namespaceSelector
					// This is only supported in kubernetes version >= 1.11
					if !util.IsNewNwPolicyVerFlag {
						continue
					}

					nsSelectorLabels, _, _ := ParseSelector(fromRule.NamespaceSelector)
					lists = append(lists, nsSelectorLabels...)
					for _, nsLabel := range nsSelectorLabels {
						nsSelectorOp, nsSelectorLabel := util.GetOperatorAndLabel(nsLabel)
						hashedNsSelectorLabelName := util.GetHashedName(nsSelectorLabel)

						podSelectorLabels, _, _ := ParseSelector(fromRule.PodSelector)
						sets = append(sets, podSelectorLabels...)
						for _, podLabel := range podSelectorLabels {
							podSelectorOp, podSelectorLabel := util.GetOperatorAndLabel(podLabel)
							hashedPodSelectorLabelName := util.GetHashedName(podSelectorLabel)
							entry := &iptm.IptEntry{
								Chain: util.IptablesAzureIngressFromChain,
								Specs: []string{
									util.IptablesModuleFlag,
									util.IptablesSetModuleFlag,
									nsSelectorOp,
									util.IptablesMatchSetFlag,
									hashedNsSelectorLabelName,
									util.IptablesSrcFlag,
									util.IptablesModuleFlag,
									util.IptablesSetModuleFlag,
									podSelectorOp,
									util.IptablesMatchSetFlag,
									hashedPodSelectorLabelName,
									util.IptablesSrcFlag,
									util.IptablesModuleFlag,
									util.IptablesSetModuleFlag,
									op,
									util.IptablesMatchSetFlag,
									hashedLabelName,
									util.IptablesDstFlag,
									util.IptablesJumpFlag,
									util.IptablesAzureIngressFromChain,
									util.IptablesModuleFlag,
									util.IptablesCommentModuleFlag,
									util.IptablesCommentFlag,
									"allow-" + nsSelectorOp + nsSelectorLabel +
									"-AND-" + podSelectorOp + podSelectorLabel +
									"-to-" + op + label,
								},
							}
							entries = append(entries, entry)
						}
					}
				}
			}
		}
	}

	for _, entry := range entries {
		entry.Specs = util.DropEmptyFields(entry.Specs)
	}

	log.Printf("finished parsing ingress rule")
	return sets, lists, entries
}

func translateEgress(ns string, targetSelector metav1.LabelSelector, rules []networkingv1.NetworkPolicyEgressRule) ([]string, []string, []*iptm.IptEntry) {
	return nil, nil, nil
}

// Allow traffic from/to kube-system pods
func getAllowKubeSystemEntries(ns string, targetSelector metav1.LabelSelector) []*iptm.IptEntry {
	var entries []*iptm.IptEntry

	labels, _, _ := ParseSelector(&targetSelector)
	hashedKubeSystemSet := util.GetHashedName(util.KubeSystemFlag)
	for _, label := range labels {
		hashedLabelName := util.GetHashedName(label)
		allowKubeSystemIngress := &iptm.IptEntry{
			Name:  util.KubeSystemFlag,
			Chain: util.IptablesAzureIngressPortChain,
			Specs: []string{
				util.IptablesModuleFlag,
				util.IptablesSetModuleFlag,
				util.IptablesMatchSetFlag,
				hashedKubeSystemSet,
				util.IptablesSrcFlag,
				util.IptablesModuleFlag,
				util.IptablesSetModuleFlag,
				util.IptablesMatchSetFlag,
				hashedLabelName,
				util.IptablesDstFlag,
				util.IptablesJumpFlag,
				util.IptablesAccept,
			},
		}
		entries = append(entries, allowKubeSystemIngress)

		allowKubeSystemEgress := &iptm.IptEntry{
			Name:  util.KubeSystemFlag,
			Chain: util.IptablesAzureEgressPortChain,
			Specs: []string{
				util.IptablesModuleFlag,
				util.IptablesSetModuleFlag,
				util.IptablesMatchSetFlag,
				hashedLabelName,
				util.IptablesSrcFlag,
				util.IptablesModuleFlag,
				util.IptablesSetModuleFlag,
				util.IptablesMatchSetFlag,
				hashedKubeSystemSet,
				util.IptablesDstFlag,
				util.IptablesJumpFlag,
				util.IptablesAccept,
			},
		}
		entries = append(entries, allowKubeSystemEgress)
	}

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
	if len(npObj.Spec.Ingress) > 0 || len(npObj.Spec.Egress) > 0 {
		entries = append(entries, getAllowKubeSystemEntries(npNs, npObj.Spec.PodSelector)...)
	}

	if len(npObj.Spec.PolicyTypes) == 0 {
		ingressPodSets, ingressNsSets, ingressEntries := translateIngress(npNs, npObj.Spec.PodSelector, npObj.Spec.Ingress)
		resultSets = append(resultSets, ingressPodSets...)
		resultLists = append(resultLists, ingressNsSets...)
		entries = append(entries, ingressEntries...)

		egressPodSets, egressNsSets, egressEntries := translateEgress(npNs, npObj.Spec.PodSelector, npObj.Spec.Egress)
		resultSets = append(resultSets, egressPodSets...)
		resultLists = append(resultLists, egressNsSets...)
		entries = append(entries, egressEntries...)

		return util.UniqueStrSlice(resultSets), util.UniqueStrSlice(resultLists), entries
	}

	for _, ptype := range npObj.Spec.PolicyTypes {
		if ptype == networkingv1.PolicyTypeIngress {
			ingressPodSets, ingressNsSets, ingressEntries := translateIngress(npNs, npObj.Spec.PodSelector, npObj.Spec.Ingress)
			resultSets = append(resultSets, ingressPodSets...)
			resultLists = append(resultLists, ingressNsSets...)
			entries = append(entries, ingressEntries...)
		}

		if ptype == networkingv1.PolicyTypeEgress {
			egressPodSets, egressNsSets, egressEntries := translateEgress(npNs, npObj.Spec.PodSelector, npObj.Spec.Egress)
			resultSets = append(resultSets, egressPodSets...)
			resultLists = append(resultLists, egressNsSets...)
			entries = append(entries, egressEntries...)
		}
	}

	resultSets = append(resultSets, npNs)

	return util.UniqueStrSlice(resultSets), util.UniqueStrSlice(resultLists), entries
}
