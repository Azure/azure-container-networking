package translation

import (
	"fmt"
	"reflect"
	"testing"

	"github.com/stretchr/testify/require"
	networkingv1 "k8s.io/api/networking/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
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
				Key:      "tenant",
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
					Key:      "tenant",
					Operator: metav1.LabelSelectorOpNotIn,
					Values:   []string{"x"},
				},
				{
					Key:      "tenant",
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
				Key:      "tenant",
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
				require.Equal(t, "tenant", req.Key)
				notInValues = append(notInValues, req.Values[0])
			case metav1.LabelSelectorOpIn:
				require.Equal(t, "role", req.Key)
				inValues = append(inValues, req.Values[0])
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
				Key:      "tenant",
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
					Key:      "tenant",
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
		ObjectMeta: metav1.ObjectMeta{Name: "expand", Namespace: "default"},
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
