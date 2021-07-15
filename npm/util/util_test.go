package util

import (
	"reflect"
	"testing"

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

func TestCompareResourceVersions(t *testing.T) {
	oldRv := "12345"
	newRV := "23456"

	check := CompareResourceVersions(oldRv, newRV)
	if !check {
		t.Errorf("TestCompareResourceVersions failed @ compare RVs with error returned wrong result ")
	}

}

func TestInValidOldResourceVersions(t *testing.T) {
	oldRv := "sssss"
	newRV := "23456"

	check := CompareResourceVersions(oldRv, newRV)
	if !check {
		t.Errorf("TestInValidOldResourceVersions failed @ compare RVs with error returned wrong result ")
	}

}

func TestInValidNewResourceVersions(t *testing.T) {
	oldRv := "12345"
	newRV := "sssss"

	check := CompareResourceVersions(oldRv, newRV)
	if check {
		t.Errorf("TestInValidNewResourceVersions failed @ compare RVs with error returned wrong result ")
	}

}

func TestParseResourceVersion(t *testing.T) {
	testRv := "string"

	check := ParseResourceVersion(testRv)
	if check > 0 {
		t.Errorf("TestParseResourceVersion failed @ inavlid RV gave no error")
	}
}

func TestCompareSlices(t *testing.T) {
	list1 := []string{
		"a",
		"b",
		"c",
		"d",
	}
	list2 := []string{
		"c",
		"d",
		"a",
		"b",
	}

	if !CompareSlices(list1, list2) {
		t.Errorf("TestCompareSlices failed @ slice comparison 1")
	}

	list2 = []string{
		"c",
		"a",
		"b",
	}

	if CompareSlices(list1, list2) {
		t.Errorf("TestCompareSlices failed @ slice comparison 2")
	}
	list1 = []string{
		"a",
		"b",
		"c",
		"d",
		"123",
		"44",
	}
	list2 = []string{
		"c",
		"44",
		"d",
		"a",
		"b",
		"123",
	}

	if !CompareSlices(list1, list2) {
		t.Errorf("TestCompareSlices failed @ slice comparison 3")
	}

	list1 = []string{}
	list2 = []string{}

	if !CompareSlices(list1, list2) {
		t.Errorf("TestCompareSlices failed @ slice comparison 4")
	}
}
