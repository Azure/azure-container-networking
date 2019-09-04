// Copyright 2018 Microsoft. All rights reserved.
// MIT License
package npm

import (
	"math/bits"
	"sort"
	"strconv"
	"strings"

	"github.com/kalebmorris/azure-container-networking/log"
	"github.com/kalebmorris/azure-container-networking/npm/util"
	"github.com/kalebmorris/azure-container-networking/npm/vfpm"
	networkingv1 "k8s.io/api/networking/v1"
)

// getTargetTags gathers the tags that are affected by the policy.
func getTargetTags(npObj *networkingv1.NetworkPolicy) []string {
	var tags []string

	labels := npObj.Spec.PodSelector.MatchLabels

	if len(labels) == 0 {
		tags = append(tags, npObj.ObjectMeta.Namespace)
		return tags
	}

	for key, val := range labels {
		tag := util.KubeAllNamespacesFlag + "-" + key + ":" + val
		tags = append(tags, tag)
	}

	return util.UniqueStrSlice(tags)
}

// getPolicyTypes identifies whether ingress and egress rules are present.
func getPolicyTypes(npObj *networkingv1.NetworkPolicy) (bool, bool) {
	ingressExists, egressExists := false, false

	policyTypes := npObj.Spec.PolicyTypes
	if len(policyTypes) == 2 {
		ingressExists = true
		egressExists = true
	} else if len(policyTypes) == 1 && policyTypes[0] == networkingv1.PolicyTypeIngress {
		ingressExists = true
	} else if len(policyTypes) == 1 && policyTypes[0] == networkingv1.PolicyTypeEgress {
		egressExists = true
	} else {
		if npObj.Spec.Ingress != nil {
			ingressExists = true
		}
		if npObj.Spec.Egress != nil {
			egressExists = true
		}
	}

	return ingressExists, egressExists
}

// ipToInt takes a string of the form "x.y.z.w", and returns the corresponding
// uint32, where the 8 greatest bits represent the number x, the next 8 greatest
// represent the number y, and so on.
func ipToInt(ip string) (uint32, error) {
	result := uint32(0)
	bytes := strings.Split(ip, ".")
	for _, strByte := range bytes {
		result = result << 8
		converted, err := strconv.ParseUint(strByte, 10, 8)
		if err != nil {
			return 0, err
		}
		result += uint32(converted)
	}
	return result, nil
}

// getRanges takes an IPBlock and returns two uint32 slices of the same length.
// The first contains the starts of the IP ranges that the IPBlock represents,
// and the second contains the end of those ranges.
func getRanges(ipb *networkingv1.IPBlock) ([]uint32, []uint32) {
	idx := strings.Index(ipb.CIDR, "/")

	// Convert IP to integer.
	ipStr := ipb.CIDR[:idx]
	ip, err := ipToInt(ipStr)
	if err != nil {
		return nil, nil
	}

	// Convert CIDR mask to int.
	mask64, err := strconv.ParseUint(ipb.CIDR[idx+1:], 10, 6)
	if err != nil {
		return nil, nil
	}
	mask := ^((uint32(1) << (32 - mask64)) - 1)

	globalStart := ip & mask
	globalEnd := globalStart | ^mask
	starts := []uint32{globalStart}
	ends := []uint32{globalEnd}

	for _, except := range ipb.Except {
		idx = strings.Index(except, "/")

		// Convert IP to integer.
		ipStr = except[:idx]
		ip, err = ipToInt(ipStr)
		if err != nil {
			return nil, nil
		}

		// Convert CIDR mask to int.
		mask64, err = strconv.ParseUint(except[idx+1:], 10, 6)
		if err != nil {
			return nil, nil
		}
		mask = ^((uint32(1) << (32 - mask64)) - 1)
		start := ip & mask
		end := start | ^mask
		if start < globalStart || end > globalEnd {
			continue
		}

		// Cut out parts of the ranges that overlap with except.
		var tmpStarts, tmpEnds []uint32
		for i := range starts {
			if start > ends[i] || end < starts[i] {
				// Case 0: Ranges don't overlap.
				tmpStarts = append(tmpStarts, starts[i])
				tmpEnds = append(tmpEnds, ends[i])
			} else if start <= starts[i] && end >= ends[i] {
				// Case 1: Range is covered by except entirely.
				continue
			} else if start <= starts[i] {
				// Case 2: Range's left endpoint covered.
				tmpStarts = append(tmpStarts, end+1)
				tmpEnds = append(tmpEnds, ends[i])
			} else if end >= ends[i] {
				// Case 3: Range's right endpoint covered.
				tmpStarts = append(tmpStarts, starts[i])
				tmpEnds = append(tmpEnds, start-1)
			} else {
				// Case 4: Except lies entirely inside range.
				tmpStarts = append(tmpStarts, starts[i])
				tmpEnds = append(tmpEnds, start-1)
				tmpStarts = append(tmpStarts, end+1)
				tmpEnds = append(tmpEnds, ends[i])
			}
		}
		starts = tmpStarts
		ends = tmpEnds
	}

	sort.Slice(starts, func(i, j int) bool { return starts[i] < starts[j] })
	sort.Slice(ends, func(i, j int) bool { return ends[i] < ends[j] })
	return starts, ends
}

