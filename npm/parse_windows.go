// Copyright 2018 Microsoft. All rights reserved.
// MIT License
package npm

import (
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

func parseIngress(npObj *networkingv1.NetworkPolicy, targetTags []string) ([]string, []string, []*vfpm.Rule) {
	var (
		ingressTags   []string
		ingressNLTags []string
		ingressRules  []*vfpm.Rule
	)

	for _, targetTag := range targetTags {
		hashedTag := util.GetHashedName(targetTag)

		// Add default deny rule.
		drop := &vfpm.Rule{
			Name:     targetTag,
			Group:    util.NPMIngressDefaultGroup,
			DstTags:  hashedTag,
			Priority: ^uint16(0),
		}
	}
}

// ParsePolicy parses network policy.
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
