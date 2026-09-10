package translation

import (
	"fmt"
	"reflect"
	"testing"

	"github.com/Azure/azure-container-networking/npm/pkg/dataplane/policies"
	"github.com/stretchr/testify/require"
	networkingv1 "k8s.io/api/networking/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
)

func TestFlattenNameSpaceSelectorCases(t *testing.T) {
	firstSelector := &metav1.LabelSelector{}

	testSelectors, err := flattenNameSpaceSelector(firstSelector)
	require.Nil(t, err)
	if len(testSelectors) != 1 {
		t.Errorf("TestFlattenNameSpaceSelectorCases failed @ 1st selector length check %+v", testSelectors)
	}

	var secondSelector *metav1.LabelSelector

	testSelectors, err = flattenNameSpaceSelector(secondSelector)
	require.Nil(t, err)
	if len(testSelectors) > 0 {
		t.Errorf("TestFlattenNameSpaceSelectorCases failed @ 1st selector length check %+v", testSelectors)
	}
}

func TestFlattenNameSpaceSelector(t *testing.T) {
	commonMatchLabel := map[string]string{
		"c": "d",
		"a": "b",
	}

	firstSelector := &metav1.LabelSelector{
		MatchExpressions: []metav1.LabelSelectorRequirement{
			{
				Key:      "testIn",
				Operator: metav1.LabelSelectorOpIn,
				Values: []string{
					"backend",
				},
			},
			{
				Key:      "pod",
				Operator: metav1.LabelSelectorOpIn,
				Values: []string{
					"a",
				},
			},
			{
				Key:      "testExists",
				Operator: metav1.LabelSelectorOpExists,
				Values:   []string{},
			},
			{
				Key:      "ns",
				Operator: metav1.LabelSelectorOpIn,
				Values: []string{
					"t",
				},
			},
		},
		MatchLabels: commonMatchLabel,
	}

	testSelectors, err := flattenNameSpaceSelector(firstSelector)
	require.Nil(t, err)
	if len(testSelectors) != 1 {
		t.Errorf("TestFlattenNameSpaceSelector failed @ 1st selector length check %+v", testSelectors)
	}

	if !reflect.DeepEqual(testSelectors[0], *firstSelector) {
		t.Errorf("TestFlattenNameSpaceSelector failed @ 1st selector deepEqual check.\n Expected: %+v \n Actual: %+v", *firstSelector, testSelectors[0])
	}

	secondSelector := &metav1.LabelSelector{
		MatchExpressions: []metav1.LabelSelectorRequirement{
			{
				Key:      "testIn",
				Operator: metav1.LabelSelectorOpIn,
				Values: []string{
					"backend",
					"frontend",
				},
			},
			{
				Key:      "pod",
				Operator: metav1.LabelSelectorOpIn,
				Values: []string{
					"a",
					"b",
				},
			},
			{
				Key:      "testExists",
				Operator: metav1.LabelSelectorOpExists,
				Values:   []string{},
			},
			{
				Key:      "ns",
				Operator: metav1.LabelSelectorOpIn,
				Values: []string{
					"t",
					"y",
				},
			},
		},
		MatchLabels: commonMatchLabel,
	}

	testSelectors, err = flattenNameSpaceSelector(secondSelector)
	require.Nil(t, err)
	if len(testSelectors) != 8 {
		t.Errorf("TestFlattenNameSpaceSelector failed @ 2nd selector length check %+v", testSelectors)
	}

	expectedSelectors := []metav1.LabelSelector{
		{
			MatchExpressions: []metav1.LabelSelectorRequirement{
				{
					Key:      "testExists",
					Operator: metav1.LabelSelectorOpExists,
					Values:   []string{},
				},
				{
					Key:      "testIn",
					Operator: metav1.LabelSelectorOpIn,
					Values: []string{
						"backend",
					},
				},
				{
					Key:      "pod",
					Operator: metav1.LabelSelectorOpIn,
					Values: []string{
						"a",
					},
				},
				{
					Key:      "ns",
					Operator: metav1.LabelSelectorOpIn,
					Values: []string{
						"t",
					},
				},
			},
			MatchLabels: commonMatchLabel,
		},
		{
			MatchExpressions: []metav1.LabelSelectorRequirement{
				{
					Key:      "testExists",
					Operator: metav1.LabelSelectorOpExists,
					Values:   []string{},
				},
				{
					Key:      "testIn",
					Operator: metav1.LabelSelectorOpIn,
					Values: []string{
						"backend",
					},
				},
				{
					Key:      "pod",
					Operator: metav1.LabelSelectorOpIn,
					Values: []string{
						"a",
					},
				},
				{
					Key:      "ns",
					Operator: metav1.LabelSelectorOpIn,
					Values: []string{
						"y",
					},
				},
			},
			MatchLabels: commonMatchLabel,
		},
		{
			MatchExpressions: []metav1.LabelSelectorRequirement{
				{
					Key:      "testExists",
					Operator: metav1.LabelSelectorOpExists,
					Values:   []string{},
				},
				{
					Key:      "testIn",
					Operator: metav1.LabelSelectorOpIn,
					Values: []string{
						"backend",
					},
				},
				{
					Key:      "pod",
					Operator: metav1.LabelSelectorOpIn,
					Values: []string{
						"b",
					},
				},
				{
					Key:      "ns",
					Operator: metav1.LabelSelectorOpIn,
					Values: []string{
						"t",
					},
				},
			},
			MatchLabels: commonMatchLabel,
		},
		{
			MatchExpressions: []metav1.LabelSelectorRequirement{
				{
					Key:      "testExists",
					Operator: metav1.LabelSelectorOpExists,
					Values:   []string{},
				},
				{
					Key:      "testIn",
					Operator: metav1.LabelSelectorOpIn,
					Values: []string{
						"backend",
					},
				},
				{
					Key:      "pod",
					Operator: metav1.LabelSelectorOpIn,
					Values: []string{
						"b",
					},
				},
				{
					Key:      "ns",
					Operator: metav1.LabelSelectorOpIn,
					Values: []string{
						"y",
					},
				},
			},
			MatchLabels: commonMatchLabel,
		},
		{
			MatchExpressions: []metav1.LabelSelectorRequirement{
				{
					Key:      "testExists",
					Operator: metav1.LabelSelectorOpExists,
					Values:   []string{},
				},
				{
					Key:      "testIn",
					Operator: metav1.LabelSelectorOpIn,
					Values: []string{
						"frontend",
					},
				},
				{
					Key:      "pod",
					Operator: metav1.LabelSelectorOpIn,
					Values: []string{
						"a",
					},
				},
				{
					Key:      "ns",
					Operator: metav1.LabelSelectorOpIn,
					Values: []string{
						"t",
					},
				},
			},
			MatchLabels: commonMatchLabel,
		},
		{
			MatchExpressions: []metav1.LabelSelectorRequirement{
				{
					Key:      "testExists",
					Operator: metav1.LabelSelectorOpExists,
					Values:   []string{},
				},
				{
					Key:      "testIn",
					Operator: metav1.LabelSelectorOpIn,
					Values: []string{
						"frontend",
					},
				},
				{
					Key:      "pod",
					Operator: metav1.LabelSelectorOpIn,
					Values: []string{
						"a",
					},
				},
				{
					Key:      "ns",
					Operator: metav1.LabelSelectorOpIn,
					Values: []string{
						"y",
					},
				},
			},
			MatchLabels: commonMatchLabel,
		},
		{
			MatchExpressions: []metav1.LabelSelectorRequirement{
				{
					Key:      "testExists",
					Operator: metav1.LabelSelectorOpExists,
					Values:   []string{},
				},
				{
					Key:      "testIn",
					Operator: metav1.LabelSelectorOpIn,
					Values: []string{
						"frontend",
					},
				},
				{
					Key:      "pod",
					Operator: metav1.LabelSelectorOpIn,
					Values: []string{
						"b",
					},
				},
				{
					Key:      "ns",
					Operator: metav1.LabelSelectorOpIn,
					Values: []string{
						"t",
					},
				},
			},
			MatchLabels: commonMatchLabel,
		},
		{
			MatchExpressions: []metav1.LabelSelectorRequirement{
				{
					Key:      "testExists",
					Operator: metav1.LabelSelectorOpExists,
					Values:   []string{},
				},
				{
					Key:      "testIn",
					Operator: metav1.LabelSelectorOpIn,
					Values: []string{
						"frontend",
					},
				},
				{
					Key:      "pod",
					Operator: metav1.LabelSelectorOpIn,
					Values: []string{
						"b",
					},
				},
				{
					Key:      "ns",
					Operator: metav1.LabelSelectorOpIn,
					Values: []string{
						"y",
					},
				},
			},
			MatchLabels: commonMatchLabel,
		},
	}

	if !reflect.DeepEqual(expectedSelectors, testSelectors) {
		t.Errorf("TestFlattenNameSpaceSelector failed @ 2nd selector deepEqual check.\n Expected: %+v \n Actual: %+v", expectedSelectors, testSelectors)
	}
}