// getStrCIDR takes an ip (as a uint32) and a CIDR mask and returns
// the string version of that CIDR in the usual format.
// ex: getStrCIDR(1,32) would return "0.0.0.1/32"
func getStrCIDR(start uint32, maskNum int) string {
	result := ""
	for i := 3; i >= 0; i-- {
		byteMask := (uint32(1) << 8) - 1
		isolated := (start >> uint32(8*i)) & byteMask
		result += strconv.FormatUint(uint64(isolated), 10)
		if i != 0 {
			result += "."
		} else {
			result += "/"
		}
	}
	result += strconv.Itoa(maskNum)
	return result
}

// getCIDRs takes an IPBlock and returns a comma-separated string
// of the CIDRS it represents.
func getCIDRs(ipb *networkingv1.IPBlock) string {
	var cidrs []string
	starts, ends := getRanges(ipb)
	for i := range starts {
		for {
			needToGo := ends[i] - starts[i] + 1
			maskNum := 32 - bits.TrailingZeros32(starts[i])
			canGo := uint32(1) << uint32(32-maskNum)
			for canGo > needToGo {
				canGo /= 2
				maskNum++
			}

			cidr := getStrCIDR(starts[i], maskNum)
			cidrs = append(cidrs, cidr)
			starts[i] += canGo

			if starts[i] >= ends[i] {
				break
			}
		}
	}

	return strings.Join(cidrs, ",")
}

// getAffectedNamespaces gathers the namespaces selected by matchLabels, and also returns the NLTags affected.
func getAffectedNamespaces(matchLabels map[string]string, tMgr *vfpm.TagManager) ([]string, []string) {
	var affectedNamespaces []string
	var NLTags []string
	for key, val := range matchLabels {
		labelNLTag := util.GetNsIpsetName(key, val)
		NLTags = append(NLTags, labelNLTag)

		elementStr := tMgr.GetFromNLTag(labelNLTag)
		elements := strings.Split(elementStr, ",")
		for _, element := range elements {
			if element == "" {
				continue
			}
			affectedNamespaces = append(affectedNamespaces, element)
		}
	}
	return util.UniqueStrSlice(affectedNamespaces), util.UniqueStrSlice(NLTags)
}

