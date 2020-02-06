package util

import (
	"testing"
	"reflect"

	"k8s.io/apimachinery/pkg/version"
)

func TestSortMap(t *testing.T) {
	m := &map[string]string{
		"e": "f",
		"c": "d",
		"a": "b",
	}

	sortedKeys, sortedVals := SortMap(m)
	
	expectedKeys := []string{
		"a",
		"c",
		"e",
	}

	expectedVals := []string{
		"b",
		"d",
		"f",
	}

	if !reflect.DeepEqual(sortedKeys, expectedKeys) {
		t.Errorf("TestSortMap failed @ key comparison")
		t.Errorf("sortedKeys: %v", sortedKeys)
		t.Errorf("expectedKeys: %v", expectedKeys)
	}

	if !reflect.DeepEqual(sortedVals, expectedVals) {
		t.Errorf("TestSortMap failed @ val comparison")
		t.Errorf("sortedVals: %v", sortedVals)
		t.Errorf("expectedVals: %v", expectedVals)
	}
}

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

	firstVer = &version.Info{
		Major: "1",
		Minor: "14.8-hotfix.20191113",
	}
	
	secondVer = &version.Info{
		Major: "1",
		Minor: "11",
	}
	
	if res := CompareK8sVer(firstVer, secondVer); res != 1 {
		t.Errorf("TestCompareK8sVer failed @ firstVer > secondVer w/ hotfix tag/pre-release")
	}

	firstVer = &version.Info{
		Major: "1",
		Minor: "14+",
	}
	
	secondVer = &version.Info{
		Major: "1",
		Minor: "11",
	}
	
	if res := CompareK8sVer(firstVer, secondVer); res != 1 {
		t.Errorf("TestCompareK8sVer failed @ firstVer > secondVer w/ minor+ release")
	}

	firstVer = &version.Info{
		Major: "2",
		Minor: "1",
	}

	secondVer = &version.Info{
		Major: "1",
		Minor: "11",
	}

	if res := CompareK8sVer(firstVer, secondVer); res != 1 {
		t.Errorf("TestCompareK8sVer failed @ firstVer > secondVer w/ major version upgrade")
	}

	firstVer = &version.Info{
		Major: "1",
		Minor: "11",
	}

	secondVer = &version.Info{
		Major: "2",
		Minor: "1",
	}

	if res := CompareK8sVer(firstVer, secondVer); res != -1 {
		t.Errorf("TestCompareK8sVer failed @ firstVer < secondVer w/ major version upgrade")
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