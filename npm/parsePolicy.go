// Copyright 2018 Microsoft. All rights reserved.
// MIT License
package npm

import (
	networkingv1 "k8s.io/api/networking/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func splitPolicy(npObj *networkingv1.NetworkPolicy) ([]string, []*networkingv1.NetworkPolicy) {
	var policies []*networkingv1.NetworkPolicy

	labels, keys, vals := ParseSelector(&(npObj.Spec.PodSelector))
	for i := range keys {
		policy := *npObj
		policy.Spec.PodSelector.MatchExpressions = []metav1.LabelSelectorRequirement{}
		policy.Spec.PodSelector.MatchLabels[keys[i]] = vals[i]
		policies = append(policies, &policy)
	}

	return labels, policies
}

// mergePolicy merges policies based on labels.
func mergePolicy(old *networkingv1.NetworkPolicy, new *networkingv1.NetworkPolicy) (*networkingv1.NetworkPolicy, error) {
	// if namespace matches && podSelector matches, then merge
	// else return as is.
	return &networkingv1.NetworkPolicy{}, nil
}