// getSourceRules gets rules according to the from rules in "from".
func getSourceRules(from []networkingv1.NetworkPolicyPeer, ns string, dest string, tMgr *vfpm.TagManager) ([]string, []string, []*vfpm.Rule) {
	var (
		ingressTags   []string
		ingressNLTags []string
		ingressRules  []*vfpm.Rule
	)
	for _, source := range from {
		if source.IPBlock != nil {
			// Add IPBlock rules.
			cidrs := getCIDRs(source.IPBlock)
			cidrsRule := &vfpm.Rule{
				Name:     dest + "-ip-" + cidrs,
				Group:    util.NPMIngressGroup,
				DstTags:  dest,
				SrcIPs:   cidrs,
				Priority: 50,
				Action:   util.Allow,
			}
			ingressRules = append(ingressRules, cidrsRule)
		} else {
			// Add namespace and pod selector rules.
			var allNamespaces, allPods bool
			if source.NamespaceSelector != nil && source.PodSelector != nil {
				if len(source.NamespaceSelector.MatchLabels) == 0 {
					allNamespaces = true
				}
				if len(source.PodSelector.MatchLabels) == 0 {
					allPods = true
				}

				if allNamespaces && allPods {
					// Add a rule to allow all ingress traffic to the target tag.
					allowAllRule := &vfpm.Rule{
						Name:     dest + "-allow-all",
						Group:    util.NPMIngressGroup,
						DstTags:  dest,
						Priority: 50,
						Action:   util.Allow,
					}
					ingressRules = append(ingressRules, allowAllRule)
				} else if allNamespaces {
					// Add rules to allow ingress traffic from select labels.
					for key, val := range source.PodSelector.MatchLabels {
						labelTag := util.KubeAllNamespacesFlag + "-" + key + ":" + val
						allowLabelRule := &vfpm.Rule{
							Name:     dest + "-" + labelTag,
							Group:    util.NPMIngressGroup,
							DstTags:  dest,
							SrcTags:  labelTag,
							Priority: 50,
							Action:   util.Allow,
						}
						ingressRules = append(ingressRules, allowLabelRule)
						ingressTags = append(ingressTags, labelTag)
					}
				} else if allPods {
					// Add rules to allow ingress traffic from select namespaces.
					var newNLTags []string
					affectedNamespaces, newNLTags := getAffectedNamespaces(source.NamespaceSelector.MatchLabels, tMgr)
					for _, ns := range affectedNamespaces {
						allowNsRule := &vfpm.Rule{
							Name:     dest + "-" + ns,
							Group:    util.NPMIngressGroup,
							DstTags:  dest,
							SrcTags:  ns,
							Priority: 50,
							Action:   util.Allow,
						}
						ingressRules = append(ingressRules, allowNsRule)
					}
					ingressNLTags = append(ingressNLTags, newNLTags...)
				} else {
					// Add rules to allow ingress traffic from select namespaces and select pod label groups.
					var newNLTags []string
					affectedNamespaces, newNLTags := getAffectedNamespaces(source.NamespaceSelector.MatchLabels, tMgr)
					for key, val := range source.PodSelector.MatchLabels {
						for _, ns := range affectedNamespaces {
							labelTag := ns + "-" + key + ":" + val
							allowLabelRule := &vfpm.Rule{
								Name:     dest + "-" + labelTag,
								Group:    util.NPMIngressGroup,
								DstTags:  dest,
								SrcTags:  labelTag,
								Priority: 50,
								Action:   util.Allow,
							}
							ingressRules = append(ingressRules, allowLabelRule)
							ingressTags = append(ingressTags, labelTag)
						}
					}
					ingressNLTags = append(ingressNLTags, newNLTags...)
				}
			} else if source.NamespaceSelector != nil {
				// Add rules to allow ingress traffic from select namespaces.
				var newNLTags []string
				affectedNamespaces, newNLTags := getAffectedNamespaces(source.NamespaceSelector.MatchLabels, tMgr)
				for _, ns := range affectedNamespaces {
					allowNsRule := &vfpm.Rule{
						Name:     dest + "-" + ns,
						Group:    util.NPMIngressGroup,
						DstTags:  dest,
						SrcTags:  ns,
						Priority: 50,
						Action:   util.Allow,
					}
					ingressRules = append(ingressRules, allowNsRule)
				}
				ingressNLTags = append(ingressNLTags, newNLTags...)
				ingressNLTags = append(ingressNLTags, util.KubeAllNamespacesFlag)
			} else if source.PodSelector != nil {
				// Add rules to allow ingress traffic from select pods in the network policy's namespace.
				for key, val := range source.PodSelector.MatchLabels {
					labelTag := ns + "-" + key + ":" + val
					allowLabelRule := &vfpm.Rule{
						Name:     dest + "-" + labelTag,
						Group:    util.NPMIngressGroup,
						DstTags:  dest,
						SrcTags:  labelTag,
						Priority: 50,
						Action:   util.Allow,
					}
					ingressRules = append(ingressRules, allowLabelRule)
					ingressTags = append(ingressTags, labelTag)
				}
				ingressTags = append(ingressTags, ns)
			}
		}
	}

	return ingressTags, ingressNLTags, ingressRules
}

