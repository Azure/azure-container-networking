package npm

import (
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

func TestParseSelector(t *testing.T) {
	var selector, expectedSelector *metav1.LabelSelector
	selector, expectedSelector = nil, nil
	labels, keys, vals := ParseSelector(selector)
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
	labels, keys, vals = ParseSelector(selector)
	expectedLabels = []string{util.KubeAllNamespacesFlag}
	expectedKeys = []string{util.KubeAllNamespacesFlag}
	expectedVals = []string{""}
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

	labels, keys, vals = ParseSelector(selector)
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

	labels, keys, vals = ParseSelector(selector)
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

	labels, keys, vals = ParseSelector(selector)
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

	labels, keys, vals = ParseSelector(selector)
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
