// Copyright 2018 Microsoft. All rights reserved.
// MIT License
package npm

import (
	"fmt"

	"github.com/Microsoft/hcsshim/hcn"
	"github.com/kalebmorris/azure-container-networking/log"
	"github.com/kalebmorris/azure-container-networking/npm/util"
	networkingv1 "k8s.io/api/networking/v1"
)

const (
	ingressPort    uint16 = 1000
	ingressFromNs  uint16 = 1001
	ingressFromPod uint16 = 1002
	egressPort     uint16 = 1003
	egressFromNs   uint16 = 1004
	egressFromPod  uint16 = 1005
	defaults       uint16 = 1006
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

func parseIngress(ns string, targetSets []string, rules []networkingv1.NetworkPolicyIngressRule) ([]string, []string, []*hcn.AclPolicySetting) {
	var (
		portRuleExists    = false
		fromRuleExists    = false
		isAppliedToNs     = false
		protPortPairSlice []*portsInfo
		podNsRuleSets     []string // pod sets listed in one ingress rules.
		nsRuleLists       []string // namespace sets listed in one ingress rule
		policyRuleSets    []string // policy-wise pod sets
		policyRuleLists   []string // policy-wise namespace sets
		policies          []*hcn.AclPolicySetting
	)

	if len(targetSets) == 0 { // Select all
		targetSets = append(targetSets, ns)
		isAppliedToNs = true
	}

	if isAppliedToNs {
		hashedTargetSetName := util.GetHashedName(ns)

		nsInDrop := &hcn.AclPolicySetting{
			Protocols: "6",
			Action:    hcn.ActionTypeBlock,
			Direction: hcn.DirectionTypeIn,
			LocalTags: hashedTargetSetName,
			RuleType:  hcn.RuleTypeSwitch,
			Priority:  defaults,
		}
		policies = append(policies, nsInDrop)
	}

	for _, targetSet := range targetSets {
		log.Printf("Parsing ACL policies for label %s", targetSet)

		hashedTargetSetName := util.GetHashedName(targetSet)

		if len(rules) == 0 {
			drop := &hcn.AclPolicySetting{
				Protocols: "6",
				Action:    hcn.ActionTypeBlock,
				Direction: hcn.DirectionTypeIn,
				LocalTags: hashedTargetSetName,
				RuleType:  hcn.RuleTypeSwitch,
				Priority:  ingressPort,
			}
			policies = append(policies, drop)
			continue
		}

		// allow kube-system
		hashedKubeSystemSet := util.GetHashedName(util.KubeSystemFlag)
		allowKubeSystemIngress := &hcn.AclPolicySetting{
			Protocols:  "6",
			Action:     hcn.ActionTypeAllow,
			Direction:  hcn.DirectionTypeIn,
			LocalTags:  hashedTargetSetName,
			RemoteTags: hashedKubeSystemSet,
			RuleType:   hcn.RuleTypeSwitch,
			Priority:   ingressPort,
		}
		policies = append(policies, allowKubeSystemIngress)

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
				allow := &hcn.AclPolicySetting{
					Protocols: "6",
					Action:    hcn.ActionTypeAllow,
					Direction: hcn.DirectionTypeIn,
					LocalTags: hashedTargetSetName,
					RuleType:  hcn.RuleTypeSwitch,
					Priority:  ingressPort,
				}
				policies = append(policies, allow)
				continue
			}

			if !fromRuleExists {
				policy := &hcn.AclPolicySetting{
					Protocols: "6",
					Action:    hcn.ActionTypeAllow,
					Direction: hcn.DirectionTypeIn,
					LocalTags: hashedTargetSetName,
					RuleType:  hcn.RuleTypeSwitch,
					Priority:  ingressFromNs,
				}
				policies = append(policies, policy)
				continue
			}

			for _, fromRule := range rule.From {
				// Handle IPBlock field of NetworkPolicyPeer
				if fromRule.IPBlock != nil {
					if len(fromRule.IPBlock.CIDR) > 0 {
						cidrPolicy := &hcn.AclPolicySetting{
							Protocols:       "6",
							Action:          hcn.ActionTypeAllow,
							Direction:       hcn.DirectionTypeIn,
							LocalTags:       hashedTargetSetName,
							RemoteAddresses: fromRule.IPBlock.CIDR,
							RuleType:        hcn.RuleTypeSwitch,
							Priority:        ingressFromNs,
						}
						policies = append(policies, cidrPolicy)
					}

					if len(fromRule.IPBlock.Except) > 0 {
						log.Errorf("Error: except blocks currently unsupported.")
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
						policy := &hcn.AclPolicySetting{
							Protocols:  "6",
							Action:     hcn.ActionTypeAllow,
							Direction:  hcn.DirectionTypeIn,
							LocalTags:  hashedTargetSetName,
							RemoteTags: hashedNsRuleSetName,
							RuleType:   hcn.RuleTypeSwitch,
							Priority:   ingressFromNs,
						}
						policies = append(policies, policy)
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
						podPolicy := &hcn.AclPolicySetting{
							Protocols:  "6",
							Action:     hcn.ActionTypeAllow,
							Direction:  hcn.DirectionTypeIn,
							LocalTags:  hashedTargetSetName,
							RemoteTags: hashedPodRuleSetName,
							RuleType:   hcn.RuleTypeSwitch,
							Priority:   ingressFromPod,
						}
						policies = append(policies, podPolicy)
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
						policy := &hcn.AclPolicySetting{
							Protocols:  "6",
							Action:     hcn.ActionTypeAllow,
							Direction:  hcn.DirectionTypeIn,
							LocalTags:  hashedTargetSetName,
							RemoteTags: hashedPodRuleSetName,
							RuleType:   hcn.RuleTypeSwitch,
							Priority:   ingressFromPod,
						}
						policies = append(policies, policy)
					}
					appendAndClearSets(&podNsRuleSets, &nsRuleLists, &policyRuleSets, &policyRuleLists)
				}
			}
		}
	}

	log.Printf("finished parsing ingress rule")
	return policyRuleSets, policyRuleLists, policies
}