func TestFlattenNameSpaceSelectorWoMatchLabels(t *testing.T) {
	firstSelector := &metav1.LabelSelector{
		MatchExpressions: []metav1.LabelSelectorRequirement{
			{
				Key:      "testIn",
				Operator: metav1.LabelSelectorOpIn,
				Values: []string{
					"backend",
				},
			},
			{
				Key:      "pod",
				Operator: metav1.LabelSelectorOpIn,
				Values: []string{
					"a",
				},
			},
			{
				Key:      "testExists",
				Operator: metav1.LabelSelectorOpExists,
				Values:   []string{},
			},
			{
				Key:      "ns",
				Operator: metav1.LabelSelectorOpIn,
				Values: []string{
					"t",
					"y",
				},
			},
		},
	}

	testSelectors, err := flattenNameSpaceSelector(firstSelector)
	require.Nil(t, err)
	if len(testSelectors) != 2 {
		t.Errorf("TestFlattenNameSpaceSelector failed @ 1st selector length check %+v", testSelectors)
	}

	expectedSelectors := []metav1.LabelSelector{
		{
			MatchExpressions: []metav1.LabelSelectorRequirement{
				{
					Key:      "testIn",
					Operator: metav1.LabelSelectorOpIn,
					Values: []string{
						"backend",
					},
				},
				{
					Key:      "pod",
					Operator: metav1.LabelSelectorOpIn,
					Values: []string{
						"a",
					},
				},
				{
					Key:      "testExists",
					Operator: metav1.LabelSelectorOpExists,
					Values:   []string{},
				},
				{
					Key:      "ns",
					Operator: metav1.LabelSelectorOpIn,
					Values: []string{
						"t",
					},
				},
			},
		},
		{
			MatchExpressions: []metav1.LabelSelectorRequirement{
				{
					Key:      "testIn",
					Operator: metav1.LabelSelectorOpIn,
					Values: []string{
						"backend",
					},
				},
				{
					Key:      "pod",
					Operator: metav1.LabelSelectorOpIn,
					Values: []string{
						"a",
					},
				},
				{
					Key:      "testExists",
					Operator: metav1.LabelSelectorOpExists,
					Values:   []string{},
				},
				{
					Key:      "ns",
					Operator: metav1.LabelSelectorOpIn,
					Values: []string{
						"y",
					},
				},
			},
		},
	}

	if !reflect.DeepEqual(testSelectors, expectedSelectors) {
		t.Errorf("TestFlattenNameSpaceSelector failed @ 1st selector deepEqual check.\n Expected: %+v \n Actual: %+v", expectedSelectors, testSelectors)
	}
}

