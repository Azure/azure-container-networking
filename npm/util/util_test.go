package util

import (
	"testing"
	"reflect"

	"k8s.io/apimachinery/pkg/version"
)

func TestCompareK8sVer(t *testing.T) {
	firstVer := &version.Info{
		Major: "!",
		Minor: "%",
	}

	secondVer := &version.Info{
		Major: "@",
		Minor: "11",
	}

	if res := CompareK8sVer(firstVer, secondVer); res != -2 {
		t.Errorf("TestCompareK8sVer failed @ invalid version test")
	}

	firstVer = &version.Info{
		Major: "1",
		Minor: "10",
	}

	secondVer = &version.Info{
		Major: "1",
		Minor: "11",
	}

	if res := CompareK8sVer(firstVer, secondVer); res != -1 {
		t.Errorf("TestCompareK8sVer failed @ firstVer < secondVer")
	}

	firstVer = &version.Info{
		Major: "1",
		Minor: "11",
	}

	secondVer = &version.Info{
		Major: "1",
		Minor: "11",
	}

	if res := CompareK8sVer(firstVer, secondVer); res != 0 {
		t.Errorf("TestCompareK8sVer failed @ firstVer == secondVer")
	}

	firstVer = &version.Info{
		Major: "1",
		Minor: "11",
	}

	secondVer = &version.Info{
		Major: "1",
		Minor: "10",
	}

	if res := CompareK8sVer(firstVer, secondVer); res != 1 {
		t.Errorf("TestCompareK8sVer failed @ firstVer > secondVer")
	}
}

func TestIsNewNwPolicyVer(t *testing.T) {
	ver := &version.Info{
		Major: "!",
		Minor: "%",
	}

	isNew, err := IsNewNwPolicyVer(ver)
	if isNew || err == nil {
		t.Errorf("TestIsNewNwPolicyVer failed @ invalid version test")
	}

	ver = &version.Info{
		Major: "1",
		Minor: "9",
	}

	isNew, err = IsNewNwPolicyVer(ver)
	if isNew || err != nil {
		t.Errorf("TestIsNewNwPolicyVer failed @ older version test")
	}

	ver = &version.Info{
		Major: "1",
		Minor: "11",
	}

	isNew, err = IsNewNwPolicyVer(ver)
	if !isNew || err != nil {
		t.Errorf("TestIsNewNwPolicyVer failed @ same version test")
	}

	ver = &version.Info{
		Major: "1",
		Minor: "13",
	}

	isNew, err = IsNewNwPolicyVer(ver)
	if !isNew || err != nil {
		t.Errorf("TestIsNewNwPolicyVer failed @ newer version test")
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
		IptablesNotFlag,
		IptablesNotFlag,
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

func TestGetLabelsWithoutOperators(t *testing.T) {
	testLabels := []string{
		"k:v",
		"",
		"!a:b",
	}

	resultLabels := GetLabelsWithoutOperators(testLabels)
	expectedLabels := []string{
		"k:v",
		"a:b",
	}

	if !reflect.DeepEqual(resultLabels, expectedLabels) {
		t.Errorf("TestGetLabelsWithoutOperators failed @ label comparision")
	}
}

func TestDropEmptyFields(t *testing.T) {
	testSlice := []string{
		"",
		"a:b",
		"",
		"!",
		"-m",
		"--match-set",
		"",
	}

	resultSlice := DropEmptyFields(testSlice)
	expectedSlice := []string{
		"a:b",
		"!",
		"-m",
		"--match-set",
	}

	if !reflect.DeepEqual(resultSlice, expectedSlice) {
		t.Errorf("TestDropEmptyFields failed @ slice comparison")
	}

	testSlice = []string{""}
	resultSlice = DropEmptyFields(testSlice)
	expectedSlice = []string{}

	if !reflect.DeepEqual(resultSlice, expectedSlice) {
		t.Errorf("TestDropEmptyFields failed @ slice comparison")
	}
}