func parseEgress(ns string, targetSets []string, rules []networkingv1.NetworkPolicyEgressRule) ([]string, []string, []*hcn.AclPolicySetting) {
	var (
		portRuleExists    = false
		toRuleExists      = false
		isAppliedToNs     = false
		protPortPairSlice []*portsInfo
		podNsRuleSets     []string // pod sets listed in one egress rules.
		nsRuleLists       []string // namespace sets listed in one egress rule
		policyRuleSets    []string // policy-wise pod sets
		policyRuleLists   []string // policy-wise namespace sets
		policies          []*hcn.AclPolicySetting
	)

	if len(targetSets) == 0 { // Select all
		targetSets = append(targetSets, ns)
		isAppliedToNs = true
	}

	if isAppliedToNs {
		hashedTargetSetName := util.GetHashedName(ns)

		nsOutDrop := &hcn.AclPolicySetting{
			Protocols: "6",
			Action:    hcn.ActionTypeBlock,
			Direction: hcn.DirectionTypeOut,
			LocalTags: hashedTargetSetName,
			RuleType:  hcn.RuleTypeSwitch,
			Priority:  defaults,
		}
		policies = append(policies, nsOutDrop)
	}

	for _, targetSet := range targetSets {
		log.Printf("Parsing ACL policies for label %s", targetSet)

		hashedTargetSetName := util.GetHashedName(targetSet)

		if len(rules) == 0 {
			drop := &hcn.AclPolicySetting{
				Protocols: "6",
				Action:    hcn.ActionTypeBlock,
				Direction: hcn.DirectionTypeOut,
				LocalTags: hashedTargetSetName,
				RuleType:  hcn.RuleTypeSwitch,
				Priority:  egressPort,
			}
			policies = append(policies, drop)
			continue
		}

		// allow kube-system
		hashedKubeSystemSet := util.GetHashedName(util.KubeSystemFlag)
		allowKubeSystemEgress := &hcn.AclPolicySetting{
			Protocols:  "6",
			Action:     hcn.ActionTypeAllow,
			Direction:  hcn.DirectionTypeOut,
			LocalTags:  hashedTargetSetName,
			RemoteTags: hashedKubeSystemSet,
			RuleType:   hcn.RuleTypeSwitch,
			Priority:   egressPort,
		}
		policies = append(policies, allowKubeSystemEgress)

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
				allow := &hcn.AclPolicySetting{
					Protocols: "6",
					Action:    hcn.ActionTypeAllow,
					Direction: hcn.DirectionTypeOut,
					LocalTags: hashedTargetSetName,
					RuleType:  hcn.RuleTypeSwitch,
					Priority:  egressPort,
				}
				policies = append(policies, allow)
				continue
			}

			if !toRuleExists {
				policy := &hcn.AclPolicySetting{
					Protocols: "6",
					Action:    hcn.ActionTypeAllow,
					Direction: hcn.DirectionTypeOut,
					LocalTags: hashedTargetSetName,
					RuleType:  hcn.RuleTypeSwitch,
					Priority:  egressFromNs,
				}
				policies = append(policies, policy)
				continue
			}

			for _, toRule := range rule.To {
				// Handle IPBlock field of NetworkPolicyPeer
				if toRule.IPBlock != nil {
					if len(toRule.IPBlock.CIDR) > 0 {
						cidrPolicy := &hcn.AclPolicySetting{
							Protocols:       "6",
							Action:          hcn.ActionTypeAllow,
							Direction:       hcn.DirectionTypeOut,
							LocalTags:       hashedTargetSetName,
							RemoteAddresses: toRule.IPBlock.CIDR,
							RuleType:        hcn.RuleTypeSwitch,
							Priority:        egressFromNs,
						}
						policies = append(policies, cidrPolicy)
					}

					if len(toRule.IPBlock.Except) > 0 {
						log.Errorf("Error: except blocks currently unsupported.")
					}
				}

				if toRule.PodSelector == nil && toRule.NamespaceSelector == nil {
					continue
				}

				// Allow traffic to namespaceSelector
				if toRule.PodSelector == nil && toRule.NamespaceSelector != nil {
					// allow traffic to all namespaces
					if len(toRule.NamespaceSelector.MatchLabels) == 0 {
						nsRuleLists = append(nsRuleLists, util.KubeAllNamespacesFlag)
					}

					for nsLabelKey, nsLabelVal := range toRule.NamespaceSelector.MatchLabels {
						nsRuleLists = append(nsRuleLists, util.GetNsIpsetName(nsLabelKey, nsLabelVal))
					}

					for _, nsRuleSet := range nsRuleLists {
						hashedNsRuleSetName := util.GetHashedName(nsRuleSet)
						policy := &hcn.AclPolicySetting{
							Protocols:  "6",
							Action:     hcn.ActionTypeAllow,
							Direction:  hcn.DirectionTypeOut,
							LocalTags:  hashedTargetSetName,
							RemoteTags: hashedNsRuleSetName,
							RuleType:   hcn.RuleTypeSwitch,
							Priority:   egressFromNs,
						}
						policies = append(policies, policy)
					}
					appendAndClearSets(&podNsRuleSets, &nsRuleLists, &policyRuleSets, &policyRuleLists)
					continue
				}

				// Allow traffic to podSelector
				if toRule.PodSelector != nil && toRule.NamespaceSelector == nil {
					// allow traffic to the same namespace
					if len(toRule.PodSelector.MatchLabels) == 0 {
						podNsRuleSets = append(podNsRuleSets, ns)
					}

					for podLabelKey, podLabelVal := range toRule.PodSelector.MatchLabels {
						podNsRuleSets = append(podNsRuleSets, util.KubeAllNamespacesFlag+"-"+podLabelKey+":"+podLabelVal)
					}

					// Handle PodSelector field of NetworkPolicyPeer.
					for _, podRuleSet := range podNsRuleSets {
						hashedPodRuleSetName := util.GetHashedName(podRuleSet)
						podPolicy := &hcn.AclPolicySetting{
							Protocols:  "6",
							Action:     hcn.ActionTypeAllow,
							Direction:  hcn.DirectionTypeOut,
							LocalTags:  hashedTargetSetName,
							RemoteTags: hashedPodRuleSetName,
							RuleType:   hcn.RuleTypeSwitch,
							Priority:   egressFromPod,
						}
						policies = append(policies, podPolicy)
					}
					appendAndClearSets(&podNsRuleSets, &nsRuleLists, &policyRuleSets, &policyRuleLists)
					continue
				}

				// Allow traffic to podSelector intersects namespaceSelector
				// This is only supported in kubernetes version >= 1.11
				if util.IsNewNwPolicyVerFlag {
					// allow traffic to all namespaces
					if len(toRule.NamespaceSelector.MatchLabels) == 0 {
						nsRuleLists = append(nsRuleLists, util.KubeAllNamespacesFlag)
					}

					for nsLabelKey, nsLabelVal := range toRule.NamespaceSelector.MatchLabels {
						nsRuleLists = append(nsRuleLists, util.GetNsIpsetName(nsLabelKey, nsLabelVal))
					}

					// allow traffic to the same namespace
					if len(toRule.PodSelector.MatchLabels) == 0 {
						podNsRuleSets = append(podNsRuleSets, ns)
					}

					for podLabelKey, podLabelVal := range toRule.PodSelector.MatchLabels {
						podNsRuleSets = append(podNsRuleSets, util.KubeAllNamespacesFlag+"-"+podLabelKey+":"+podLabelVal)
					}

					// Handle PodSelector field of NetworkPolicyPeer.
					for _, podRuleSet := range podNsRuleSets {
						hashedPodRuleSetName := util.GetHashedName(podRuleSet)
						policy := &hcn.AclPolicySetting{
							Protocols:  "6",
							Action:     hcn.ActionTypeAllow,
							Direction:  hcn.DirectionTypeOut,
							LocalTags:  hashedTargetSetName,
							RemoteTags: hashedPodRuleSetName,
							RuleType:   hcn.RuleTypeSwitch,
							Priority:   egressFromPod,
						}
						policies = append(policies, policy)
					}
					appendAndClearSets(&podNsRuleSets, &nsRuleLists, &policyRuleSets, &policyRuleLists)
				}
			}
		}
	}

	log.Printf("finished parsing egress rule")
	return policyRuleSets, policyRuleLists, policies
}