func TestFlattenNamespaceSelectorError(t *testing.T) {
	tests := []struct {
		name     string
		selector *metav1.LabelSelector
		wantErr  bool
	}{
		{
			name: "good alphanumeric with hyphen",
			selector: &metav1.LabelSelector{
				MatchExpressions: []metav1.LabelSelectorRequirement{
					{
						Key:      "testIn",
						Operator: metav1.LabelSelectorOpIn,
						Values: []string{
							"good",
							"good-1",
							"good2-too",
						},
					},
					{
						Key:      "testNotIn",
						Operator: metav1.LabelSelectorOpNotIn,
						Values: []string{
							"good",
							"good-1",
							"good2-too",
						},
					},
				},
			},
			wantErr: false,
		},
		{
			name: "bad in",
			selector: &metav1.LabelSelector{
				MatchExpressions: []metav1.LabelSelectorRequirement{
					{
						Key:      "testIn",
						Operator: metav1.LabelSelectorOpIn,
						Values: []string{
							"good-1",
							"bad$",
						},
					},
				},
			},
			wantErr: true,
		},
		{
			name: "bad not in",
			selector: &metav1.LabelSelector{
				MatchExpressions: []metav1.LabelSelectorRequirement{
					{
						Key:      "testNotIn",
						Operator: metav1.LabelSelectorOpIn,
						Values: []string{
							"bad$",
							"good-1",
						},
					},
				},
			},
			wantErr: true,
		},
		{
			name: "good and bad",
			selector: &metav1.LabelSelector{
				MatchExpressions: []metav1.LabelSelectorRequirement{
					{
						Key:      "testIn",
						Operator: metav1.LabelSelectorOpIn,
						Values: []string{
							"good-1",
						},
					},
					{
						Key:      "testNotIn",
						Operator: metav1.LabelSelectorOpIn,
						Values: []string{
							"bad$",
							"good-1",
						},
					},
				},
			},
			wantErr: true,
		},
		{
			name: "bad with space",
			selector: &metav1.LabelSelector{
				MatchExpressions: []metav1.LabelSelectorRequirement{
					{
						Key:      "testIn",
						Operator: metav1.LabelSelectorOpIn,
						Values: []string{
							"bad space",
							"good-1",
						},
					},
				},
			},
			wantErr: true,
		},
	}

	for i, tt := range tests {
		tt := tt
		t.Run(fmt.Sprintf("test %d", i), func(t *testing.T) {
			s, err := flattenNameSpaceSelector(tt.selector)
			if tt.wantErr {
				require.Error(t, err)
				require.Nil(t, s)
			} else {
				require.NoError(t, err)
				require.NotNil(t, s)
			}
		})
	}
}

