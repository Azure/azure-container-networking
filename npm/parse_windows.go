// Copyright 2018 Microsoft. All rights reserved.
// MIT License
package npm

import (
	"math/bits"
	"strconv"
	"strings"

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
	mask64, err := strconv.ParseUint(ipb.CIDR[idx+1:], 10, 5)
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
		mask64, err = strconv.ParseUint(except[idx+1:], 10, 5)
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
			if starts[i] < start && ends[i] > end {
				// Ranges don't overlap.
				continue
			}

			if start <= starts[i] && end >= ends[i] {
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

	return starts, ends
}

// getStrCIDR takes an ip (as a uint32) and a CIDR mask and returns 
// the string version of that CIDR in the usual format. 
// ex: getStrCIDR(1,32) would return "0.0.0.1/32"
func getStrCIDR(start uint32, maskNum int) string {
	result := ""
	for i := 3; i >= 0; i-- {
		byteMask := (uint32(1) << 8) - 1
		isolated := (start >> uint32(8 * i)) & byteMask
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
			canGo := uint32(1) << uint32(32 - maskNum)
			for canGo > needToGo {
				canGo /= 2
				maskNum++
			}

			cidr := getStrCIDR(starts[i], maskNum)
			cidrs = append(cidrs, cidr)
			starts[i] += canGo

			if starts[i] == ends[i] {
				break
			}
		}
	}

	return strings.Join(cidrs, ",")
}

func parseIngress(npObj *networkingv1.NetworkPolicy, targetTags []string) ([]string, []string, []*vfpm.Rule) {
	var (
		ingressTags   []string
		ingressNLTags []string
		ingressRules  []*vfpm.Rule
	)

	for _, targetTag := range targetTags {
		hashedTag := util.GetHashedName(targetTag)
		hashedKubeSys := util.GetHashedName(util.KubeSystemFlag)

		// Add default deny rule.
		drop := &vfpm.Rule{
			Name:     hashedTag + "-drop",
			Group:    util.NPMIngressDefaultGroup,
			DstTags:  hashedTag,
			Priority: ^uint16(0), // max uint16
			Action:   util.Block,
		}
		ingressRules = append(ingressRules, drop)

		// Add kube-system allow rule.
		kubeSysAllow := &vfpm.Rule{
			Name:     hashedTag + "-kube-system-allow",
			Group:    util.NPMIngressDefaultGroup,
			DstTags:  hashedTag,
			SrcTags:  hashedKubeSys,
			Priority: 0,
			Action:   util.Allow,
		}
		ingressRules = append(ingressRules, kubeSysAllow)

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
					Name:     hashedTag + "-allow",
					Group:    util.NPMIngressGroup,
					DstTags:  hashedTag,
					Priority: 0,
					Action:   util.Allow,
				}
				ingressRules = append(ingressRules, allow)
			} else if allSources {
				// Add port rules.
				for _, port := range rule.Ports {
					portAllow := &vfpm.Rule{
						Name:     hashedTag + "-port-" + port.Port.String(),
						Group:    util.NPMIngressGroup,
						DstTags:  hashedTag,
						SrcPrts:  port.Port.String(),
						Priority: 0,
						Action:   util.Allow,
					}
					ingressRules = append(ingressRules, portAllow)
				}
			} else if allPorts {
				// Add source rules.
				for _, source := range rule.From {
					if source.IPBlock != nil {
						// Add IPBlock rules.
						cidrs := getCIDRs(source.IPBlock)
						cidrsRule := &vfpm.Rule{
							Name:     hashedTag + "-ip-" + cidrs,
							Group:    util.NPMIngressGroup,
							DstTags:  hashedTag,
							SrcIPs:   cidrs,
							Priority: 0,
							Action:   util.Allow,
						}
						ingressRules = append(ingressRules, cidrsRule)
					} else {

					}
				}
			} else {

			}
		}
	}

	return ingressTags, ingressNLTags, ingressRules
}

// parsePolicy parses network policy.
func parsePolicy(npObj *networkingv1.NetworkPolicy) ([]string, []string, []*vfpm.Rule) {
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
		ingressTags, ingressNLTags, ingressRules := parseIngress(npObj, targetTags)
		resultTags = append(resultTags, ingressTags...)
		resultNLTags = append(resultNLTags, ingressNLTags...)
		resultRules = append(resultRules, ingressRules...)
	}

	// Parse egress rules.
	if egressExists {
		egressTags, egressNLTags, egressRules := parseEgress(npObj, targetTags)
		resultTags = append(resultTags, egressTags...)
		resultNLTags = append(resultNLTags, egressNLTags...)
		resultRules = append(resultRules, egressRules...)
	}

	// Account for target sets for the returned sets.
	resultTags = append(resultTags, targetTags...)

	// Return unique sets.
	return util.UniqueStrSlice(resultTags), util.UniqueStrSlice(resultNLTags), resultRules
}
