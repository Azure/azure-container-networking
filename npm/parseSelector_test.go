package npm

import (
	"testing"

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
		},
	}

	labels, keys, vals := ParseSelector(selector)
	expectedLabels := []string{
		"testIn:frontend",
		"testIn:backend",
	}
	expectedKeys := []string{
		"testIn",
		"testIn",
	}
	expectedVals := []string{
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

	for i := range labels {
		if labels[i] != expectedLabels[i] {
			t.Errorf("TestparseSelector failed @ label comparison")
		}

		if keys[i] != expectedKeys[i] {
			t.Errorf("TestparseSelector failed @ key comparison")
		}

		if vals[i] != expectedVals[i] {
			t.Errorf("TestparseSelector failed @ value comparison")
		}
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

	for i := range labels {
		if labels[i] != expectedLabels[i] {
			t.Errorf("TestparseSelector failed @ label comparison")
		}

		if keys[i] != expectedKeys[i] {
			t.Errorf("TestparseSelector failed @ key comparison")
		}

		if vals[i] != expectedVals[i] {
			t.Errorf("TestparseSelector failed @ value comparison")
		}
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

	for i := range labels {
		if labels[i] != expectedLabels[i] {
			t.Errorf("TestparseSelector failed @ label comparison")
		}

		if keys[i] != expectedKeys[i] {
			t.Errorf("TestparseSelector failed @ key comparison")
		}

		if vals[i] != expectedVals[i] {
			t.Errorf("TestparseSelector failed @ value comparison")
		}
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

	for i := range labels {
		if labels[i] != expectedLabels[i] {
			t.Errorf("TestparseSelector failed @ label comparison")
		}

		if keys[i] != expectedKeys[i] {
			t.Errorf("TestparseSelector failed @ key comparison")
		}

		if vals[i] != expectedVals[i] {
			t.Errorf("TestparseSelector failed @ value comparison")
		}
	}
}
