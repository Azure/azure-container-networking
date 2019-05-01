// Copyright 2018 Microsoft. All rights reserved.
// MIT License
package npm

import (
	"github.com/Azure/azure-container-networking/npm/iptm"
	"github.com/Azure/azure-container-networking/npm/util"
	networkingv1 "k8s.io/api/networking/v1"
)

type portsInfo struct {
	protocol string
	port     string
}

func translateIngress(ns string, targetSets []string, rules []networkingv1.NetworkPolicyIngressRule) ([]string, []string, []*iptm.IptEntry) {
	return nil, nil, nil
}

func translateIngress(ns string, targetSets []string, rules []networkingv1.NetworkPolicyIngressRule) ([]string, []string, []*iptm.IptEntry) {
	return nil, nil, nil
}

// Allow traffic from/to kube-system pods
func getAllowKubeSystemEntries(ns string, targetSets []string) []*iptm.IptEntry {
	var entries []*iptm.IptEntry

	if len(targetSets) == 0 {
		targetSets = append(targetSets, ns)
	}

	for _, targetSet := range targetSets {
		hashedTargetSetName := util.GetHashedName(targetSet)
		hashedKubeSystemSet := util.GetHashedName(util.KubeSystemFlag)
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
				hashedTargetSetName,
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
				hashedTargetSetName,
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

	// Get targeting pods.
	targetSets, _, _ := ParseSelector(npObj.Spec.PodSelector)

	if len(npObj.Spec.Ingress) > 0 || len(npObj.Spec.Egress) > 0 {
		entries = append(entries, getAllowKubeSystemEntries(npNs, targetSets)...)
	}

	if len(npObj.Spec.PolicyTypes) == 0 {
		ingressPodSets, ingressNsSets, ingressEntries := translateIngress(npNs, targetSets, npObj.Spec.Ingress)
		resultSets = append(resultSets, ingressPodSets...)
		resultLists = append(resultLists, ingressNsSets...)
		entries = append(entries, ingressEntries...)

		egressPodSets, egressNsSets, egressEntries := translateEgress(npNs, targetSets, npObj.Spec.Egress)
		resultSets = append(resultSets, egressPodSets...)
		resultLists = append(resultLists, egressNsSets...)
		entries = append(entries, egressEntries...)

		resultSets = append(resultSets, targetSets...)

		return util.UniqueStrSlice(resultSets), util.UniqueStrSlice(resultLists), entries
	}

	for _, ptype := range npObj.Spec.PolicyTypes {
		if ptype == networkingv1.PolicyTypeIngress {
			ingressPodSets, ingressNsSets, ingressEntries := translateIngress(npNs, targetSets, npObj.Spec.Ingress)
			resultSets = append(resultSets, ingressPodSets...)
			resultLists = append(resultLists, ingressNsSets...)
			entries = append(entries, ingressEntries...)
		}

		if ptype == networkingv1.PolicyTypeEgress {
			egressPodSets, egressNsSets, egressEntries := translateEgress(npNs, targetSets, npObj.Spec.Egress)
			resultSets = append(resultSets, egressPodSets...)
			resultLists = append(resultLists, egressNsSets...)
			entries = append(entries, egressEntries...)
		}
	}

	resultSets = append(resultSets, targetSets...)
	resultSets = append(resultSets, npNs)

	return util.UniqueStrSlice(resultSets), util.UniqueStrSlice(resultLists), entries
}
