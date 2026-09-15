package translation

import (
	"fmt"
	"testing"

	"github.com/Azure/azure-container-networking/npm/pkg/dataplane/policies"
	"github.com/stretchr/testify/require"
	networkingv1 "k8s.io/api/networking/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
)

func combinedBudgetPolicy(direction networkingv1.PolicyType, labelCount, branchCount, portCount int) *networkingv1.NetworkPolicy {
	labels := make(map[string]string, labelCount)
	for index := 0; index < labelCount; index++ {
		labels[fmt.Sprintf("key%d", index)] = "value"
	}
	values := make([]string, branchCount)
	for index := range values {
		values[index] = fmt.Sprintf("branch%d", index)
	}
	ports := make([]networkingv1.NetworkPolicyPort, portCount)
	for index := range ports {
		port := intstr.FromInt(1000 + index)
		ports[index].Port = &port
	}
	peer := networkingv1.NetworkPolicyPeer{
		PodSelector: &metav1.LabelSelector{MatchLabels: labels},
		NamespaceSelector: &metav1.LabelSelector{MatchExpressions: []metav1.LabelSelectorRequirement{{
			Key: teamLabelKey, Operator: metav1.LabelSelectorOpIn, Values: values,
		}}},
	}
	policy := &networkingv1.NetworkPolicy{
		ObjectMeta: metav1.ObjectMeta{Name: "combined", Namespace: defaultNS},
		Spec:       networkingv1.NetworkPolicySpec{PolicyTypes: []networkingv1.PolicyType{direction}},
	}
	if direction == networkingv1.PolicyTypeIngress {
		policy.Spec.Ingress = []networkingv1.NetworkPolicyIngressRule{{Ports: ports, From: []networkingv1.NetworkPolicyPeer{peer}}}
	} else {
		policy.Spec.Egress = []networkingv1.NetworkPolicyEgressRule{{Ports: ports, To: []networkingv1.NetworkPolicyPeer{peer}}}
	}
	return policy
}

func TestCombinedPeerBudgetBeforeAllocation(t *testing.T) {
	for _, direction := range []networkingv1.PolicyType{networkingv1.PolicyTypeIngress, networkingv1.PolicyTypeEgress} {
		t.Run(string(direction), func(t *testing.T) {
			policy := combinedBudgetPolicy(direction, 20000, 16, 124)
			before := policy.DeepCopy()
			var translated *policies.NPMNetworkPolicy
			var err error
			allocations := testing.AllocsPerRun(3, func() {
				translated, err = TranslatePolicy(policy, false)
			})
			require.ErrorIs(t, err, ErrTooManySelectorMatches)
			require.Nil(t, translated)
			require.Less(t, allocations, float64(64), "rejection must happen before per-label sets or branch ACLs are allocated")
			require.Equal(t, before, policy)
		})
	}
}

func TestCombinedPeerPortReplicationBudget(t *testing.T) {
	for _, direction := range []networkingv1.PolicyType{networkingv1.PolicyTypeIngress, networkingv1.PolicyTypeEgress} {
		t.Run(string(direction), func(t *testing.T) {
			policy := combinedBudgetPolicy(direction, 10, 16, 124)
			translated, err := TranslatePolicy(policy, false)
			require.ErrorIs(t, err, ErrTooManyPolicyMatches)
			require.Nil(t, translated)

			control := combinedBudgetPolicy(direction, 4, 16, 124)
			translated, err = TranslatePolicy(control, false)
			require.NoError(t, err)
			require.Len(t, translated.ACLs, 1985)
		})
	}
}

func TestPolicyMatchBudgetAccumulatesAcrossRulesAndDirections(t *testing.T) {
	for _, dualDirection := range []bool{false, true} {
		t.Run(fmt.Sprintf("dualDirection=%t", dualDirection), func(t *testing.T) {
			policy := combinedBudgetPolicy(networkingv1.PolicyTypeIngress, 99, 1, 50)
			if dualDirection {
				policy.Spec.PolicyTypes = append(policy.Spec.PolicyTypes, networkingv1.PolicyTypeEgress)
				policy.Spec.Egress = combinedBudgetPolicy(networkingv1.PolicyTypeEgress, 99, 1, 50).Spec.Egress
			} else {
				policy.Spec.Ingress = append(policy.Spec.Ingress, policy.Spec.Ingress[0])
			}
			translated, err := TranslatePolicy(policy, false)
			require.ErrorIs(t, err, ErrTooManyPolicyMatches)
			require.Nil(t, translated)
		})
	}
}

func TestPolicyMatchBudgetExactBoundaryAndNamedPorts(t *testing.T) {
	budget := policyWorkBudget{}
	require.NoError(t, budget.reserve(10, maxSelectorMatches, nil, 0))
	require.Equal(t, maxTotalPolicyMatches, budget.matches)
	require.NoError(t, budget.reserve(1, 0, nil, 0), "a zero-match default drop still fits")
	before := budget
	require.ErrorIs(t, budget.reserve(1, 1, nil, 0), ErrTooManyPolicyMatches)
	require.Equal(t, before, budget, "rejection must not reserve partial work")

	namedPort := intstr.FromString("web")
	ports := []networkingv1.NetworkPolicyPort{{Port: &namedPort}}
	budget = policyWorkBudget{}
	require.NoError(t, budget.reserve(1, maxSelectorMatches-1, ports, 0))
	require.Equal(t, maxSelectorMatches, budget.matches)
	require.ErrorIs(t, budget.reserve(1, maxSelectorMatches, ports, 0), ErrTooManySelectorMatches)
}

func TestSelectedPodAndNestedMemberBudgets(t *testing.T) {
	policy := combinedBudgetPolicy(networkingv1.PolicyTypeIngress, 0, 1, 1)
	policy.Spec.PodSelector.MatchLabels = make(map[string]string, maxSelectorMatches)
	for index := 0; index < maxSelectorMatches; index++ {
		policy.Spec.PodSelector.MatchLabels[fmt.Sprintf("selected%d", index)] = "value"
	}
	translated, err := TranslatePolicy(policy, false)
	require.ErrorIs(t, err, ErrTooManySelectorMatches)
	require.ErrorContains(t, err, fmt.Sprintf("selected pod selector expands into %d matches including its namespace anchor, past the %d limit",
		maxSelectorMatches+1, maxSelectorMatches))
	require.NotContains(t, err.Error(), "namespaceSelector")
	require.Nil(t, translated)

	policy.Spec.PodSelector = metav1.LabelSelector{}
	values := make([]string, maxSelectorMatches+1)
	for index := range values {
		values[index] = fmt.Sprintf("value%d", index)
	}
	policy.Spec.Ingress[0].From[0].PodSelector = &metav1.LabelSelector{MatchExpressions: []metav1.LabelSelectorRequirement{{
		Key: "app", Operator: metav1.LabelSelectorOpIn, Values: values,
	}}}
	translated, err = TranslatePolicy(policy, false)
	require.ErrorIs(t, err, ErrTooManySelectorMatches)
	require.Nil(t, translated)
}
