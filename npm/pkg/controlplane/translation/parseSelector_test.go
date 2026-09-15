package translation

import (
	"fmt"
	"reflect"
	"testing"

	"github.com/stretchr/testify/require"
	networkingv1 "k8s.io/api/networking/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// tenantLabelKey is a shared label key used across the selector translation tests.
const tenantLabelKey = "tenant"

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
				t.Fatalf("unexpected operator %s", req.Operator)
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

// TestParsePodSelectorFailsClosed verifies that a podSelector matchExpression that Kubernetes
// would not admit fails closed instead of emitting a malformed set: an unsupported operator
// (which would otherwise leave a zero-value SetInfo) and an empty In/NotIn value list (which
// would otherwise produce a set with no members). This mirrors the namespaceSelector guard.
func TestParsePodSelectorFailsClosed(t *testing.T) {
	for _, test := range []struct {
		name string
		req  metav1.LabelSelectorRequirement
		err  error
	}{
		{"unsupported operator", metav1.LabelSelectorRequirement{Key: appLabelKey, Operator: "Superset", Values: []string{"a"}}, ErrUnsupportedMatchExpressionOperator},
		{"empty In", metav1.LabelSelectorRequirement{Key: appLabelKey, Operator: metav1.LabelSelectorOpIn}, ErrEmptyMatchExpressionValues},
		{"empty NotIn", metav1.LabelSelectorRequirement{Key: appLabelKey, Operator: metav1.LabelSelectorOpNotIn}, ErrEmptyMatchExpressionValues},
		{"invalid value", metav1.LabelSelectorRequirement{Key: appLabelKey, Operator: metav1.LabelSelectorOpIn, Values: []string{"bad value"}}, ErrInvalidMatchExpressionValues},
	} {
		t.Run(test.name, func(t *testing.T) {
			selector := &metav1.LabelSelector{MatchExpressions: []metav1.LabelSelectorRequirement{test.req}}
			parsed, err := parsePodSelector("ns/policy", selector)
			require.ErrorIs(t, err, test.err)
			require.Nil(t, parsed)
		})
	}
}

// TestTranslatePolicyPodSelectorFailsClosed verifies the podSelector guard surfaces through the
// full translation path for both the selected pods and a peer podSelector, in both directions.
func TestTranslatePolicyPodSelectorFailsClosed(t *testing.T) {
	bad := metav1.LabelSelectorRequirement{Key: appLabelKey, Operator: metav1.LabelSelectorOpIn} // empty In
	for _, direction := range []networkingv1.PolicyType{networkingv1.PolicyTypeIngress, networkingv1.PolicyTypeEgress} {
		for _, position := range []string{"selected", "peer"} {
			t.Run(string(direction)+"/"+position, func(t *testing.T) {
				policy := &networkingv1.NetworkPolicy{
					ObjectMeta: metav1.ObjectMeta{Name: "podsel", Namespace: defaultNS},
					Spec:       networkingv1.NetworkPolicySpec{PolicyTypes: []networkingv1.PolicyType{direction}},
				}
				badSelector := metav1.LabelSelector{MatchExpressions: []metav1.LabelSelectorRequirement{bad}}
				peer := networkingv1.NetworkPolicyPeer{PodSelector: &metav1.LabelSelector{MatchLabels: map[string]string{"role": "x"}}}
				if position == "selected" {
					policy.Spec.PodSelector = badSelector
				} else {
					peer.PodSelector = &badSelector
				}
				if direction == networkingv1.PolicyTypeIngress {
					policy.Spec.Ingress = []networkingv1.NetworkPolicyIngressRule{{From: []networkingv1.NetworkPolicyPeer{peer}}}
				} else {
					policy.Spec.Egress = []networkingv1.NetworkPolicyEgressRule{{To: []networkingv1.NetworkPolicyPeer{peer}}}
				}
				translated, err := TranslatePolicy(policy, false)
				require.ErrorIs(t, err, ErrEmptyMatchExpressionValues)
				require.Nil(t, translated)
			})
		}
	}
}