// Drop all non-whitelisted packets.
func getDefaultDropPolicies(targetSets []string) []*hcn.AclPolicySetting {
	var policies []*hcn.AclPolicySetting

	for _, targetSet := range targetSets {
		hashedTargetSetName := util.GetHashedName(targetSet)
		policy := &hcn.AclPolicySetting{
			Protocols: "6",
			Action:    hcn.ActionTypeBlock,
			Direction: hcn.DirectionTypeOut,
			LocalTags: hashedTargetSetName,
			RuleType:  hcn.RuleTypeSwitch,
			Priority:  defaults,
		}
		policies = append(policies, policy)

		policy = &hcn.AclPolicySetting{
			Protocols: "6",
			Action:    hcn.ActionTypeBlock,
			Direction: hcn.DirectionTypeIn,
			LocalTags: hashedTargetSetName,
			RuleType:  hcn.RuleTypeSwitch,
			Priority:  defaults,
		}
		policies = append(policies, policy)
	}

	return policies
}

// Allow traffic from/to kube-system pods
func getAllowKubeSystemPolicies(ns string, targetSets []string) []*hcn.AclPolicySetting {
	var policies []*hcn.AclPolicySetting

	if len(targetSets) == 0 {
		targetSets = append(targetSets, ns)
	}

	for _, targetSet := range targetSets {
		hashedTargetSetName := util.GetHashedName(targetSet)
		hashedKubeSystemSet := util.GetHashedName(util.KubeSystemFlag)
		allowKubeSystemIngress := &hcn.AclPolicySetting{
			Protocols:  "6",
			Action:     hcn.ActionTypeAllow,
			Direction:  hcn.DirectionTypeIn,
			RemoteTags: hashedKubeSystemSet,
			LocalTags:  hashedTargetSetName,
			RuleType:   hcn.RuleTypeSwitch,
			Priority:   ingressPort,
		}
		policies = append(policies, allowKubeSystemIngress)

		allowKubeSystemEgress := &hcn.AclPolicySetting{
			Protocols:  "6",
			Action:     hcn.ActionTypeAllow,
			Direction:  hcn.DirectionTypeOut,
			RemoteTags: hashedKubeSystemSet,
			LocalTags:  hashedTargetSetName,
			RuleType:   hcn.RuleTypeSwitch,
			Priority:   egressPort,
		}
		policies = append(policies, allowKubeSystemEgress)
	}

	return policies
}

