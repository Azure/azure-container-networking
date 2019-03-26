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

type portsInfo struct {
	protocol string
	port     string
}

func appendAndClearSets(podNsRuleSets *[]string, nsRuleLists *[]string, policyRuleSets *[]string, policyRuleLists *[]string) {
	*policyRuleSets = append(*policyRuleSets, *podNsRuleSets...)
	*policyRuleLists = append(*policyRuleLists, *nsRuleLists...)
	podNsRuleSets, nsRuleLists = nil, nil
}

func parseIngress(ns string, targetSets []string, rules []networkingv1.NetworkPolicyIngressRule) ([]string, []string, []*iptm.IptEntry) {
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

	if len(targetSets) == 0 {
		targetSets = append(targetSets, ns)
		isAppliedToNs = true
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

		// allow kube-system
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

		for _, rule := range rules {
			for _, portRule := range rule.Ports {
				protPortPairSlice = append(protPortPairSlice,
					&portsInfo{
						protocol: string(*portRule.Protocol),
						port:     fmt.Sprint(portRule.Port.IntVal),
					})

				portRuleExists = true
			}

			if rule.From != nil {
				for _, fromRule := range rule.From {
					if fromRule.PodSelector != nil {
						fromRuleExists = true
					}
					if fromRule.NamespaceSelector != nil {
						fromRuleExists = true
					}
					if fromRule.IPBlock != nil {
						fromRuleExists = true
					}
				}
			}
		}

		for _, rule := range rules {
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
						util.IptablesAzureIngressFromNsChain,
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
							util.IptablesAzureIngressFromNsChain,
						},
					}
					entries = append(entries, entry)
				}
			}

			if !fromRuleExists {
				entry := &iptm.IptEntry{
					Name:       targetSet,
					HashedName: hashedTargetSetName,
					Chain:      util.IptablesAzureIngressFromNsChain,
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

			for _, fromRule := range rule.From {
				// Handle IPBlock field of NetworkPolicyPeer
				if fromRule.IPBlock != nil {
					if len(fromRule.IPBlock.CIDR) > 0 {
						cidrEntry := &iptm.IptEntry{
							Chain: util.IptablesAzureIngressFromNsChain,
							Specs: []string{
								util.IptablesMatchFlag,
								util.IptablesSetFlag,
								util.IptablesMatchSetFlag,
								hashedTargetSetName,
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
				}

				if fromRule.PodSelector == nil && fromRule.NamespaceSelector == nil {
					continue
				}

				// Allow traffic from namespaceSelector
				if fromRule.PodSelector == nil && fromRule.NamespaceSelector != nil {
					// allow traffic from all namespaces
					if len(fromRule.NamespaceSelector.MatchLabels) == 0 {
						nsRuleLists = append(nsRuleLists, util.KubeAllNamespacesFlag)
					}

					for nsLabelKey, nsLabelVal := range fromRule.NamespaceSelector.MatchLabels {
						nsRuleLists = append(nsRuleLists, util.GetNsIpsetName(nsLabelKey, nsLabelVal))
					}

					for _, nsRuleSet := range nsRuleLists {
						hashedNsRuleSetName := util.GetHashedName(nsRuleSet)
						entry := &iptm.IptEntry{
							Name:       nsRuleSet,
							HashedName: hashedNsRuleSetName,
							Chain:      util.IptablesAzureIngressFromNsChain,
							Specs: []string{
								util.IptablesMatchFlag,
								util.IptablesSetFlag,
								util.IptablesMatchSetFlag,
								hashedNsRuleSetName,
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
					appendAndClearSets(&podNsRuleSets, &nsRuleLists, &policyRuleSets, &policyRuleLists)
					continue
				}

				// Allow traffic from podSelector
				if fromRule.PodSelector != nil && fromRule.NamespaceSelector == nil {
					// allow traffic from the same namespace
					if len(fromRule.PodSelector.MatchLabels) == 0 {
						podNsRuleSets = append(podNsRuleSets, ns)
					}

					for podLabelKey, podLabelVal := range fromRule.PodSelector.MatchLabels {
						podNsRuleSets = append(podNsRuleSets, util.KubeAllNamespacesFlag+"-"+podLabelKey+":"+podLabelVal)
					}

					// Handle PodSelector field of NetworkPolicyPeer.
					for _, podRuleSet := range podNsRuleSets {
						hashedPodRuleSetName := util.GetHashedName(podRuleSet)
						nsEntry := &iptm.IptEntry{
							Name:       podRuleSet,
							HashedName: hashedPodRuleSetName,
							Chain:      util.IptablesAzureIngressFromNsChain,
							Specs: []string{
								util.IptablesMatchFlag,
								util.IptablesSetFlag,
								util.IptablesMatchSetFlag,
								hashedTargetSetName,
								util.IptablesDstFlag,
								util.IptablesJumpFlag,
								util.IptablesAzureIngressFromPodChain,
							},
						}
						entries = append(entries, nsEntry)

						podEntry := &iptm.IptEntry{
							Name:       podRuleSet,
							HashedName: hashedPodRuleSetName,
							Chain:      util.IptablesAzureIngressFromPodChain,
							Specs: []string{
								util.IptablesMatchFlag,
								util.IptablesSetFlag,
								util.IptablesMatchSetFlag,
								hashedPodRuleSetName,
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
						entries = append(entries, podEntry)
					}
					appendAndClearSets(&podNsRuleSets, &nsRuleLists, &policyRuleSets, &policyRuleLists)
					continue
				}

				// Allow traffic from podSelector intersects namespaceSelector
				// This is only supported in kubernetes version >= 1.11
				if util.IsNewNwPolicyVerFlag {
					// allow traffic from all namespaces
					if len(fromRule.NamespaceSelector.MatchLabels) == 0 {
						nsRuleLists = append(nsRuleLists, util.KubeAllNamespacesFlag)
					}

					for nsLabelKey, nsLabelVal := range fromRule.NamespaceSelector.MatchLabels {
						nsRuleLists = append(nsRuleLists, util.GetNsIpsetName(nsLabelKey, nsLabelVal))
					}

					for _, nsRuleSet := range nsRuleLists {
						hashedNsRuleSetName := util.GetHashedName(nsRuleSet)
						entry := &iptm.IptEntry{
							Name:       nsRuleSet,
							HashedName: hashedNsRuleSetName,
							Chain:      util.IptablesAzureIngressFromNsChain,
							Specs: []string{
								util.IptablesMatchFlag,
								util.IptablesSetFlag,
								util.IptablesMatchSetFlag,
								hashedNsRuleSetName,
								util.IptablesSrcFlag,
								util.IptablesMatchFlag,
								util.IptablesSetFlag,
								util.IptablesMatchSetFlag,
								hashedTargetSetName,
								util.IptablesDstFlag,
								util.IptablesJumpFlag,
								util.IptablesAzureIngressFromPodChain,
							},
						}
						entries = append(entries, entry)
					}

					// allow traffic from the same namespace
					if len(fromRule.PodSelector.MatchLabels) == 0 {
						podNsRuleSets = append(podNsRuleSets, ns)
					}

					for podLabelKey, podLabelVal := range fromRule.PodSelector.MatchLabels {
						podNsRuleSets = append(podNsRuleSets, util.KubeAllNamespacesFlag+"-"+podLabelKey+":"+podLabelVal)
					}

					// Handle PodSelector field of NetworkPolicyPeer.
					for _, podRuleSet := range podNsRuleSets {
						hashedPodRuleSetName := util.GetHashedName(podRuleSet)
						entry := &iptm.IptEntry{
							Name:       podRuleSet,
							HashedName: hashedPodRuleSetName,
							Chain:      util.IptablesAzureIngressFromPodChain,
							Specs: []string{
								util.IptablesMatchFlag,
								util.IptablesSetFlag,
								util.IptablesMatchSetFlag,
								hashedPodRuleSetName,
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
					appendAndClearSets(&podNsRuleSets, &nsRuleLists, &policyRuleSets, &policyRuleLists)
				}
			}
		}
	}

	log.Printf("finished parsing ingress rule")
	return policyRuleSets, policyRuleLists, entries
}

func parseEgress(ns string, targetSets []string, rules []networkingv1.NetworkPolicyEgressRule) ([]string, []string, []*iptm.IptEntry) {
	var (
		portRuleExists    = false
		toRuleExists      = false
		isAppliedToNs     = false
		protPortPairSlice []*portsInfo
		podNsRuleSets     []string // pod sets listed in one ingress rules.
		nsRuleLists       []string // namespace sets listed in one ingress rule
		policyRuleSets    []string // policy-wise pod sets
		policyRuleLists   []string // policy-wise namespace sets
		entries           []*iptm.IptEntry
	)

	if len(targetSets) == 0 {
		targetSets = append(targetSets, ns)
		isAppliedToNs = true
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

		// allow kube-system
		hashedKubeSystemSet := util.GetHashedName(util.KubeSystemFlag)
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

		for _, rule := range rules {
			for _, portRule := range rule.Ports {
				protPortPairSlice = append(protPortPairSlice,
					&portsInfo{
						protocol: string(*portRule.Protocol),
						port:     fmt.Sprint(portRule.Port.IntVal),
					})

				portRuleExists = true
			}

			if rule.To != nil {
				for _, toRule := range rule.To {
					if toRule.PodSelector != nil {
						toRuleExists = true
					}
					if toRule.NamespaceSelector != nil {
						toRuleExists = true
					}
					if toRule.IPBlock != nil {
						toRuleExists = true
					}
				}
			}
		}

		for _, rule := range rules {
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
						util.IptablesSrcFlag,
						util.IptablesJumpFlag,
						util.IptablesAzureEgressToNsChain,
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
							util.IptablesAzureEgressToNsChain,
						},
					}
					entries = append(entries, entry)
				}
			}

			if !toRuleExists {
				entry := &iptm.IptEntry{
					Name:       targetSet,
					HashedName: hashedTargetSetName,
					Chain:      util.IptablesAzureEgressToNsChain,
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
				entries = append(entries, entry)
				continue
			}

			for _, toRule := range rule.To {
				// Handle IPBlock field of NetworkPolicyPeer
				if toRule.IPBlock != nil {
					if len(toRule.IPBlock.CIDR) > 0 {
						cidrEntry := &iptm.IptEntry{
							Chain: util.IptablesAzureEgressToNsChain,
							Specs: []string{
								util.IptablesMatchFlag,
								util.IptablesSetFlag,
								util.IptablesMatchSetFlag,
								hashedTargetSetName,
								util.IptablesSrcFlag,
								util.IptablesDFlag,
								toRule.IPBlock.CIDR,
								util.IptablesJumpFlag,
								util.IptablesAccept,
							},
						}
						entries = append(entries, cidrEntry)
					}

					if len(toRule.IPBlock.Except) > 0 {
						for _, except := range toRule.IPBlock.Except {
							entry := &iptm.IptEntry{
								Chain: util.IptablesAzureEgressToNsChain,
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
				}

				if toRule.PodSelector == nil && toRule.NamespaceSelector == nil {
					continue
				}

				// Allow traffic from namespaceSelector
				if toRule.PodSelector == nil && toRule.NamespaceSelector != nil {
					// allow traffic from all namespaces
					if len(toRule.NamespaceSelector.MatchLabels) == 0 {
						nsRuleLists = append(nsRuleLists, util.KubeAllNamespacesFlag)
					}

					for nsLabelKey, nsLabelVal := range toRule.NamespaceSelector.MatchLabels {
						nsRuleLists = append(nsRuleLists, util.GetNsIpsetName(nsLabelKey, nsLabelVal))
					}

					for _, nsRuleSet := range nsRuleLists {
						hashedNsRuleSetName := util.GetHashedName(nsRuleSet)
						entry := &iptm.IptEntry{
							Name:       nsRuleSet,
							HashedName: hashedNsRuleSetName,
							Chain:      util.IptablesAzureEgressToNsChain,
							Specs: []string{
								util.IptablesMatchFlag,
								util.IptablesSetFlag,
								util.IptablesMatchSetFlag,
								hashedTargetSetName,
								util.IptablesSrcFlag,
								util.IptablesMatchFlag,
								util.IptablesSetFlag,
								util.IptablesMatchSetFlag,
								hashedNsRuleSetName,
								util.IptablesDstFlag,
								util.IptablesJumpFlag,
								util.IptablesAccept,
							},
						}
						entries = append(entries, entry)
					}
					appendAndClearSets(&podNsRuleSets, &nsRuleLists, &policyRuleSets, &policyRuleLists)
					continue
				}

				// Allow traffic from podSelector
				if toRule.PodSelector != nil && toRule.NamespaceSelector == nil {
					// allow traffic from the same namespace
					if len(toRule.PodSelector.MatchLabels) == 0 {
						podNsRuleSets = append(podNsRuleSets, ns)
					}

					for podLabelKey, podLabelVal := range toRule.PodSelector.MatchLabels {
						podNsRuleSets = append(podNsRuleSets, util.KubeAllNamespacesFlag+"-"+podLabelKey+":"+podLabelVal)
					}

					// Handle PodSelector field of NetworkPolicyPeer.
					for _, podRuleSet := range podNsRuleSets {
						hashedPodRuleSetName := util.GetHashedName(podRuleSet)
						nsEntry := &iptm.IptEntry{
							Name:       podRuleSet,
							HashedName: hashedPodRuleSetName,
							Chain:      util.IptablesAzureEgressToNsChain,
							Specs: []string{
								util.IptablesMatchFlag,
								util.IptablesSetFlag,
								util.IptablesMatchSetFlag,
								hashedTargetSetName,
								util.IptablesSrcFlag,
								util.IptablesJumpFlag,
								util.IptablesAzureEgressToPodChain,
							},
						}
						entries = append(entries, nsEntry)

						podEntry := &iptm.IptEntry{
							Name:       podRuleSet,
							HashedName: hashedPodRuleSetName,
							Chain:      util.IptablesAzureEgressToPodChain,
							Specs: []string{
								util.IptablesMatchFlag,
								util.IptablesSetFlag,
								util.IptablesMatchSetFlag,
								hashedTargetSetName,
								util.IptablesSrcFlag,
								util.IptablesMatchFlag,
								util.IptablesSetFlag,
								util.IptablesMatchSetFlag,
								hashedPodRuleSetName,
								util.IptablesDstFlag,
								util.IptablesJumpFlag,
								util.IptablesAccept,
							},
						}
						entries = append(entries, podEntry)
					}
					appendAndClearSets(&podNsRuleSets, &nsRuleLists, &policyRuleSets, &policyRuleLists)
					continue
				}

				// Allow traffic from podSelector intersects namespaceSelector
				// This is only supported in kubernetes version >= 1.11
				if util.IsNewNwPolicyVerFlag {
					log.Printf("Kubernetes version > 1.11, parsing podSelector AND namespaceSelector")
					// allow traffic from all namespaces
					if len(toRule.NamespaceSelector.MatchLabels) == 0 {
						nsRuleLists = append(nsRuleLists, util.KubeAllNamespacesFlag)
					}

					for nsLabelKey, nsLabelVal := range toRule.NamespaceSelector.MatchLabels {
						nsRuleLists = append(nsRuleLists, util.GetNsIpsetName(nsLabelKey, nsLabelVal))
					}

					for _, nsRuleSet := range nsRuleLists {
						hashedNsRuleSetName := util.GetHashedName(nsRuleSet)
						entry := &iptm.IptEntry{
							Name:       nsRuleSet,
							HashedName: hashedNsRuleSetName,
							Chain:      util.IptablesAzureEgressToNsChain,
							Specs: []string{
								util.IptablesMatchFlag,
								util.IptablesSetFlag,
								util.IptablesMatchSetFlag,
								hashedTargetSetName,
								util.IptablesSrcFlag,
								util.IptablesMatchFlag,
								util.IptablesSetFlag,
								util.IptablesMatchSetFlag,
								hashedNsRuleSetName,
								util.IptablesDstFlag,
								util.IptablesJumpFlag,
								util.IptablesAzureEgressToPodChain,
							},
						}
						entries = append(entries, entry)
					}

					// allow traffic from the same namespace
					if len(toRule.PodSelector.MatchLabels) == 0 {
						podNsRuleSets = append(podNsRuleSets, ns)
					}

					for podLabelKey, podLabelVal := range toRule.PodSelector.MatchLabels {
						podNsRuleSets = append(podNsRuleSets, util.KubeAllNamespacesFlag+"-"+podLabelKey+":"+podLabelVal)
					}

					// Handle PodSelector field of NetworkPolicyPeer.
					for _, podRuleSet := range podNsRuleSets {
						hashedPodRuleSetName := util.GetHashedName(podRuleSet)
						entry := &iptm.IptEntry{
							Name:       podRuleSet,
							HashedName: hashedPodRuleSetName,
							Chain:      util.IptablesAzureEgressToPodChain,
							Specs: []string{
								util.IptablesMatchFlag,
								util.IptablesSetFlag,
								util.IptablesMatchSetFlag,
								hashedTargetSetName,
								util.IptablesSrcFlag,
								util.IptablesMatchFlag,
								util.IptablesSetFlag,
								util.IptablesMatchSetFlag,
								hashedPodRuleSetName,
								util.IptablesDstFlag,
								util.IptablesJumpFlag,
								util.IptablesAccept,
							},
						}
						entries = append(entries, entry)
					}
					appendAndClearSets(&podNsRuleSets, &nsRuleLists, &policyRuleSets, &policyRuleLists)
				}
			}
		}
	}

	log.Printf("finished parsing ingress rule")
	return policyRuleSets, policyRuleLists, entries
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

	if len(npObj.Spec.Ingress) > 0 || len(npObj.Spec.Egress) > 0 {
		entries = append(entries, getAllowKubeSystemEntries(npNs, affectedSets)...)
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
