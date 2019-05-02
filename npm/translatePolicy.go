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
		isAppliedToNs     = false
		protPortPairSlice []*portsInfo
		podNsRuleSets     []string // pod sets listed in one ingress rules.
		nsRuleLists       []string // namespace sets listed in one ingress rule
		policyRuleSets    []string // policy-wise pod sets
		policyRuleLists   []string // policy-wise namespace sets
		entries           []*iptm.IptEntry
	)

	labels, keys, vals := ParseSelector(&targetSelector)
	for i := range labels {
		label, key, val := labels[i], keys[i], vals[i]
		log.Printf("Parsing iptables for label %s", label)
		
		hashedLabelName := util.GetHashedName(label)

		for _, rule := range rules {
			// parse Ports field
			for _, portRule := range rule.Ports {
				protPortPairSlice = append(protPortPairSlice,
					&portsInfo{
						protocol: string(*portRule.ProtoMessage),
						port: protRule.Port.String(),
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

			// TODO
			if !portRuleExists && !fromRuleExists {
				break
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
				util.IptablesSetFlag,
				util.IptablesMatchSetFlag,
				hashedKubeSystemSet,
				util.IptablesSrcFlag,
				util.IptablesMatchFlag,
				util.IptablesSetFlag,
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
				util.IptablesSetFlag,
				util.IptablesMatchSetFlag,
				hashedLabelName,
				util.IptablesSrcFlag,
				util.IptablesMatchFlag,
				util.IptablesSetFlag,
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

		resultSets = append(resultSets, npObj.Spec.PodSelector...)

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

	resultSets = append(resultSets, npObj.Spec.PodSelector...)
	resultSets = append(resultSets, npNs)

	return util.UniqueStrSlice(resultSets), util.UniqueStrSlice(resultLists), entries
}