// ParsePolicy parses network policy.
func parsePolicy(npObj *networkingv1.NetworkPolicy) ([]string, []string, []*hcn.AclPolicySetting) {
	var (
		resultPodSets []string
		resultNsLists []string
		affectedSets  []string
		policies      []*hcn.AclPolicySetting
	)

	// Get affected pods.
	npNs, selector := npObj.ObjectMeta.Namespace, npObj.Spec.PodSelector.MatchLabels
	for podLabelKey, podLabelVal := range selector {
		affectedSet := util.KubeAllNamespacesFlag + "-" + podLabelKey + ":" + podLabelVal
		affectedSets = append(affectedSets, affectedSet)
	}

	if len(npObj.Spec.Ingress) > 0 || len(npObj.Spec.Egress) > 0 {
		policies = append(policies, getAllowKubeSystemPolicies(npNs, affectedSets)...)
	}

	if len(npObj.Spec.PolicyTypes) == 0 {
		ingressPodSets, ingressNsSets, ingressPolicies := parseIngress(npNs, affectedSets, npObj.Spec.Ingress)
		resultPodSets = append(resultPodSets, ingressPodSets...)
		resultNsLists = append(resultNsLists, ingressNsSets...)
		policies = append(policies, ingressPolicies...)

		egressPodSets, egressNsSets, egressPolicies := parseEgress(npNs, affectedSets, npObj.Spec.Egress)
		resultPodSets = append(resultPodSets, egressPodSets...)
		resultNsLists = append(resultNsLists, egressNsSets...)
		policies = append(policies, egressPolicies...)

		policies = append(policies, getDefaultDropPolicies(affectedSets)...)

		resultPodSets = append(resultPodSets, affectedSets...)

		return util.UniqueStrSlice(resultPodSets), util.UniqueStrSlice(resultNsLists), policies
	}

	for _, ptype := range npObj.Spec.PolicyTypes {
		if ptype == networkingv1.PolicyTypeIngress {
			ingressPodSets, ingressNsSets, ingressPolicies := parseIngress(npNs, affectedSets, npObj.Spec.Ingress)
			resultPodSets = append(resultPodSets, ingressPodSets...)
			resultNsLists = append(resultNsLists, ingressNsSets...)
			policies = append(policies, ingressPolicies...)
		}

		if ptype == networkingv1.PolicyTypeEgress {
			egressPodSets, egressNsSets, egressPolicies := parseEgress(npNs, affectedSets, npObj.Spec.Egress)
			resultPodSets = append(resultPodSets, egressPodSets...)
			resultNsLists = append(resultNsLists, egressNsSets...)
			policies = append(policies, egressPolicies...)
		}

		policies = append(policies, getDefaultDropPolicies(affectedSets)...)
	}

	resultPodSets = append(resultPodSets, affectedSets...)
	resultPodSets = append(resultPodSets, npNs)

	return util.UniqueStrSlice(resultPodSets), util.UniqueStrSlice(resultNsLists), policies
}