// parseIngress parses npObj for ingress rules.
func parseIngress(npObj *networkingv1.NetworkPolicy, targetTags []string, tMgr *vfpm.TagManager) ([]string, []string, []*vfpm.Rule) {
	var (
		ingressTags   []string
		ingressNLTags []string
		ingressRules  []*vfpm.Rule
	)

	for _, targetTag := range targetTags {
		// Add default deny rule.
		drop := &vfpm.Rule{
			Name:     targetTag + "-drop",
			Group:    util.NPMIngressGroup,
			DstTags:  targetTag,
			Priority: 59999, // one less than max of 60000
			Action:   util.Block,
		}
		ingressRules = append(ingressRules, drop)

		// Add kube-system allow rule.
		kubeSysAllow := &vfpm.Rule{
			Name:     targetTag + "-kube-system-allow",
			Group:    util.NPMIngressGroup,
			DstTags:  targetTag,
			SrcTags:  util.KubeSystemFlag,
			Priority: 50,
			Action:   util.Allow,
		}
		ingressRules = append(ingressRules, kubeSysAllow)
		ingressTags = append(ingressTags, util.KubeSystemFlag)

		// Process all rules.
		for _, rule := range npObj.Spec.Ingress {
			// Identify special cases on sources and ports.
			var allSources, allPorts bool
			if rule.From == nil || len(rule.From) == 0 {
				allSources = true
			}
			if rule.Ports == nil || len(rule.Ports) == 0 {
				allPorts = true
			}

			if allSources && allPorts {
				// Add allow rule.
				allow := &vfpm.Rule{
					Name:     targetTag + "-allow",
					Group:    util.NPMIngressGroup,
					DstTags:  targetTag,
					Priority: 50,
					Action:   util.Allow,
				}
				ingressRules = append(ingressRules, allow)
			} else if allSources {
				// Add port rules.
				for _, port := range rule.Ports {
					portAllow := &vfpm.Rule{
						Name:     targetTag + "-port-" + port.Port.String(),
						Group:    util.NPMIngressGroup,
						DstTags:  targetTag,
						SrcPrts:  port.Port.String(),
						Priority: 50,
						Action:   util.Allow,
					}
					ingressRules = append(ingressRules, portAllow)
				}
			} else if allPorts {
				// Add source rules.
				var (
					newTags   []string
					newNLTags []string
					newRules  []*vfpm.Rule
				)
				newTags, newNLTags, newRules = getSourceRules(rule.From, npObj.ObjectMeta.Namespace, targetTag, tMgr)
				ingressTags = append(ingressTags, newTags...)
				ingressNLTags = append(ingressNLTags, newNLTags...)
				ingressRules = append(ingressRules, newRules...)
			} else {
				// Add rules to allow ingress traffic on the provided ports from the provided sources.
				var (
					newTags   []string
					newNLTags []string
					newRules  []*vfpm.Rule
				)
				newTags, newNLTags, newRules = getSourceRules(rule.From, npObj.ObjectMeta.Namespace, targetTag, tMgr)
				for _, port := range rule.Ports {
					for _, sourceRule := range newRules {
						allowRule := *sourceRule
						allowRule.SrcPrts = port.Port.String()
						allowRule.Name = allowRule.Name + "-port-" + port.Port.String()
						ingressRules = append(ingressRules, &allowRule)
					}
				}
				ingressTags = append(ingressTags, newTags...)
				ingressNLTags = append(ingressNLTags, newNLTags...)
			}
		}
	}

	return ingressTags, ingressNLTags, ingressRules
}

