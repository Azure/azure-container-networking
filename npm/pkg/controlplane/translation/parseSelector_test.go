package translation

import (
	"reflect"
	"testing"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestFlattenNameSpaceSelectorCases(t *testing.T) {
	firstSelector := &metav1.LabelSelector{}

	testSelectors := flattenNameSpaceSelector(firstSelector)
	if len(testSelectors) != 1 {
		t.Errorf("TestFlattenNameSpaceSelectorCases failed @ 1st selector length check %+v", testSelectors)
	}

	var secondSelector *metav1.LabelSelector

	testSelectors = flattenNameSpaceSelector(secondSelector)
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

	testSelectors := flattenNameSpaceSelector(firstSelector)
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

	testSelectors = flattenNameSpaceSelector(secondSelector)
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

	testSelectors := flattenNameSpaceSelector(firstSelector)
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
