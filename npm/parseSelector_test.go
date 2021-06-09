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
	labels, keys, vals := parseSelector(selector)
	expectedLabels, expectedKeys, expectedVals := []string{}, []string{}, []string{}

	if len(labels) != len(expectedLabels) {
		t.Errorf("TestparseSelector failed @ labels length comparison")
	}

	if len(keys) != len(expectedKeys) {
		t.Errorf("TestparseSelector failed @ keys length comparison")
	}

	if len(vals) != len(expectedVals) {
		t.Errorf("TestparseSelector failed @ vals length comparison")
	}

	if selector != expectedSelector {
		t.Errorf("TestparseSelector failed @ vals length comparison")
	}

	selector = &metav1.LabelSelector{}
	labels, keys, vals = parseSelector(selector)
	expectedLabels, expectedKeys, expectedVals = []string{""}, []string{""}, []string{""}
	if len(labels) != len(expectedLabels) {
		t.Errorf("TestparseSelector failed @ labels length comparison")
	}

	if len(keys) != len(expectedKeys) {
		t.Errorf("TestparseSelector failed @ keys length comparison")
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

	labels, keys, vals = parseSelector(selector)
	expectedLabels = []string{
		"testIn:frontend",
		"testIn:backend",
	}
	expectedKeys = []string{
		"testIn",
		"testIn",
	}
	expectedVals = []string{
		"frontend",
		"backend",
	}

	if len(labels) != len(expectedLabels) {
		t.Errorf("TestparseSelector failed @ labels length comparison")
	}

	if len(keys) != len(expectedKeys) {
		t.Errorf("TestparseSelector failed @ keys length comparison")
	}

	if len(vals) != len(vals) {
		t.Errorf("TestparseSelector failed @ vals length comparison")
	}

	if !reflect.DeepEqual(labels, expectedLabels) {
		t.Errorf("TestparseSelector failed @ label comparison")
	}
	if !reflect.DeepEqual(keys, expectedKeys) {
		t.Errorf("TestparseSelector failed @ key comparison")
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

	labels, keys, vals = parseSelector(selector)
	addedLabels := []string{
		"!testNotIn:frontend",
		"!testNotIn:backend",
	}
	addedKeys := []string{
		"!testNotIn",
		"!testNotIn",
	}
	addedVals := []string{
		"frontend",
		"backend",
	}
	expectedLabels = append(expectedLabels, addedLabels...)
	expectedKeys = append(expectedKeys, addedKeys...)
	expectedVals = append(expectedVals, addedVals...)

	if len(labels) != len(expectedLabels) {
		t.Errorf("TestparseSelector failed @ labels length comparison")
	}

	if len(keys) != len(expectedKeys) {
		t.Errorf("TestparseSelector failed @ keys length comparison")
	}

	if len(vals) != len(vals) {
		t.Errorf("TestparseSelector failed @ vals length comparison")
	}

	if !reflect.DeepEqual(labels, expectedLabels) {
		t.Errorf("TestparseSelector failed @ label comparison")
	}
	if !reflect.DeepEqual(keys, expectedKeys) {
		t.Errorf("TestparseSelector failed @ key comparison")
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

	labels, keys, vals = parseSelector(selector)
	addedLabels = []string{
		"testExists",
	}
	addedKeys = []string{
		"testExists",
	}
	addedVals = []string{
		"",
	}
	expectedLabels = append(expectedLabels, addedLabels...)
	expectedKeys = append(expectedKeys, addedKeys...)
	expectedVals = append(expectedVals, addedVals...)

	if len(labels) != len(expectedLabels) {
		t.Errorf("TestparseSelector failed @ labels length comparison")
	}

	if len(keys) != len(expectedKeys) {
		t.Errorf("TestparseSelector failed @ keys length comparison")
	}

	if len(vals) != len(vals) {
		t.Errorf("TestparseSelector failed @ vals length comparison")
	}

	if !reflect.DeepEqual(labels, expectedLabels) {
		t.Errorf("TestparseSelector failed @ label comparison")
	}
	if !reflect.DeepEqual(keys, expectedKeys) {
		t.Errorf("TestparseSelector failed @ key comparison")
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

	labels, keys, vals = parseSelector(selector)
	addedLabels = []string{
		"!testDoesNotExist",
	}
	addedKeys = []string{
		"!testDoesNotExist",
	}
	addedVals = []string{
		"",
	}
	expectedLabels = append(expectedLabels, addedLabels...)
	expectedKeys = append(expectedKeys, addedKeys...)
	expectedVals = append(expectedVals, addedVals...)

	if len(labels) != len(expectedLabels) {
		t.Errorf("TestparseSelector failed @ labels length comparison")
	}

	if len(keys) != len(expectedKeys) {
		t.Errorf("TestparseSelector failed @ keys length comparison")
	}

	if len(vals) != len(vals) {
		t.Errorf("TestparseSelector failed @ vals length comparison")
	}

	if !reflect.DeepEqual(labels, expectedLabels) {
		t.Errorf("TestparseSelector failed @ label comparison")
	}

	if !reflect.DeepEqual(keys, expectedKeys) {
		t.Errorf("TestparseSelector failed @ key comparison")
	}

	if !reflect.DeepEqual(vals, expectedVals) {
		t.Errorf("TestparseSelector failed @ value comparison")
	}
}