// getDestinationRules gets rules according to the to rules in "to".
func getDestinationRules(to []networkingv1.NetworkPolicyPeer, ns string, src string, tMgr *vfpm.TagManager) ([]string, []string, []*vfpm.Rule) {
	var (
		egressTags   []string
		egressNLTags []string
		egressRules  []*vfpm.Rule
	)
	for _, dest := range to {
		if dest.IPBlock != nil {
			// Add IPBlock rules.
			cidrs := getCIDRs(dest.IPBlock)
			cidrsRule := &vfpm.Rule{
				Name:     src + "-ip-" + cidrs,
				Group:    util.NPMEgressGroup,
				SrcTags:  src,
				DstIPs:   cidrs,
				Priority: 50,
				Action:   util.Allow,
			}
			egressRules = append(egressRules, cidrsRule)
		} else {
			// Add namespace and pod selector rules.
			var allNamespaces, allPods bool
			if dest.NamespaceSelector != nil && dest.PodSelector != nil {
				if len(dest.NamespaceSelector.MatchLabels) == 0 {
					allNamespaces = true
				}
				if len(dest.PodSelector.MatchLabels) == 0 {
					allPods = true
				}

				if allNamespaces && allPods {
					// Add a rule to allow all egress traffic to the target tag.
					allowAllRule := &vfpm.Rule{
						Name:     src + "-allow-all",
						Group:    util.NPMEgressGroup,
						SrcTags:  src,
						Priority: 50,
						Action:   util.Allow,
					}
					egressRules = append(egressRules, allowAllRule)
				} else if allNamespaces {
					// Add rules to allow egress traffic to select labels.
					for key, val := range dest.PodSelector.MatchLabels {
						labelTag := util.KubeAllNamespacesFlag + "-" + key + ":" + val
						allowLabelRule := &vfpm.Rule{
							Name:     src + "-" + labelTag,
							Group:    util.NPMEgressGroup,
							SrcTags:  src,
							DstTags:  labelTag,
							Priority: 50,
							Action:   util.Allow,
						}
						egressRules = append(egressRules, allowLabelRule)
						egressTags = append(egressTags, labelTag)
					}
				} else if allPods {
					// Add rules to allow egress traffic to select namespaces.
					var newNLTags []string
					affectedNamespaces, newNLTags := getAffectedNamespaces(dest.NamespaceSelector.MatchLabels, tMgr)
					for _, ns := range affectedNamespaces {
						allowNsRule := &vfpm.Rule{
							Name:     src + "-" + ns,
							Group:    util.NPMEgressGroup,
							SrcTags:  src,
							DstTags:  ns,
							Priority: 50,
							Action:   util.Allow,
						}
						egressRules = append(egressRules, allowNsRule)
					}
					egressNLTags = append(egressNLTags, newNLTags...)
				} else {
					// Add rules to allow egress traffic to select namespaces and select pod label groups.
					var newNLTags []string
					affectedNamespaces, newNLTags := getAffectedNamespaces(dest.NamespaceSelector.MatchLabels, tMgr)
					for key, val := range dest.PodSelector.MatchLabels {
						for _, ns := range affectedNamespaces {
							labelTag := ns + "-" + key + ":" + val
							allowLabelRule := &vfpm.Rule{
								Name:     src + "-" + labelTag,
								Group:    util.NPMEgressGroup,
								SrcTags:  src,
								DstTags:  labelTag,
								Priority: 50,
								Action:   util.Allow,
							}
							egressRules = append(egressRules, allowLabelRule)
							egressTags = append(egressTags, labelTag)
						}
					}
					egressNLTags = append(egressNLTags, newNLTags...)
				}
			} else if dest.NamespaceSelector != nil {
				// Add rules to allow egress traffic from select namespaces.
				var newNLTags []string
				affectedNamespaces, newNLTags := getAffectedNamespaces(dest.NamespaceSelector.MatchLabels, tMgr)
				for _, ns := range affectedNamespaces {
					allowNsRule := &vfpm.Rule{
						Name:     src + "-" + ns,
						Group:    util.NPMEgressGroup,
						SrcTags:  src,
						DstTags:  ns,
						Priority: 50,
						Action:   util.Allow,
					}
					egressRules = append(egressRules, allowNsRule)
				}
				egressNLTags = append(egressNLTags, newNLTags...)
				egressNLTags = append(egressNLTags, util.KubeAllNamespacesFlag)
			} else if dest.PodSelector != nil {
				// Add rules to allow egress traffic to select pods in the network policy's namespace.
				for key, val := range dest.PodSelector.MatchLabels {
					labelTag := ns + "-" + key + ":" + val
					allowLabelRule := &vfpm.Rule{
						Name:     src + "-" + labelTag,
						Group:    util.NPMEgressGroup,
						SrcTags:  src,
						DstTags:  labelTag,
						Priority: 50,
						Action:   util.Allow,
					}
					egressRules = append(egressRules, allowLabelRule)
					egressTags = append(egressTags, labelTag)
				}
				egressTags = append(egressTags, ns)
			}
		}
	}

	return egressTags, egressNLTags, egressRules
}