// TestFlattenNameSpaceSelectorMultiValueNotIn verifies that a multi-value NotIn
// requirement is preserved as a single conjunction rather than fanned out into
// separate selectors. Separate selectors would become independent additive allow
// rules, so a namespace carrying one excluded value could still match the rule
// negating a different value.
func TestFlattenNameSpaceSelectorMultiValueNotIn(t *testing.T) {
	selector := &metav1.LabelSelector{
		MatchExpressions: []metav1.LabelSelectorRequirement{
			{
				Key:      tenantLabelKey,
				Operator: metav1.LabelSelectorOpNotIn,
				Values:   []string{"x", "y"},
			},
		},
	}

	testSelectors, err := flattenNameSpaceSelector(selector)
	require.NoError(t, err)

	expected := []metav1.LabelSelector{
		{
			MatchExpressions: []metav1.LabelSelectorRequirement{
				{
					Key:      tenantLabelKey,
					Operator: metav1.LabelSelectorOpNotIn,
					Values:   []string{"x"},
				},
				{
					Key:      tenantLabelKey,
					Operator: metav1.LabelSelectorOpNotIn,
					Values:   []string{"y"},
				},
			},
		},
	}

	require.Equal(t, expected, testSelectors)
}

// TestFlattenNameSpaceSelectorMixedInAndNotIn verifies that multi-value In values
// fan out into disjunctive branches while every multi-value NotIn exclusion is
// carried conjunctively into each branch.
func TestFlattenNameSpaceSelectorMixedInAndNotIn(t *testing.T) {
	selector := &metav1.LabelSelector{
		MatchExpressions: []metav1.LabelSelectorRequirement{
			{
				Key:      tenantLabelKey,
				Operator: metav1.LabelSelectorOpNotIn,
				Values:   []string{"x", "y"},
			},
			{
				Key:      "role",
				Operator: metav1.LabelSelectorOpIn,
				Values:   []string{"a", "b"},
			},
		},
	}

	testSelectors, err := flattenNameSpaceSelector(selector)
	require.NoError(t, err)

	// Two In branches, each carrying both NotIn exclusions conjunctively.
	require.Len(t, testSelectors, 2)
	for _, s := range testSelectors {
		var notInValues []string
		var inValues []string
		for _, req := range s.MatchExpressions {
			require.Len(t, req.Values, 1, "every requirement must be single-value after flatten")
			switch req.Operator {
			case metav1.LabelSelectorOpNotIn:
				require.Equal(t, tenantLabelKey, req.Key)
				notInValues = append(notInValues, req.Values[0])
			case metav1.LabelSelectorOpIn:
				require.Equal(t, "role", req.Key)
				inValues = append(inValues, req.Values[0])
			case metav1.LabelSelectorOpExists, metav1.LabelSelectorOpDoesNotExist:
				t.Fatalf("unexpected valueless operator %s", req.Operator)
			default:
				t.Fatalf("unexpected operator %s", req.Operator)
			}
		}
		require.ElementsMatch(t, []string{"x", "y"}, notInValues, "both exclusions must be present in every branch")
		require.Len(t, inValues, 1)
	}
}

