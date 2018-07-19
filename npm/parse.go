// Copyright 2018 Microsoft. All rights reserved.
// MIT License
package npm

import (
	"fmt"

	"github.com/Azure/azure-container-networking/log"
	"github.com/Azure/azure-container-networking/npm/iptm"
	"github.com/Azure/azure-container-networking/npm/util"
	networkingv1 "k8s.io/api/networking/v1"
)

// azureNpmPrefix defines prefix for ipset.
const azureNpmPrefix string = "azure-npm-"

type portsInfo struct {
	protocol string
	port     string
}

func parseIngress(ns string, targetSets []string, rules []networkingv1.NetworkPolicyIngressRule) ([]string, []string, []*iptm.IptEntry) {
	var (
		portRuleExists    = false
		fromRuleExists    = false
		isAppliedToNs     = false
		protPortPairSlice []*portsInfo
		PodNsRuleSets     []string // pod sets listed in Ingress rules.
		nsRuleLists       []string // namespace sets listed in Ingress rules
		entries           []*iptm.IptEntry
		ipblock           *networkingv1.IPBlock
	)

	if len(targetSets) == 0 {
		targetSets = append(targetSets, ns)
		isAppliedToNs = true
	}

	for _, rule := range rules {
		for _, portRule := range rule.Ports {
			protPortPairSlice = append(protPortPairSlice,
				&portsInfo{
					protocol: string(*portRule.Protocol),
					port:     fmt.Sprint(portRule.Port.IntVal),
				})

			portRuleExists = true
		}

		for _, fromRule := range rule.From {
			if fromRule.PodSelector != nil {
				if len(fromRule.PodSelector.MatchLabels) == 0 {
					PodNsRuleSets = append(PodNsRuleSets, ns)
				}

				for podLabelKey, podLabelVal := range fromRule.PodSelector.MatchLabels {
					PodNsRuleSets = append(PodNsRuleSets, util.KubeAllNamespacesFlag+"-"+podLabelKey+":"+podLabelVal)
				}
			}

			if fromRule.NamespaceSelector != nil {
				if len(fromRule.NamespaceSelector.MatchLabels) == 0 {
					nsRuleLists = append(nsRuleLists, util.KubeAllNamespacesFlag)
				}

				for nsLabelKey, nsLabelVal := range fromRule.NamespaceSelector.MatchLabels {
					nsRuleLists = append(nsRuleLists, "ns-"+nsLabelKey+":"+nsLabelVal)
				}
			}

			if fromRule.IPBlock != nil {
				ipblock = fromRule.IPBlock
			}

			fromRuleExists = true
		}
	}

	if isAppliedToNs {
		hashedTargetSetName := util.GetHashedName(ns)

		nsDrop := &iptm.IptEntry{
			Name:       ns,
			HashedName: hashedTargetSetName,
			Chain:      util.IptablesAzureTargetSetsChain,
			Specs: []string{
				util.IptablesMatchFlag,
				util.IptablesSetFlag,
				util.IptablesMatchSetFlag,
				hashedTargetSetName,
				util.IptablesDstFlag,
				util.IptablesJumpFlag,
				util.IptablesDrop,
			},
		}
		entries = append(entries, nsDrop)
	}

	// Use hashed string for ipset name to avoid string length limit of ipset.
	for _, targetSet := range targetSets {
		log.Printf("Parsing iptables for label %s", targetSet)

		hashedTargetSetName := util.GetHashedName(targetSet)

		if len(rules) == 0 {
			drop := &iptm.IptEntry{
				Name:       targetSet,
				HashedName: hashedTargetSetName,
				Chain:      util.IptablesAzureIngressPortChain,
				Specs: []string{
					util.IptablesMatchFlag,
					util.IptablesSetFlag,
					util.IptablesMatchSetFlag,
					hashedTargetSetName,
					util.IptablesDstFlag,
					util.IptablesJumpFlag,
					util.IptablesDrop,
				},
			}
			entries = append(entries, drop)
			continue
		}

		if !portRuleExists && !fromRuleExists {
			allow := &iptm.IptEntry{
				Name:       targetSet,
				HashedName: hashedTargetSetName,
				Chain:      util.IptablesAzureIngressPortChain,
				Specs: []string{
					util.IptablesMatchFlag,
					util.IptablesSetFlag,
					util.IptablesMatchSetFlag,
					hashedTargetSetName,
					util.IptablesDstFlag,
					util.IptablesJumpFlag,
					util.IptablesAccept,
				},
			}
			entries = append(entries, allow)
			continue
		}

		if !portRuleExists {
			entry := &iptm.IptEntry{
				Name:       targetSet,
				HashedName: hashedTargetSetName,
				Chain:      util.IptablesAzureIngressPortChain,
				Specs: []string{
					util.IptablesMatchFlag,
					util.IptablesSetFlag,
					util.IptablesMatchSetFlag,
					hashedTargetSetName,
					util.IptablesDstFlag,
					util.IptablesJumpFlag,
					util.IptablesAzureIngressFromChain,
				},
			}
			entries = append(entries, entry)
		} else {
			for _, protPortPair := range protPortPairSlice {
				entry := &iptm.IptEntry{
					Name:       targetSet,
					HashedName: hashedTargetSetName,
					Chain:      util.IptablesAzureIngressPortChain,
					Specs: []string{
						util.IptablesProtFlag,
						protPortPair.protocol,
						util.IptablesDstPortFlag,
						protPortPair.port,
						util.IptablesMatchFlag,
						util.IptablesSetFlag,
						util.IptablesMatchSetFlag,
						hashedTargetSetName,
						util.IptablesDstFlag,
						util.IptablesJumpFlag,
						util.IptablesAzureIngressFromChain,
					},
				}
				entries = append(entries, entry)
			}
		}

		if !fromRuleExists {
			entry := &iptm.IptEntry{
				Name:       targetSet,
				HashedName: hashedTargetSetName,
				Chain:      util.IptablesAzureIngressFromChain,
				Specs: []string{
					util.IptablesMatchFlag,
					util.IptablesSetFlag,
					util.IptablesMatchSetFlag,
					hashedTargetSetName,
					util.IptablesDstFlag,
					util.IptablesJumpFlag,
					util.IptablesAccept,
				},
			}
			entries = append(entries, entry)
			continue
		}

		if ipblock != nil {
			// Handle ipblock field of NetworkPolicyPeer
			if len(ipblock.Except) > 0 {
				for _, except := range ipblock.Except {
					entry := &iptm.IptEntry{
						Chain: util.IptablesAzureIngressFromChain,
						Specs: []string{
							util.IptablesMatchFlag,
							util.IptablesSetFlag,
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

			if len(ipblock.CIDR) > 0 {
				cidrEntry := &iptm.IptEntry{
					Chain: util.IptablesAzureIngressFromChain,
					Specs: []string{
						util.IptablesMatchFlag,
						util.IptablesSetFlag,
						util.IptablesMatchSetFlag,
						hashedTargetSetName,
						util.IptablesDstFlag,
						util.IptablesSFlag,
						ipblock.CIDR,
						util.IptablesJumpFlag,
						util.IptablesAccept,
					},
				}
				entries = append(entries, cidrEntry)
			}
		}

		// Handle PodSelector field of NetworkPolicyPeer.
		for _, podRuleSet := range PodNsRuleSets {
			hashedRuleSetName := util.GetHashedName(podRuleSet)
			entry := &iptm.IptEntry{
				Name:       podRuleSet,
				HashedName: hashedRuleSetName,
				Chain:      util.IptablesAzureIngressFromChain,
				Specs: []string{
					util.IptablesMatchFlag,
					util.IptablesSetFlag,
					util.IptablesMatchSetFlag,
					hashedRuleSetName,
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
			entries = append(entries, entry)
		}

		// Handle NamespaceSelector field of NetworkPolicyPeer
		for _, nsRuleSet := range nsRuleLists {
			hashedRuleSetName := util.GetHashedName(nsRuleSet)
			entry := &iptm.IptEntry{
				Name:       nsRuleSet,
				HashedName: hashedRuleSetName,
				Chain:      util.IptablesAzureIngressFromChain,
				Specs: []string{
					util.IptablesMatchFlag,
					util.IptablesSetFlag,
					util.IptablesMatchSetFlag,
					hashedRuleSetName,
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
			entries = append(entries, entry)
		}
	}

	return PodNsRuleSets, nsRuleLists, entries
}

func parseEgress(ns string, targetSets []string, rules []networkingv1.NetworkPolicyEgressRule) ([]string, []string, []*iptm.IptEntry) {
	var (
		portRuleExists    = false
		toRuleExists      = false
		isAppliedToNs     = false
		protPortPairSlice []*portsInfo
		PodNsRuleSets     []string // pod sets listed in Egress rules.
		nsRuleLists       []string // namespace sets listed in Egress rules
		entries           []*iptm.IptEntry
		ipblock           *networkingv1.IPBlock
	)

	if len(targetSets) == 0 {
		targetSets = append(targetSets, ns)
		isAppliedToNs = true
	}

	for _, rule := range rules {
		for _, portRule := range rule.Ports {
			protPortPairSlice = append(protPortPairSlice,
				&portsInfo{
					protocol: string(*portRule.Protocol),
					port:     fmt.Sprint(portRule.Port.IntVal),
				})

			portRuleExists = true
		}

		for _, toRule := range rule.To {
			if toRule.PodSelector != nil {
				if len(toRule.PodSelector.MatchLabels) == 0 {
					PodNsRuleSets = append(PodNsRuleSets, ns)
				}

				for podLabelKey, podLabelVal := range toRule.PodSelector.MatchLabels {
					PodNsRuleSets = append(PodNsRuleSets, util.KubeAllNamespacesFlag+"-"+podLabelKey+":"+podLabelVal)
				}
			}

			if toRule.NamespaceSelector != nil {
				if len(toRule.NamespaceSelector.MatchLabels) == 0 {
					nsRuleLists = append(nsRuleLists, util.KubeAllNamespacesFlag)
				}

				for nsLabelKey, nsLabelVal := range toRule.NamespaceSelector.MatchLabels {
					nsRuleLists = append(nsRuleLists, "ns-"+nsLabelKey+":"+nsLabelVal)
				}
			}

			if toRule.IPBlock != nil {
				ipblock = toRule.IPBlock
			}

			toRuleExists = true
		}
	}

	if isAppliedToNs {
		hashedTargetSetName := util.GetHashedName(ns)

		nsDrop := &iptm.IptEntry{
			Name:       ns,
			HashedName: hashedTargetSetName,
			Chain:      util.IptablesAzureTargetSetsChain,
			Specs: []string{
				util.IptablesMatchFlag,
				util.IptablesSetFlag,
				util.IptablesMatchSetFlag,
				hashedTargetSetName,
				util.IptablesSrcFlag,
				util.IptablesJumpFlag,
				util.IptablesDrop,
			},
		}
		entries = append(entries, nsDrop)
	}

	// Use hashed string for ipset name to avoid string length limit of ipset.
	for _, targetSet := range targetSets {
		hashedTargetSetName := util.GetHashedName(targetSet)

		if len(rules) == 0 {
			drop := &iptm.IptEntry{
				Name:       targetSet,
				HashedName: hashedTargetSetName,
				Chain:      util.IptablesAzureEgressPortChain,
				Specs: []string{
					util.IptablesMatchFlag,
					util.IptablesSetFlag,
					util.IptablesMatchSetFlag,
					hashedTargetSetName,
					util.IptablesSrcFlag,
					util.IptablesJumpFlag,
					util.IptablesDrop,
				},
			}
			entries = append(entries, drop)
			continue
		}

		if !portRuleExists && !toRuleExists {
			allow := &iptm.IptEntry{
				Name:       targetSet,
				HashedName: hashedTargetSetName,
				Chain:      util.IptablesAzureEgressPortChain,
				Specs: []string{
					util.IptablesMatchFlag,
					util.IptablesSetFlag,
					util.IptablesMatchSetFlag,
					hashedTargetSetName,
					util.IptablesSrcFlag,
					util.IptablesJumpFlag,
					util.IptablesAccept,
				},
			}
			entries = append(entries, allow)
			continue
		}

		if !portRuleExists {
			entry := &iptm.IptEntry{
				Name:       targetSet,
				HashedName: hashedTargetSetName,
				Chain:      util.IptablesAzureEgressPortChain,
				Specs: []string{
					util.IptablesMatchFlag,
					util.IptablesSetFlag,
					util.IptablesMatchSetFlag,
					hashedTargetSetName,
					util.IptablesDstFlag,
					util.IptablesJumpFlag,
					util.IptablesAzureEgressToChain,
				},
			}
			entries = append(entries, entry)
		} else {
			for _, protPortPair := range protPortPairSlice {
				entry := &iptm.IptEntry{
					Name:       targetSet,
					HashedName: hashedTargetSetName,
					Chain:      util.IptablesAzureEgressPortChain,
					Specs: []string{
						util.IptablesProtFlag,
						protPortPair.protocol,
						util.IptablesDstPortFlag,
						protPortPair.port,
						util.IptablesMatchFlag,
						util.IptablesSetFlag,
						util.IptablesMatchSetFlag,
						hashedTargetSetName,
						util.IptablesSrcFlag,
						util.IptablesJumpFlag,
						util.IptablesAzureEgressToChain,
					},
				}
				entries = append(entries, entry)
			}
		}

		if !toRuleExists {
			entry := &iptm.IptEntry{
				Name:       targetSet,
				HashedName: hashedTargetSetName,
				Chain:      util.IptablesAzureEgressToChain,
				Specs: []string{
					util.IptablesMatchFlag,
					util.IptablesSetFlag,
					util.IptablesMatchSetFlag,
					hashedTargetSetName,
					util.IptablesDstFlag,
					util.IptablesJumpFlag,
					util.IptablesAccept,
				},
			}
			entries = append(entries, entry)
			continue
		}

		if ipblock != nil {
			// Handle ipblock field of NetworkPolicyPeer
			if len(ipblock.Except) > 0 {
				for _, except := range ipblock.Except {
					entry := &iptm.IptEntry{
						Chain: util.IptablesAzureEgressToChain,
						Specs: []string{
							util.IptablesMatchFlag,
							util.IptablesSetFlag,
							util.IptablesMatchSetFlag,
							hashedTargetSetName,
							util.IptablesSrcFlag,
							util.IptablesDFlag,
							except,
							util.IptablesJumpFlag,
							util.IptablesDrop,
						},
					}
					entries = append(entries, entry)
				}
			}

			if len(ipblock.CIDR) > 0 {
				cidrEntry := &iptm.IptEntry{
					Chain: util.IptablesAzureEgressToChain,
					Specs: []string{
						util.IptablesMatchFlag,
						util.IptablesSetFlag,
						util.IptablesMatchSetFlag,
						hashedTargetSetName,
						util.IptablesSrcFlag,
						util.IptablesDFlag,
						ipblock.CIDR,
						util.IptablesJumpFlag,
						util.IptablesAccept,
					},
				}
				entries = append(entries, cidrEntry)
			}
		}

		// Handle PodSelector field of NetworkPolicyPeer.
		for _, podRuleSet := range PodNsRuleSets {
			hashedRuleSetName := util.GetHashedName(podRuleSet)
			entry := &iptm.IptEntry{
				Name:       podRuleSet,
				HashedName: hashedRuleSetName,
				Chain:      util.IptablesAzureEgressToChain,
				Specs: []string{
					util.IptablesMatchFlag,
					util.IptablesSetFlag,
					util.IptablesMatchSetFlag,
					hashedTargetSetName,
					util.IptablesSrcFlag,
					util.IptablesMatchFlag,
					util.IptablesSetFlag,
					util.IptablesMatchSetFlag,
					hashedRuleSetName,
					util.IptablesDstFlag,
					util.IptablesJumpFlag,
					util.IptablesAccept,
				},
			}
			entries = append(entries, entry)
		}

		// Handle NamespaceSelector field of NetworkPolicyPeer
		for _, nsRuleSet := range nsRuleLists {
			hashedRuleSetName := util.GetHashedName(nsRuleSet)
			entry := &iptm.IptEntry{
				Name:       nsRuleSet,
				HashedName: hashedRuleSetName,
				Chain:      util.IptablesAzureEgressToChain,
				Specs: []string{
					util.IptablesMatchFlag,
					util.IptablesSetFlag,
					util.IptablesMatchSetFlag,
					hashedTargetSetName,
					util.IptablesSrcFlag,
					util.IptablesMatchFlag,
					util.IptablesSetFlag,
					util.IptablesMatchSetFlag,
					hashedRuleSetName,
					util.IptablesDstFlag,
					util.IptablesJumpFlag,
					util.IptablesAccept,
				},
			}
			entries = append(entries, entry)
		}
	}

	return PodNsRuleSets, nsRuleLists, entries
}

// Drop all non-whitelisted packets.
func getDefaultDropEntries(targetSets []string) []*iptm.IptEntry {
	var entries []*iptm.IptEntry

	for _, targetSet := range targetSets {
		hashedTargetSetName := util.GetHashedName(targetSet)
		entry := &iptm.IptEntry{
			Name:       targetSet,
			HashedName: hashedTargetSetName,
			Chain:      util.IptablesAzureTargetSetsChain,
			Specs: []string{
				util.IptablesMatchFlag,
				util.IptablesSetFlag,
				util.IptablesMatchSetFlag,
				hashedTargetSetName,
				util.IptablesSrcFlag,
				util.IptablesJumpFlag,
				util.IptablesDrop,
			},
		}
		entries = append(entries, entry)

		entry = &iptm.IptEntry{
			Name:       targetSet,
			HashedName: hashedTargetSetName,
			Chain:      util.IptablesAzureTargetSetsChain,
			Specs: []string{
				util.IptablesMatchFlag,
				util.IptablesSetFlag,
				util.IptablesMatchSetFlag,
				hashedTargetSetName,
				util.IptablesDstFlag,
				util.IptablesJumpFlag,
				util.IptablesDrop,
			},
		}
		entries = append(entries, entry)
	}

	return entries
}

// ParsePolicy parses network policy.
func parsePolicy(npObj *networkingv1.NetworkPolicy) ([]string, []string, []*iptm.IptEntry) {
	var (
		resultPodSets []string
		resultNsLists []string
		affectedSets  []string
		entries       []*iptm.IptEntry
	)

	// Get affected pods.
	npNs, selector := npObj.ObjectMeta.Namespace, npObj.Spec.PodSelector.MatchLabels
	for podLabelKey, podLabelVal := range selector {
		affectedSet := util.KubeAllNamespacesFlag + "-" + podLabelKey + ":" + podLabelVal
		affectedSets = append(affectedSets, affectedSet)
	}

	if len(npObj.Spec.PolicyTypes) == 0 {
		ingressPodSets, ingressNsSets, ingressEntries := parseIngress(npNs, affectedSets, npObj.Spec.Ingress)
		resultPodSets = append(resultPodSets, ingressPodSets...)
		resultNsLists = append(resultNsLists, ingressNsSets...)
		entries = append(entries, ingressEntries...)

		egressPodSets, egressNsSets, egressEntries := parseEgress(npNs, affectedSets, npObj.Spec.Egress)
		resultPodSets = append(resultPodSets, egressPodSets...)
		resultNsLists = append(resultNsLists, egressNsSets...)
		entries = append(entries, egressEntries...)

		entries = append(entries, getDefaultDropEntries(affectedSets)...)

		resultPodSets = append(resultPodSets, affectedSets...)

		return util.UniqueStrSlice(resultPodSets), util.UniqueStrSlice(resultNsLists), entries
	}

	for _, ptype := range npObj.Spec.PolicyTypes {
		if ptype == networkingv1.PolicyTypeIngress {
			ingressPodSets, ingressNsSets, ingressEntries := parseIngress(npNs, affectedSets, npObj.Spec.Ingress)
			resultPodSets = append(resultPodSets, ingressPodSets...)
			resultNsLists = append(resultNsLists, ingressNsSets...)
			entries = append(entries, ingressEntries...)
		}

		if ptype == networkingv1.PolicyTypeEgress {
			egressPodSets, egressNsSets, egressEntries := parseEgress(npNs, affectedSets, npObj.Spec.Egress)
			resultPodSets = append(resultPodSets, egressPodSets...)
			resultNsLists = append(resultNsLists, egressNsSets...)
			entries = append(entries, egressEntries...)
		}

		entries = append(entries, getDefaultDropEntries(affectedSets)...)
	}

	resultPodSets = append(resultPodSets, affectedSets...)
	resultPodSets = append(resultPodSets, npNs)

	return util.UniqueStrSlice(resultPodSets), util.UniqueStrSlice(resultNsLists), entries
}
