package npm

import (
	"container/heap"
	"reflect"
	"testing"

	"github.com/Azure/azure-container-networking/npm/util"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestParseLabel(t *testing.T) {
	label, isComplementSet := ParseLabel("test:frontend")
	expectedLabel := "test:frontend"
	if isComplementSet || label != expectedLabel {
		t.Errorf("TestParseLabel failed @ label %s", label)
	}

	label, isComplementSet = ParseLabel("!test:frontend")
	expectedLabel = "test:frontend"
	if !isComplementSet || label != expectedLabel {
		t.Errorf("TestParseLabel failed @ label %s", label)
	}

	label, isComplementSet = ParseLabel("test")
	expectedLabel = "test"
	if isComplementSet || label != expectedLabel {
		t.Errorf("TestParseLabel failed @ label %s", label)
	}

	label, isComplementSet = ParseLabel("!test")
	expectedLabel = "test"
	if !isComplementSet || label != expectedLabel {
		t.Errorf("TestParseLabel failed @ label %s", label)
	}

	label, isComplementSet = ParseLabel("!!test")
	expectedLabel = "!test"
	if !isComplementSet || label != expectedLabel {
		t.Errorf("TestParseLabel failed @ label %s", label)
	}

	label, isComplementSet = ParseLabel("test:!frontend")
	expectedLabel = "test:!frontend"
	if isComplementSet || label != expectedLabel {
		t.Errorf("TestParseLabel failed @ label %s", label)
	}

	label, isComplementSet = ParseLabel("!test:!frontend")
	expectedLabel = "test:!frontend"
	if !isComplementSet || label != expectedLabel {
		t.Errorf("TestParseLabel failed @ label %s", label)
	}
}

func TestGetOperatorAndLabel(t *testing.T) {
	testLabels := []string{
		"a",
		"k:v",
		"",
		"!a:b",
		"!a",
	}

	resultOperators, resultLabels := []string{}, []string{}
	for _, testLabel := range testLabels {
		resultOperator, resultLabel := GetOperatorAndLabel(testLabel)
		resultOperators = append(resultOperators, resultOperator)
		resultLabels = append(resultLabels, resultLabel)
	}

	expectedOperators := []string{
		"",
		"",
		"",
		util.IptablesNotFlag,
		util.IptablesNotFlag,
	}

	expectedLabels := []string{
		"a",
		"k:v",
		"",
		"a:b",
		"a",
	}

	if !reflect.DeepEqual(resultOperators, expectedOperators) {
		t.Errorf("TestGetOperatorAndLabel failed @ operator comparison")
	}

	if !reflect.DeepEqual(resultLabels, expectedLabels) {
		t.Errorf("TestGetOperatorAndLabel failed @ label comparison")
	}
}

func TestGetOperatorsAndLabels(t *testing.T) {
	testLabels := []string{
		"k:v",
		"",
		"!a:b",
	}

	resultOps, resultLabels := GetOperatorsAndLabels(testLabels)
	expectedOps := []string{
		"",
		"",
		"!",
	}
	expectedLabels := []string{
		"k:v",
		"",
		"a:b",
	}

	if !reflect.DeepEqual(resultOps, expectedOps) {
		t.Errorf("TestGetOperatorsAndLabels failed @ op comparision")
	}

	if !reflect.DeepEqual(resultLabels, expectedLabels) {
		t.Errorf("TestGetOperatorsAndLabels failed @ label comparision")
	}
}

func TestReqHeap(t *testing.T) {
	reqHeap := &ReqHeap{
		metav1.LabelSelectorRequirement{
			Key:      "testIn",
			Operator: metav1.LabelSelectorOpIn,
			Values: []string{
				"frontend",
				"backend",
			},
		},
		metav1.LabelSelectorRequirement{
			Key:      "a",
			Operator: metav1.LabelSelectorOpIn,
			Values:   []string{},
		},
		metav1.LabelSelectorRequirement{
			Key:      "testIn",
			Operator: metav1.LabelSelectorOpIn,
			Values: []string{
				"b",
				"a",
			},
		},
	}

	heap.Init(reqHeap)
	heap.Push(
		reqHeap,
		metav1.LabelSelectorRequirement{
			Key:      "testIn",
			Operator: metav1.LabelSelectorOpIn,
			Values: []string{
				"a",
			},
		},
	)

	expectedReqHeap := &ReqHeap{
		metav1.LabelSelectorRequirement{
			Key:      "a",
			Operator: metav1.LabelSelectorOpIn,
			Values:   []string{},
		},
		metav1.LabelSelectorRequirement{
			Key:      "testIn",
			Operator: metav1.LabelSelectorOpIn,
			Values: []string{
				"a",
			},
		},
		metav1.LabelSelectorRequirement{
			Key:      "testIn",
			Operator: metav1.LabelSelectorOpIn,
			Values: []string{
				"a",
				"b",
			},
		},
		metav1.LabelSelectorRequirement{
			Key:      "testIn",
			Operator: metav1.LabelSelectorOpIn,
			Values: []string{
				"backend",
				"frontend",
			},
		},
	}

	if !reflect.DeepEqual(reqHeap, expectedReqHeap) {
		t.Errorf("TestReqHeap failed @ heap comparison")
		t.Errorf("reqHeap: %v", reqHeap)
		t.Errorf("expectedReqHeap: %v", expectedReqHeap)
	}
}

func TestSortSelector(t *testing.T) {
	selector := &metav1.LabelSelector{
		MatchExpressions: []metav1.LabelSelectorRequirement{
			metav1.LabelSelectorRequirement{
				Key:      "testIn",
				Operator: metav1.LabelSelectorOpIn,
				Values: []string{
					"frontend",
					"backend",
				},
			},
			metav1.LabelSelectorRequirement{
				Key:      "a",
				Operator: metav1.LabelSelectorOpIn,
				Values: []string{
					"b",
				},
			},
		},
		MatchLabels: map[string]string{
			"c": "d",
			"a": "b",
		},
	}

	sortSelector(selector)
	expectedSelector := &metav1.LabelSelector{
		MatchExpressions: []metav1.LabelSelectorRequirement{
			metav1.LabelSelectorRequirement{
				Key:      "a",
				Operator: metav1.LabelSelectorOpIn,
				Values: []string{
					"b",
				},
			},
			metav1.LabelSelectorRequirement{
				Key:      "testIn",
				Operator: metav1.LabelSelectorOpIn,
				Values: []string{
					"backend",
					"frontend",
				},
			},
		},
		MatchLabels: map[string]string{
			"a": "b",
			"c": "d",
		},
	}

	if !reflect.DeepEqual(selector, expectedSelector) {
		t.Errorf("TestSortSelector failed @ sort selector comparison")
		t.Errorf("selector: %v", selector)
		t.Errorf("expectedSelector: %v", expectedSelector)
	}
}

func TestHashSelector(t *testing.T) {
	firstSelector := &metav1.LabelSelector{
		MatchExpressions: []metav1.LabelSelectorRequirement{
			metav1.LabelSelectorRequirement{
				Key:      "testIn",
				Operator: metav1.LabelSelectorOpIn,
				Values: []string{
					"frontend",
					"backend",
				},
			},
		},
		MatchLabels: map[string]string{
			"a": "b",
			"c": "d",
		},
	}

	secondSelector := &metav1.LabelSelector{
		MatchExpressions: []metav1.LabelSelectorRequirement{
			metav1.LabelSelectorRequirement{
				Key:      "testIn",
				Operator: metav1.LabelSelectorOpIn,
				Values: []string{
					"backend",
					"frontend",
				},
			},
		},
		MatchLabels: map[string]string{
			"c": "d",
			"a": "b",
		},
	}

	hashedFirstSelector := HashSelector(firstSelector)
	hashedSecondSelector := HashSelector(secondSelector)
	if hashedFirstSelector != hashedSecondSelector {
		t.Errorf("TestHashSelector failed @ hashed selector comparison")
		t.Errorf("hashedFirstSelector: %v", hashedFirstSelector)
		t.Errorf("hashedSecondSelector: %v", hashedSecondSelector)
	}
}

func TestParseSelector(t *testing.T) {
	var selector, expectedSelector *metav1.LabelSelector
	selector, expectedSelector = nil, nil
	labels, vals := parseSelector(selector)
	expectedLabels, expectedVals := []string{}, make(map[string][]string)

	if len(labels) != len(expectedLabels) {
		t.Errorf("TestparseSelector failed @ labels length comparison")
	}

	if len(vals) != len(expectedVals) {
		t.Errorf("TestparseSelector failed @ vals length comparison")
	}

	if selector != expectedSelector {
		t.Errorf("TestparseSelector failed @ vals length comparison")
	}

	selector = &metav1.LabelSelector{}
	labels, vals = parseSelector(selector)
	expectedLabels = []string{""}
	if len(labels) != len(expectedLabels) {
		t.Errorf("TestparseSelector failed @ labels length comparison")
	}

	if len(vals) != len(expectedVals) {
		t.Errorf("TestparseSelector failed @ vals length comparison")
	}

	selector = &metav1.LabelSelector{
		MatchExpressions: []metav1.LabelSelectorRequirement{
			metav1.LabelSelectorRequirement{
				Key:      "testIn",
				Operator: metav1.LabelSelectorOpIn,
				Values: []string{
					"frontend",
					"backend",
				},
			},
		},
	}

	labels, vals = parseSelector(selector)
	expectedLabels = []string{}
	expectedVals = map[string][]string{
		"testIn": {
			"frontend",
			"backend",
		},
	}

	if len(labels) != len(expectedLabels) {
		t.Errorf("TestparseSelector failed @ labels length comparison")
	}

	if len(vals) != len(expectedVals) {
		t.Errorf("TestparseSelector failed @ vals length comparison")
	}

	if labels != nil {
		t.Errorf("TestparseSelector failed @ label comparison")
	}
	if !reflect.DeepEqual(vals, expectedVals) {
		t.Errorf("TestparseSelector failed @ value comparison")
	}

	notIn := metav1.LabelSelectorRequirement{
		Key:      "testNotIn",
		Operator: metav1.LabelSelectorOpNotIn,
		Values: []string{
			"frontend",
			"backend",
		},
	}

	me := &selector.MatchExpressions
	*me = append(*me, notIn)

	labels, vals = parseSelector(selector)
	addedLabels := []string{}
	addedVals := map[string][]string{
		"!testNotIn": {
			"frontend",
			"backend",
		},
	}

	expectedLabels = append(expectedLabels, addedLabels...)
	for k, v := range addedVals {
		expectedVals[k] = append(expectedVals[k], v...)
	}

	if len(labels) != len(expectedLabels) {
		t.Errorf("TestparseSelector failed @ labels length comparison")
	}

	if len(vals) != len(expectedVals) {
		t.Errorf("TestparseSelector failed @ vals length comparison")
	}

	if labels != nil {
		t.Errorf("TestparseSelector failed @ label comparison")
	}
	if !reflect.DeepEqual(vals, expectedVals) {
		t.Errorf("TestparseSelector failed @ value comparison")
	}

	exists := metav1.LabelSelectorRequirement{
		Key:      "testExists",
		Operator: metav1.LabelSelectorOpExists,
		Values:   []string{},
	}

	*me = append(*me, exists)

	labels, vals = parseSelector(selector)
	addedLabels = []string{
		"testExists",
	}
	addedVals = map[string][]string{}
	expectedLabels = append(expectedLabels, addedLabels...)
	for k, v := range addedVals {
		expectedVals[k] = append(expectedVals[k], v...)
	}

	if len(labels) != len(expectedLabels) {
		t.Errorf("TestparseSelector failed @ labels length comparison")
	}

	if len(vals) != len(expectedVals) {
		t.Errorf("TestparseSelector failed @ vals length comparison")
	}

	if !reflect.DeepEqual(labels, expectedLabels) {
		t.Errorf("TestparseSelector failed @ label comparison")
	}
	if !reflect.DeepEqual(vals, expectedVals) {
		t.Errorf("TestparseSelector failed @ value comparison")
	}

	doesNotExist := metav1.LabelSelectorRequirement{
		Key:      "testDoesNotExist",
		Operator: metav1.LabelSelectorOpDoesNotExist,
		Values:   []string{},
	}

	*me = append(*me, doesNotExist)

	labels, vals = parseSelector(selector)
	addedLabels = []string{
		"!testDoesNotExist",
	}
	addedVals = map[string][]string{}
	expectedLabels = append(expectedLabels, addedLabels...)
	for k, v := range addedVals {
		expectedVals[k] = append(expectedVals[k], v...)
	}

	if len(labels) != len(expectedLabels) {
		t.Errorf("TestparseSelector failed @ labels length comparison")
	}

	if len(vals) != len(expectedVals) {
		t.Errorf("TestparseSelector failed @ vals length comparison")
	}

	if !reflect.DeepEqual(labels, expectedLabels) {
		t.Errorf("TestparseSelector failed @ label comparison")
	}

	if !reflect.DeepEqual(vals, expectedVals) {
		t.Errorf("TestparseSelector failed @ value comparison")
	}
}

func TestFlattenNameSpaceSelectorCases(t *testing.T) {
	firstSelector := &metav1.LabelSelector{}

	testSelectors := FlattenNameSpaceSelector(firstSelector)
	if len(testSelectors) != 1 {
		t.Errorf("TestFlattenNameSpaceSelectorCases failed @ 1st selector length check %+v", testSelectors)
	}

	var secondSelector *metav1.LabelSelector

	testSelectors = FlattenNameSpaceSelector(secondSelector)
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
			metav1.LabelSelectorRequirement{
				Key:      "testIn",
				Operator: metav1.LabelSelectorOpIn,
				Values: []string{
					"backend",
				},
			},
			metav1.LabelSelectorRequirement{
				Key:      "pod",
				Operator: metav1.LabelSelectorOpIn,
				Values: []string{
					"a",
				},
			},
			metav1.LabelSelectorRequirement{
				Key:      "testExists",
				Operator: metav1.LabelSelectorOpExists,
				Values:   []string{},
			},
			metav1.LabelSelectorRequirement{
				Key:      "ns",
				Operator: metav1.LabelSelectorOpIn,
				Values: []string{
					"t",
				},
			},
		},
		MatchLabels: commonMatchLabel,
	}

	testSelectors := FlattenNameSpaceSelector(firstSelector)
	if len(testSelectors) != 1 {
		t.Errorf("TestFlattenNameSpaceSelector failed @ 1st selector length check %+v", testSelectors)
	}

	if !reflect.DeepEqual(testSelectors[0], *firstSelector) {
		t.Errorf("TestFlattenNameSpaceSelector failed @ 1st selector deepEqual check.\n Expected: %+v \n Actual: %+v", *firstSelector, testSelectors[0])
	}

	secondSelector := &metav1.LabelSelector{
		MatchExpressions: []metav1.LabelSelectorRequirement{
			metav1.LabelSelectorRequirement{
				Key:      "testIn",
				Operator: metav1.LabelSelectorOpIn,
				Values: []string{
					"backend",
					"frontend",
				},
			},
			metav1.LabelSelectorRequirement{
				Key:      "pod",
				Operator: metav1.LabelSelectorOpIn,
				Values: []string{
					"a",
					"b",
				},
			},
			metav1.LabelSelectorRequirement{
				Key:      "testExists",
				Operator: metav1.LabelSelectorOpExists,
				Values:   []string{},
			},
			metav1.LabelSelectorRequirement{
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

	testSelectors = FlattenNameSpaceSelector(secondSelector)
	if len(testSelectors) != 8 {
		t.Errorf("TestFlattenNameSpaceSelector failed @ 2nd selector length check %+v", testSelectors)
	}

	expectedSelectors := []metav1.LabelSelector{
		metav1.LabelSelector{
			MatchExpressions: []metav1.LabelSelectorRequirement{
				metav1.LabelSelectorRequirement{
					Key:      "testExists",
					Operator: metav1.LabelSelectorOpExists,
					Values:   []string{},
				},
				metav1.LabelSelectorRequirement{
					Key:      "testIn",
					Operator: metav1.LabelSelectorOpIn,
					Values: []string{
						"backend",
					},
				},
				metav1.LabelSelectorRequirement{
					Key:      "pod",
					Operator: metav1.LabelSelectorOpIn,
					Values: []string{
						"a",
					},
				},
				metav1.LabelSelectorRequirement{
					Key:      "ns",
					Operator: metav1.LabelSelectorOpIn,
					Values: []string{
						"t",
					},
				},
			},
			MatchLabels: commonMatchLabel,
		},
		metav1.LabelSelector{
			MatchExpressions: []metav1.LabelSelectorRequirement{
				metav1.LabelSelectorRequirement{
					Key:      "testExists",
					Operator: metav1.LabelSelectorOpExists,
					Values:   []string{},
				},
				metav1.LabelSelectorRequirement{
					Key:      "testIn",
					Operator: metav1.LabelSelectorOpIn,
					Values: []string{
						"backend",
					},
				},
				metav1.LabelSelectorRequirement{
					Key:      "pod",
					Operator: metav1.LabelSelectorOpIn,
					Values: []string{
						"a",
					},
				},
				metav1.LabelSelectorRequirement{
					Key:      "ns",
					Operator: metav1.LabelSelectorOpIn,
					Values: []string{
						"y",
					},
				},
			},
			MatchLabels: commonMatchLabel,
		},
		metav1.LabelSelector{
			MatchExpressions: []metav1.LabelSelectorRequirement{
				metav1.LabelSelectorRequirement{
					Key:      "testExists",
					Operator: metav1.LabelSelectorOpExists,
					Values:   []string{},
				},
				metav1.LabelSelectorRequirement{
					Key:      "testIn",
					Operator: metav1.LabelSelectorOpIn,
					Values: []string{
						"backend",
					},
				},
				metav1.LabelSelectorRequirement{
					Key:      "pod",
					Operator: metav1.LabelSelectorOpIn,
					Values: []string{
						"b",
					},
				},
				metav1.LabelSelectorRequirement{
					Key:      "ns",
					Operator: metav1.LabelSelectorOpIn,
					Values: []string{
						"t",
					},
				},
			},
			MatchLabels: commonMatchLabel,
		},
		metav1.LabelSelector{
			MatchExpressions: []metav1.LabelSelectorRequirement{
				metav1.LabelSelectorRequirement{
					Key:      "testExists",
					Operator: metav1.LabelSelectorOpExists,
					Values:   []string{},
				},
				metav1.LabelSelectorRequirement{
					Key:      "testIn",
					Operator: metav1.LabelSelectorOpIn,
					Values: []string{
						"backend",
					},
				},
				metav1.LabelSelectorRequirement{
					Key:      "pod",
					Operator: metav1.LabelSelectorOpIn,
					Values: []string{
						"b",
					},
				},
				metav1.LabelSelectorRequirement{
					Key:      "ns",
					Operator: metav1.LabelSelectorOpIn,
					Values: []string{
						"y",
					},
				},
			},
			MatchLabels: commonMatchLabel,
		},
		metav1.LabelSelector{
			MatchExpressions: []metav1.LabelSelectorRequirement{
				metav1.LabelSelectorRequirement{
					Key:      "testExists",
					Operator: metav1.LabelSelectorOpExists,
					Values:   []string{},
				},
				metav1.LabelSelectorRequirement{
					Key:      "testIn",
					Operator: metav1.LabelSelectorOpIn,
					Values: []string{
						"frontend",
					},
				},
				metav1.LabelSelectorRequirement{
					Key:      "pod",
					Operator: metav1.LabelSelectorOpIn,
					Values: []string{
						"a",
					},
				},
				metav1.LabelSelectorRequirement{
					Key:      "ns",
					Operator: metav1.LabelSelectorOpIn,
					Values: []string{
						"t",
					},
				},
			},
			MatchLabels: commonMatchLabel,
		},
		metav1.LabelSelector{
			MatchExpressions: []metav1.LabelSelectorRequirement{
				metav1.LabelSelectorRequirement{
					Key:      "testExists",
					Operator: metav1.LabelSelectorOpExists,
					Values:   []string{},
				},
				metav1.LabelSelectorRequirement{
					Key:      "testIn",
					Operator: metav1.LabelSelectorOpIn,
					Values: []string{
						"frontend",
					},
				},
				metav1.LabelSelectorRequirement{
					Key:      "pod",
					Operator: metav1.LabelSelectorOpIn,
					Values: []string{
						"a",
					},
				},
				metav1.LabelSelectorRequirement{
					Key:      "ns",
					Operator: metav1.LabelSelectorOpIn,
					Values: []string{
						"y",
					},
				},
			},
			MatchLabels: commonMatchLabel,
		},
		metav1.LabelSelector{
			MatchExpressions: []metav1.LabelSelectorRequirement{
				metav1.LabelSelectorRequirement{
					Key:      "testExists",
					Operator: metav1.LabelSelectorOpExists,
					Values:   []string{},
				},
				metav1.LabelSelectorRequirement{
					Key:      "testIn",
					Operator: metav1.LabelSelectorOpIn,
					Values: []string{
						"frontend",
					},
				},
				metav1.LabelSelectorRequirement{
					Key:      "pod",
					Operator: metav1.LabelSelectorOpIn,
					Values: []string{
						"b",
					},
				},
				metav1.LabelSelectorRequirement{
					Key:      "ns",
					Operator: metav1.LabelSelectorOpIn,
					Values: []string{
						"t",
					},
				},
			},
			MatchLabels: commonMatchLabel,
		},
		metav1.LabelSelector{
			MatchExpressions: []metav1.LabelSelectorRequirement{
				metav1.LabelSelectorRequirement{
					Key:      "testExists",
					Operator: metav1.LabelSelectorOpExists,
					Values:   []string{},
				},
				metav1.LabelSelectorRequirement{
					Key:      "testIn",
					Operator: metav1.LabelSelectorOpIn,
					Values: []string{
						"frontend",
					},
				},
				metav1.LabelSelectorRequirement{
					Key:      "pod",
					Operator: metav1.LabelSelectorOpIn,
					Values: []string{
						"b",
					},
				},
				metav1.LabelSelectorRequirement{
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
			metav1.LabelSelectorRequirement{
				Key:      "testIn",
				Operator: metav1.LabelSelectorOpIn,
				Values: []string{
					"backend",
				},
			},
			metav1.LabelSelectorRequirement{
				Key:      "pod",
				Operator: metav1.LabelSelectorOpIn,
				Values: []string{
					"a",
				},
			},
			metav1.LabelSelectorRequirement{
				Key:      "testExists",
				Operator: metav1.LabelSelectorOpExists,
				Values:   []string{},
			},
			metav1.LabelSelectorRequirement{
				Key:      "ns",
				Operator: metav1.LabelSelectorOpIn,
				Values: []string{
					"t",
					"y",
				},
			},
		},
	}

	testSelectors := FlattenNameSpaceSelector(firstSelector)
	if len(testSelectors) != 2 {
		t.Errorf("TestFlattenNameSpaceSelector failed @ 1st selector length check %+v", testSelectors)
	}

	expectedSelectors := []metav1.LabelSelector{
		metav1.LabelSelector{
			MatchExpressions: []metav1.LabelSelectorRequirement{
				metav1.LabelSelectorRequirement{
					Key:      "testIn",
					Operator: metav1.LabelSelectorOpIn,
					Values: []string{
						"backend",
					},
				},
				metav1.LabelSelectorRequirement{
					Key:      "pod",
					Operator: metav1.LabelSelectorOpIn,
					Values: []string{
						"a",
					},
				},
				metav1.LabelSelectorRequirement{
					Key:      "testExists",
					Operator: metav1.LabelSelectorOpExists,
					Values:   []string{},
				},
				metav1.LabelSelectorRequirement{
					Key:      "ns",
					Operator: metav1.LabelSelectorOpIn,
					Values: []string{
						"t",
					},
				},
			},
		},
		metav1.LabelSelector{
			MatchExpressions: []metav1.LabelSelectorRequirement{
				metav1.LabelSelectorRequirement{
					Key:      "testIn",
					Operator: metav1.LabelSelectorOpIn,
					Values: []string{
						"backend",
					},
				},
				metav1.LabelSelectorRequirement{
					Key:      "pod",
					Operator: metav1.LabelSelectorOpIn,
					Values: []string{
						"a",
					},
				},
				metav1.LabelSelectorRequirement{
					Key:      "testExists",
					Operator: metav1.LabelSelectorOpExists,
					Values:   []string{},
				},
				metav1.LabelSelectorRequirement{
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