// TestFlattenNameSpaceSelectorUnsupportedOperator verifies that a matchExpression with
// an operator other than In/NotIn/Exists/DoesNotExist is rejected (fail closed) rather
// than silently dropped, which could otherwise widen the selector.
func TestFlattenNameSpaceSelectorUnsupportedOperator(t *testing.T) {
	selector := &metav1.LabelSelector{
		MatchExpressions: []metav1.LabelSelectorRequirement{
			{
				Key:      tenantLabelKey,
				Operator: metav1.LabelSelectorOperator("Frobnicate"),
				Values:   []string{"x"},
			},
		},
	}
	s, err := flattenNameSpaceSelector(selector)
	require.ErrorIs(t, err, ErrUnsupportedMatchExpressionOperator)
	require.Nil(t, s)
}

// TestFlattenNameSpaceSelectorEmptyValues verifies that In/NotIn requirements with
// no values are rejected (fail closed) rather than silently dropped, which could
// otherwise widen a selector or produce no rules at all.
func TestFlattenNameSpaceSelectorEmptyValues(t *testing.T) {
	for _, op := range []metav1.LabelSelectorOperator{metav1.LabelSelectorOpIn, metav1.LabelSelectorOpNotIn} {
		selector := &metav1.LabelSelector{
			MatchExpressions: []metav1.LabelSelectorRequirement{
				{
					Key:      tenantLabelKey,
					Operator: op,
					Values:   []string{},
				},
			},
		}
		s, err := flattenNameSpaceSelector(selector)
		require.ErrorIs(t, err, ErrEmptyMatchExpressionValues, "operator %s", op)
		require.Nil(t, s)
	}
}

// TestFlattenNameSpaceSelectorExpansionLimit verifies that a namespaceSelector whose
// multi-value In requirements would expand into more selectors than NPM is willing to
// translate is rejected before any allocation. Each flattened selector is deep-copied and
// later becomes its own IPSet and ACL, and the count is the product of the value counts,
// so an unbounded selector exhausts memory on every node running NPM.
func TestFlattenNameSpaceSelectorExpansionLimit(t *testing.T) {
	twoValueReqs := func(n int) []metav1.LabelSelectorRequirement {
		reqs := make([]metav1.LabelSelectorRequirement, 0, n)
		for i := 0; i < n; i++ {
			reqs = append(reqs, metav1.LabelSelectorRequirement{
				Key:      fmt.Sprintf("key%d", i),
				Operator: metav1.LabelSelectorOpIn,
				Values:   []string{"a", "b"},
			})
		}
		return reqs
	}

	// 2^9 = 512 selectors is under the limit and must still translate.
	under := &metav1.LabelSelector{MatchExpressions: twoValueReqs(9)}
	selectors, err := flattenNameSpaceSelector(under)
	require.NoError(t, err)
	require.Len(t, selectors, 512)

	// 2^19 = 524288 selectors is the reported exhaustion case and must be rejected.
	over := &metav1.LabelSelector{MatchExpressions: twoValueReqs(19)}
	selectors, err = flattenNameSpaceSelector(over)
	require.ErrorIs(t, err, ErrTooManyFlattenedSelectors)
	require.Nil(t, selectors)

	// A single requirement wider than the limit is rejected on the first iteration,
	// so the guard cannot be sidestepped by using one very wide requirement.
	values := make([]string, maxFlattenedNSSelectors+1)
	for i := range values {
		values[i] = fmt.Sprintf("v%d", i)
	}
	wide := &metav1.LabelSelector{
		MatchExpressions: []metav1.LabelSelectorRequirement{
			{Key: "key", Operator: metav1.LabelSelectorOpIn, Values: values},
		},
	}
	selectors, err = flattenNameSpaceSelector(wide)
	require.ErrorIs(t, err, ErrTooManyFlattenedSelectors)
	require.Nil(t, selectors)
}

