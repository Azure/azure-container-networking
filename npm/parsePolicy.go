// Copyright 2018 Microsoft. All rights reserved.
// MIT License
package npm

import (
	"fmt"
	"reflect"

	networkingv1 "k8s.io/api/networking/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func splitPolicy(npObj *networkingv1.NetworkPolicy) ([]string, []*networkingv1.NetworkPolicy) {
	var policies []*networkingv1.NetworkPolicy

	labels, keys, vals := ParseSelector(&(npObj.Spec.PodSelector))
	for i := range keys {
		policy := *npObj
		policy.ObjectMeta.Name = labels[i]
		policy.Spec.PodSelector.MatchExpressions = []metav1.LabelSelectorRequirement{}
		policy.Spec.PodSelector.MatchLabels = map[string]string{keys[i]: vals[i]}
		policies = append(policies, &policy)
	}

	return labels, policies
}

// addPolicy merges policies based on labels.
func addPolicy(old, new *networkingv1.NetworkPolicy) (*networkingv1.NetworkPolicy, error) {
	// if namespace matches && podSelector matches, then merge
	// else return as is.
	if !reflect.DeepEqual(old.TypeMeta, new.TypeMeta) {
		return nil, fmt.Errorf("Old and new networkpolicy don't have the same TypeMeta")
	}

	if old.ObjectMeta.Namespace != new.ObjectMeta.Namespace {
		return nil, fmt.Errorf("Old and new networkpolicy don't have the same namespace")
	}

	if len(old.Spec.PodSelector.MatchLabels) != 1 || !reflect.DeepEqual(old.Spec.PodSelector, new.Spec.PodSelector) {
		return nil, fmt.Errorf("Old and new networkpolicy don't have apply to the same set of target pods")
	}

	addedPolicy := &networkingv1.NetworkPolicy{
		TypeMeta: old.TypeMeta,
		ObjectMeta: metav1.ObjectMeta{
			Name:      old.ObjectMeta.Name,
			Namespace: old.ObjectMeta.Namespace,
		},
		Spec: networkingv1.NetworkPolicySpec{
			PodSelector: old.Spec.PodSelector,
		},
	}

	spec := &(addedPolicy.Spec)
	if len(old.Spec.PolicyTypes) == 1 && old.Spec.PolicyTypes[0] == networkingv1.PolicyTypeEgress &&
		len(new.Spec.PolicyTypes) == 1 && new.Spec.PolicyTypes[0] == networkingv1.PolicyTypeEgress {
		spec.PolicyTypes = []networkingv1.PolicyType{networkingv1.PolicyTypeEgress}
	} else {
		spec.PolicyTypes = []networkingv1.PolicyType{
			networkingv1.PolicyTypeIngress,
			networkingv1.PolicyTypeEgress,
		}
	}

	ingress := append(old.Spec.Ingress, new.Spec.Ingress...)
	egress := append(old.Spec.Egress, new.Spec.Egress...)
	addedPolicy.Spec.Ingress = ingress
	addedPolicy.Spec.Egress = egress

	return addedPolicy, nil
}

// deductPolicy deduct one policy from the other.
func deductPolicy(old, new *networkingv1.NetworkPolicy) (*networkingv1.NetworkPolicy, error) {
	// if namespace matches && podSelector matches, then merge
	// else return as is.
	if !reflect.DeepEqual(old.TypeMeta, new.TypeMeta) {
		return nil, fmt.Errorf("Old and new networkpolicy don't have the same TypeMeta")
	}

	if old.ObjectMeta.Namespace != new.ObjectMeta.Namespace {
		return nil, fmt.Errorf("Old and new networkpolicy don't have the same namespace")
	}

	if len(old.Spec.PodSelector.MatchLabels) != 1 || !reflect.DeepEqual(old.Spec.PodSelector, new.Spec.PodSelector) {
		return nil, fmt.Errorf("Old and new networkpolicy don't have apply to the same set of target pods")
	}

}
