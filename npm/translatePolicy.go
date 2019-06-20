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

func translateIngress(ns string, targetSelector metav1.LabelSelector, rules []networkingv1.NetworkPolicyIngressRule) ([]string, []string, []*iptm.IptEntry) {
	var (
		portRuleExists    = false
		fromRuleExists    = false
		protPortPairSlice []*portsInfo
		//podNsRuleSets     []string // pod sets listed in one ingress rules.
		//nsRuleLists       []string // namespace sets listed in one ingress rule
		policyRuleSets  []string // policy-wise pod sets
		policyRuleLists []string // policy-wise namespace sets
		entries         []*iptm.IptEntry
	)

	labels, _, _ := ParseSelector(&targetSelector)
	for i := range labels {
		label := labels[i]
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
					Name:       "allow-all-to-" + label,
					HashedName: hashedLabelName,
					Chain:      util.IptablesAzureIngressFromChain,
					Specs: []string{
						util.IptablesMatchFlag,
						util.IptablesSetModuleFlag,
						util.IptablesMatchSetFlag,
						hashedLabelName,
						util.IptablesDstFlag,
						util.IptablesJumpFlag,
						util.IptablesAccept,
						util.IptablesMatchFlag,
						util.IptablesCommentFlag,
					},
				}
				entries = append(entries, entry)
				continue
			}

			if !fromRuleExists {
				for _, protPortPair := range protPortPairSlice {
					entry := &iptm.IptEntry{
						Name:       "allow-to-ports-of-" + label,
						HashedName: hashedLabelName,
						Chain:      util.IptablesAzureIngressPortChain,
						Specs: []string{
							util.IptablesProtFlag,
							protPortPair.protocol,
							util.IptablesDstPortFlag,
							protPortPair.port,
							util.IptablesMatchFlag,
							util.IptablesSetModuleFlag,
							util.IptablesMatchSetFlag,
							hashedLabelName,
							util.IptablesDstFlag,
							util.IptablesJumpFlag,
							util.IptablesAzureIngressFromNsChain,
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
								Chain: util.IptablesAzureIngressFromNsChain,
								Specs: []string{
									util.IptablesMatchFlag,
									util.IptablesSetModuleFlag,
									util.IptablesMatchSetFlag,
									hashedLabelName,
									util.IptablesDstFlag,
									util.IptablesSFlag,
									fromRule.IPBlock.CIDR,
									util.IptablesJumpFlag,
									util.IptablesAccept,
								},
							}
							entries = append(entries, cidrEntry)
						}

						if len(fromRule.IPBlock.Except) > 0 {
							for _, except := range fromRule.IPBlock.Except {
								entry := &iptm.IptEntry{
									Chain: util.IptablesAzureIngressFromNsChain,
									Specs: []string{
										util.IptablesMatchFlag,
										util.IptablesSetModuleFlag,
										util.IptablesMatchSetFlag,
										hashedTargetSetName,
										util.IptablesDstFlag,
										util.IptablesSFlag,
										except,
										util.IptablesJumpFlag,
										util.IptablesDrop,
									},
								}
								entries = append(entries, entry)
							}
						}
					}

					// Handle podSelector and namespaceSelector
				}
			}
		}
	}

	log.Printf("finished parsing ingress rule")
	return policyRuleSets, policyRuleLists, entries
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
			Name:       util.KubeSystemFlag,
			HashedName: hashedKubeSystemSet,
			Chain:      util.IptablesAzureIngressPortChain,
			Specs: []string{
				util.IptablesMatchFlag,
				util.IptablesSetModuleFlag,
				util.IptablesMatchSetFlag,
				hashedKubeSystemSet,
				util.IptablesSrcFlag,
				util.IptablesMatchFlag,
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
			Name:       util.KubeSystemFlag,
			HashedName: hashedKubeSystemSet,
			Chain:      util.IptablesAzureEgressPortChain,
			Specs: []string{
				util.IptablesMatchFlag,
				util.IptablesSetModuleFlag,
				util.IptablesMatchSetFlag,
				hashedLabelName,
				util.IptablesSrcFlag,
				util.IptablesMatchFlag,
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