// TestTranslatePolicyExpansionLimit verifies the expansion guard surfaces through the full
// translation path rather than being swallowed, so an oversized policy is rejected instead
// of being expanded.
func TestTranslatePolicyExpansionLimit(t *testing.T) {
	reqs := make([]metav1.LabelSelectorRequirement, 0, 19)
	for i := 0; i < 19; i++ {
		reqs = append(reqs, metav1.LabelSelectorRequirement{
			Key:      fmt.Sprintf("key%d", i),
			Operator: metav1.LabelSelectorOpIn,
			Values:   []string{"a", "b"},
		})
	}

	pol := &networkingv1.NetworkPolicy{
		ObjectMeta: metav1.ObjectMeta{Name: "expand", Namespace: defaultNS},
		Spec: networkingv1.NetworkPolicySpec{
			PodSelector: metav1.LabelSelector{},
			PolicyTypes: []networkingv1.PolicyType{networkingv1.PolicyTypeIngress},
			Ingress: []networkingv1.NetworkPolicyIngressRule{
				{
					From: []networkingv1.NetworkPolicyPeer{
						{NamespaceSelector: &metav1.LabelSelector{MatchExpressions: reqs}},
					},
				},
			},
		},
	}

	npmNetPol, err := TranslatePolicy(pol, false)
	require.ErrorIs(t, err, ErrTooManyFlattenedSelectors)
	require.Nil(t, npmNetPol)
}

func TestIsValidLabel(t *testing.T) {
	good := []string{
		"",
		"1",
		"abc",
		"ABC",
		"abc1",
		"ABC1",
		"abc-1",
		"ABC-1",
		"ABC_1",
		"ABC_-a54--f",
	}

	for _, g := range good {
		require.True(t, isValidLabelValue(g), "string was [%s]", g)
	}

	bad := []string{
		"-",
		"_",
		"$",
		" ",
		"abc-",
		"abc$",
		"abc$123",
		"bad space",
		"end-with-hyphen-",
		"end-with-underscore_",
		"end-with-space ",
	}

	for _, b := range bad {
		require.False(t, isValidLabelValue(b), "string was [%s]", b)
	}
}

// TestTranslatePolicyACLBudget covers the multiplication the selector cap alone does not
// catch. Every flattened namespaceSelector branch is emitted once per port in the rule, so a
// policy whose selector expansion is comfortably under the selector limit can still generate
// an enormous number of ACLs by listing many ports. Each ACL becomes an iptables rule.
func TestTranslatePolicyACLBudget(t *testing.T) {
	// 2^9 = 512 flattened selectors: under maxFlattenedNSSelectors.
	reqs := make([]metav1.LabelSelectorRequirement, 0, 9)
	for i := 0; i < 9; i++ {
		reqs = append(reqs, metav1.LabelSelectorRequirement{
			Key:      fmt.Sprintf("key%d", i),
			Operator: metav1.LabelSelectorOpIn,
			Values:   []string{"a", "b"},
		})
	}

	// Sanity: the selector expansion on its own is accepted.
	flattened, err := flattenNameSpaceSelector(&metav1.LabelSelector{MatchExpressions: reqs})
	require.NoError(t, err)
	require.Len(t, flattened, 512)

	// 512 selectors x 512 ports would be 262144 ACLs.
	ports := make([]networkingv1.NetworkPolicyPort, 0, 512)
	for i := 0; i < 512; i++ {
		p := intstr.FromInt(1000 + i)
		ports = append(ports, networkingv1.NetworkPolicyPort{Port: &p})
	}

	pol := &networkingv1.NetworkPolicy{
		ObjectMeta: metav1.ObjectMeta{Name: "expand", Namespace: defaultNS},
		Spec: networkingv1.NetworkPolicySpec{
			PodSelector: metav1.LabelSelector{},
			PolicyTypes: []networkingv1.PolicyType{networkingv1.PolicyTypeIngress},
			Ingress: []networkingv1.NetworkPolicyIngressRule{{
				Ports: ports,
				From: []networkingv1.NetworkPolicyPeer{
					{NamespaceSelector: &metav1.LabelSelector{MatchExpressions: reqs}},
				},
			}},
		},
	}

	npmNetPol, err := TranslatePolicy(pol, false)
	require.ErrorIs(t, err, ErrTooManyACLs,
		"a policy that multiplies selectors by ports must be rejected even when the selector count is under its own limit")
	require.Nil(t, npmNetPol)
}