// parseEgress parses npObj for Egress rules.
func parseEgress(npObj *networkingv1.NetworkPolicy, targetTags []string, tMgr *vfpm.TagManager) ([]string, []string, []*vfpm.Rule) {
	var (
		egressTags   []string
		egressNLTags []string
		egressRules  []*vfpm.Rule
	)

	for _, targetTag := range targetTags {
		// Add default deny rule.
		drop := &vfpm.Rule{
			Name:     targetTag + "-drop",
			Group:    util.NPMEgressGroup,
			SrcTags:  targetTag,
			Priority: 59999, // one less than max of 60000
			Action:   util.Block,
		}
		egressRules = append(egressRules, drop)

		// Add kube-system allow rule.
		kubeSysAllow := &vfpm.Rule{
			Name:     targetTag + "-kube-system-allow",
			Group:    util.NPMEgressGroup,
			SrcTags:  targetTag,
			DstTags:  util.KubeSystemFlag,
			Priority: 50,
			Action:   util.Allow,
		}
		egressRules = append(egressRules, kubeSysAllow)
		egressTags = append(egressTags, util.KubeSystemFlag)

		// Process all rules.
		for _, rule := range npObj.Spec.Egress {
			// Identify special cases on destinations and ports.
			var allDestinations, allPorts bool
			if rule.To == nil || len(rule.To) == 0 {
				allDestinations = true
			}
			if rule.Ports == nil || len(rule.Ports) == 0 {
				allPorts = true
			}

			if allDestinations && allPorts {
				// Add allow rule.
				allow := &vfpm.Rule{
					Name:     targetTag + "-allow",
					Group:    util.NPMEgressGroup,
					SrcTags:  targetTag,
					Priority: 50,
					Action:   util.Allow,
				}
				egressRules = append(egressRules, allow)
			} else if allDestinations {
				// Add port rules.
				for _, port := range rule.Ports {
					portAllow := &vfpm.Rule{
						Name:     targetTag + "-port-" + port.Port.String(),
						Group:    util.NPMEgressGroup,
						SrcTags:  targetTag,
						DstPrts:  port.Port.String(),
						Priority: 50,
						Action:   util.Allow,
					}
					egressRules = append(egressRules, portAllow)
				}
			} else if allPorts {
				// Add destination rules.
				var (
					newTags   []string
					newNLTags []string
					newRules  []*vfpm.Rule
				)
				newTags, newNLTags, newRules = getDestinationRules(rule.To, npObj.ObjectMeta.Namespace, targetTag, tMgr)
				egressTags = append(egressTags, newTags...)
				egressNLTags = append(egressNLTags, newNLTags...)
				egressRules = append(egressRules, newRules...)
			} else {
				// Add rules to allow egress traffic on the provided ports to the provided destinations.
				var (
					newTags   []string
					newNLTags []string
					newRules  []*vfpm.Rule
				)
				newTags, newNLTags, newRules = getDestinationRules(rule.To, npObj.ObjectMeta.Namespace, targetTag, tMgr)
				for _, port := range rule.Ports {
					for _, destinationRule := range newRules {
						allowRule := *destinationRule
						allowRule.DstPrts = port.Port.String()
						allowRule.Name = allowRule.Name + "-port-" + port.Port.String()
						egressRules = append(egressRules, &allowRule)
					}
				}
				egressTags = append(egressTags, newTags...)
				egressNLTags = append(egressNLTags, newNLTags...)
			}
		}
	}

	return egressTags, egressNLTags, egressRules
}

// parsePolicy parses network policy.
func parsePolicy(npObj *networkingv1.NetworkPolicy, tMgr *vfpm.TagManager) ([]string, []string, []*vfpm.Rule) {
	var (
		resultTags   []string
		resultNLTags []string
		resultRules  []*vfpm.Rule
	)

	// Retrieve the selected pods (tags).
	targetTags := getTargetTags(npObj)

	// Identify which rule types exist.
	ingressExists, egressExists := getPolicyTypes(npObj)

	// Parse ingress rules.
	if ingressExists {
		ingressTags, ingressNLTags, ingressRules := parseIngress(npObj, targetTags, tMgr)
		resultTags = append(resultTags, ingressTags...)
		resultNLTags = append(resultNLTags, ingressNLTags...)
		resultRules = append(resultRules, ingressRules...)
	}

	// Parse egress rules.
	if egressExists {
		egressTags, egressNLTags, egressRules := parseEgress(npObj, targetTags, tMgr)
		resultTags = append(resultTags, egressTags...)
		resultNLTags = append(resultNLTags, egressNLTags...)
		resultRules = append(resultRules, egressRules...)
	}

	// Account for target sets for the returned sets.
	resultTags = append(resultTags, targetTags...)

	log.Printf("Finished parsing policy: %s", npObj.ObjectMeta.Name)

	// Return unique sets.
	return util.UniqueStrSlice(resultTags), util.UniqueStrSlice(resultNLTags), resultRules
}
