// Copyright 2018 Microsoft. All rights reserved.
// MIT License
package npm

import (
	"fmt"
	"reflect"

	networkingv1 "k8s.io/api/networking/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func isSamePolicy(old, new *networkingv1.NetworkPolicy) bool {
	if !reflect.DeepEqual(old.TypeMeta, new.TypeMeta) {
		return false
	}

	if old.ObjectMeta.Namespace != new.ObjectMeta.Namespace {
		return false
	}

	if !reflect.DeepEqual(old.Spec, new.Spec) {
		return false
	}

	return true
}

// addPolicy merges policies based on labels.
func addPolicy(old, new *networkingv1.NetworkPolicy) (*networkingv1.NetworkPolicy, error) {
	// if namespace matches && podSelector matches, then merge
	// else return as is.
	if !reflect.DeepEqual(old.TypeMeta, new.TypeMeta) {
		return nil, fmt.Errorf("Old and new networkpolicies don't have the same TypeMeta")
	}

	if old.ObjectMeta.Namespace != new.ObjectMeta.Namespace {
		return nil, fmt.Errorf("Old and new networkpolicies don't have the same namespace")
	}

	if !reflect.DeepEqual(old.Spec.PodSelector, new.Spec.PodSelector) {
		return nil, fmt.Errorf("Old and new networkpolicies don't apply to the same set of target pods")
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
	if len(old.Spec.PolicyTypes) == 1 && len(new.Spec.PolicyTypes) == 1 && old.Spec.PolicyTypes[0] == new.Spec.PolicyTypes[0] {
		spec.PolicyTypes = []networkingv1.PolicyType{new.Spec.PolicyTypes[0]}
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

	if !reflect.DeepEqual(old.Spec.PodSelector, new.Spec.PodSelector) {
		return nil, fmt.Errorf("Old and new networkpolicy don't have apply to the same set of target pods")
	}

	if reflect.DeepEqual(old.Spec, new.Spec) {
		return nil, nil
	}

	deductedPolicy := &networkingv1.NetworkPolicy{
		TypeMeta: old.TypeMeta,
		ObjectMeta: metav1.ObjectMeta{
			Name:      old.ObjectMeta.Name,
			Namespace: old.ObjectMeta.Namespace,
		},
		Spec: networkingv1.NetworkPolicySpec{
			PodSelector: old.Spec.PodSelector,
		},
	}

	deductedIngress, newIngress := old.Spec.Ingress, new.Spec.Ingress
	deductedEgress, newEgress := old.Spec.Egress, new.Spec.Egress
	for _, ni := range newIngress {
		for i, di := range deductedIngress {
			if reflect.DeepEqual(di, ni) {
				deductedIngress = append(deductedIngress[:i], deductedIngress[i+1:]...)
				break
			}
		}
	}

	for _, ne := range newEgress {
		for i, de := range deductedEgress {
			if reflect.DeepEqual(de, ne) {
				deductedEgress = append(deductedEgress[:i], deductedEgress[i+1:]...)
				break
			}
		}
	}

	deductedPolicy.Spec.Ingress = deductedIngress
	deductedPolicy.Spec.Egress = deductedEgress

	if len(old.Spec.PolicyTypes) == 1 && len(new.Spec.PolicyTypes) == 1 && old.Spec.PolicyTypes[0] == new.Spec.PolicyTypes[0] {
		deductedPolicy.Spec.PolicyTypes = []networkingv1.PolicyType{new.Spec.PolicyTypes[0]}
	} else {
		deductedPolicy.Spec.PolicyTypes = []networkingv1.PolicyType{
			networkingv1.PolicyTypeIngress,
			networkingv1.PolicyTypeEgress,
		}
	}

	return deductedPolicy, nil
}