// TestTranslatePolicyOrdinaryPolicyWithinACLBudget guards the budget against false positives:
// a normal policy with several peers and ports must translate unaffected.
func TestTranslatePolicyOrdinaryPolicyWithinACLBudget(t *testing.T) {
	ports := make([]networkingv1.NetworkPolicyPort, 0, 8)
	for i := 0; i < 8; i++ {
		p := intstr.FromInt(8000 + i)
		ports = append(ports, networkingv1.NetworkPolicyPort{Port: &p})
	}

	pol := &networkingv1.NetworkPolicy{
		ObjectMeta: metav1.ObjectMeta{Name: "normal", Namespace: defaultNS},
		Spec: networkingv1.NetworkPolicySpec{
			PodSelector: metav1.LabelSelector{MatchLabels: map[string]string{"app": "web"}},
			PolicyTypes: []networkingv1.PolicyType{networkingv1.PolicyTypeIngress},
			Ingress: []networkingv1.NetworkPolicyIngressRule{{
				Ports: ports,
				From: []networkingv1.NetworkPolicyPeer{
					{NamespaceSelector: &metav1.LabelSelector{MatchLabels: map[string]string{teamLabelKey: teamBlueValue}}},
					{PodSelector: &metav1.LabelSelector{MatchLabels: map[string]string{"role": "client"}}},
					{IPBlock: &networkingv1.IPBlock{CIDR: "10.0.0.0/8"}},
				},
			}},
		},
	}

	npmNetPol, err := TranslatePolicy(pol, false)
	require.NoError(t, err)
	require.NotNil(t, npmNetPol)
	require.Less(t, len(npmNetPol.ACLs), maxACLsPerPolicy)
}

// TestTranslatePolicyExactlyAtACLLimit guards the boundary. The per-append guard is asked
// whether there is room for one more ACL, so it must refuse at the ceiling; the check on the
// finished policy is asked whether the policy is past the ceiling, so it must admit a policy
// that lands exactly on it. Using the same comparison for both would reject a policy of
// exactly maxACLsPerPolicy rules.
func TestTranslatePolicyExactlyAtACLLimit(t *testing.T) {
	// one ACL per port, plus the default drop the policy implies.
	portCount := maxACLsPerPolicy - 1
	ports := make([]networkingv1.NetworkPolicyPort, 0, portCount)
	for i := 0; i < portCount; i++ {
		p := intstr.FromInt(1 + i)
		ports = append(ports, networkingv1.NetworkPolicyPort{Port: &p})
	}

	pol := &networkingv1.NetworkPolicy{
		ObjectMeta: metav1.ObjectMeta{Name: "at-limit", Namespace: defaultNS},
		Spec: networkingv1.NetworkPolicySpec{
			PodSelector: metav1.LabelSelector{},
			PolicyTypes: []networkingv1.PolicyType{networkingv1.PolicyTypeIngress},
			Ingress:     []networkingv1.NetworkPolicyIngressRule{{Ports: ports}},
		},
	}

	npmNetPol, err := TranslatePolicy(pol, false)
	require.NoError(t, err, "a policy landing exactly on the ceiling must translate")
	require.NotNil(t, npmNetPol)
	require.Len(t, npmNetPol.ACLs, maxACLsPerPolicy)
}

// TestPortOnlyRuleBudgetStopsWithinPortLoop covers a rule that lists ports and no peers. That
// path emits one ACL per port with no peer expansion to bound it, so the budget has to be
// checked inside its loop rather than only by the backstop at the end of translation.
func TestPortOnlyRuleBudgetStopsWithinPortLoop(t *testing.T) {
	portCount := maxACLsPerPolicy * 2
	ports := make([]networkingv1.NetworkPolicyPort, 0, portCount)
	for i := 0; i < portCount; i++ {
		p := intstr.FromInt(1 + i)
		ports = append(ports, networkingv1.NetworkPolicyPort{Port: &p})
	}

	npmNetPol := policies.NewNPMNetworkPolicy("port-only", defaultNS)
	err := checkOnlyPortRuleExists(true, false, false, ports, false, policies.Ingress, npmNetPol)
	require.ErrorIs(t, err, ErrTooManyACLs)
	require.LessOrEqual(t, len(npmNetPol.ACLs), maxACLsPerPolicy,
		"a rule with only ports must stop once the budget is spent")
}

// TestPeerAndPortRuleBudgetStopsWithinPortLoop covers a single peer listing more ports than
// the budget allows. One peer emits one ACL per port, so a budget checked only on entry to
// peerAndPortRule would let that peer materialize every ACL before anything noticed.
func TestPeerAndPortRuleBudgetStopsWithinPortLoop(t *testing.T) {
	portCount := maxACLsPerPolicy * 2
	ports := make([]networkingv1.NetworkPolicyPort, 0, portCount)
	for i := 0; i < portCount; i++ {
		p := intstr.FromInt(1 + i)
		ports = append(ports, networkingv1.NetworkPolicyPort{Port: &p})
	}

	npmNetPol := policies.NewNPMNetworkPolicy("wide-ports", defaultNS)
	err := peerAndPortRule(npmNetPol, policies.Ingress, ports, []policies.SetInfo{}, false)
	require.ErrorIs(t, err, ErrTooManyACLs)
	require.LessOrEqual(t, len(npmNetPol.ACLs), maxACLsPerPolicy,
		"the port loop must stop once the budget is spent instead of emitting an ACL for every port")
}

// TestNotInValuesAreBounded covers a long NotIn list. Compiling it as one conjunction keeps it
// out of the flattened-selector count and out of the rule budget, because it stays a single
// selector producing a single rule, but every value still becomes its own IPSet and its own
// condition on that rule. The match bound is what stops it.
func TestNotInValuesAreBounded(t *testing.T) {
	values := make([]string, 0, maxSelectorMatches+1)
	for i := 0; i <= maxSelectorMatches; i++ {
		values = append(values, fmt.Sprintf("v%d", i))
	}

	selector := &metav1.LabelSelector{
		MatchExpressions: []metav1.LabelSelectorRequirement{
			{Key: tenantLabelKey, Operator: metav1.LabelSelectorOpNotIn, Values: values},
		},
	}

	flattened, err := flattenNameSpaceSelector(selector)
	require.ErrorIs(t, err, ErrTooManySelectorMatches,
		"a NotIn list past the match bound must be refused")
	require.Nil(t, flattened)

	// the same policy is refused end to end, so no partial rules are installed
	pol := &networkingv1.NetworkPolicy{
		ObjectMeta: metav1.ObjectMeta{Name: "wide-notin", Namespace: defaultNS},
		Spec: networkingv1.NetworkPolicySpec{
			PodSelector: metav1.LabelSelector{},
			PolicyTypes: []networkingv1.PolicyType{networkingv1.PolicyTypeIngress},
			Ingress: []networkingv1.NetworkPolicyIngressRule{{
				From: []networkingv1.NetworkPolicyPeer{{NamespaceSelector: selector}},
			}},
		},
	}
	npmNetPol, err := TranslatePolicy(pol, false)
	require.ErrorIs(t, err, ErrTooManySelectorMatches)
	require.Nil(t, npmNetPol)
}

// TestNotInValuesAtTheBoundAreAccepted keeps the bound from rejecting a selector that sits
// exactly on it, and guards the ordinary small NotIn that real policies use.
func TestNotInValuesAtTheBoundAreAccepted(t *testing.T) {
	values := make([]string, 0, maxSelectorMatches)
	for i := 0; i < maxSelectorMatches; i++ {
		values = append(values, fmt.Sprintf("v%d", i))
	}

	flattened, err := flattenNameSpaceSelector(&metav1.LabelSelector{
		MatchExpressions: []metav1.LabelSelectorRequirement{
			{Key: tenantLabelKey, Operator: metav1.LabelSelectorOpNotIn, Values: values},
		},
	})
	require.NoError(t, err, "a selector exactly on the bound must translate")
	require.Len(t, flattened, 1, "a NotIn stays a single conjunction")
	require.Len(t, flattened[0].MatchExpressions, maxSelectorMatches)
}

// TestMatchLabelsOnlySelectorIsBounded covers a selector that carries only matchLabels. It
// takes a shortcut past the expression handling, but each label still becomes its own match,
// so the bound has to be applied before that shortcut.
func TestMatchLabelsOnlySelectorIsBounded(t *testing.T) {
	labels := make(map[string]string, maxSelectorMatches+1)
	for i := 0; i <= maxSelectorMatches; i++ {
		labels[fmt.Sprintf("key%d", i)] = "v"
	}

	flattened, err := flattenNameSpaceSelector(&metav1.LabelSelector{MatchLabels: labels})
	require.ErrorIs(t, err, ErrTooManySelectorMatches,
		"a matchLabels-only selector past the bound must be refused")
	require.Nil(t, flattened)

	// an ordinary selector is untouched
	ok, err := flattenNameSpaceSelector(&metav1.LabelSelector{
		MatchLabels: map[string]string{"team": teamBlueValue},
	})
	require.NoError(t, err)
	require.Len(t, ok, 1)
}
