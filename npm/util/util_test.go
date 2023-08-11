package util

import (
	"reflect"
	"testing"

	"github.com/stretchr/testify/require"
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

func TestExists(t *testing.T) {
	type args struct {
		filePath string
	}
	dir := t.TempDir()
	tests := []struct {
		name string
		args args
		want bool
	}{
		{
			name: "Test for filepath exists",
			args: args{
				dir,
			},
			want: true,
		},
		{
			name: "Test for directory/file not exist",
			args: args{
				"unknown_directory",
			},
			want: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := Exists(tt.args.filePath); got != tt.want {
				t.Errorf("Exists() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestGetClusterID(t *testing.T) {
	type args struct {
		nodeName string
	}
	tests := []struct {
		name string
		args args
		want string
	}{
		{
			name: "Test to get cluster id for invalid azure node name",
			args: args{
				"nodename-test111",
			},
			want: "",
		},
		{
			name: "Test to get cluster id for valid azure node name",
			args: args{
				"aks-agentpool-vmss000000",
			},
			want: "vmss000000",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := GetClusterID(tt.args.nodeName); got != tt.want {
				t.Errorf("GetClusterID() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestGetIPSetListFromLabels(t *testing.T) {
	labels := make(map[string]string)
	labels["test-key"] = "test-val"
	expected := []string{
		"test-key",
		"test-key:test-val",
	}
	got := GetIPSetListFromLabels(labels)
	if len(got) != 2 || expected[0] != got[0] || expected[1] != got[1] {
		t.Errorf("GetIPSetListFromLabels(labels map[string]string) = %v, want %v", got, expected)
	}
}

func TestClearAndAppendMap(t *testing.T) {
	base := map[string]string{
		"base-key": "base-val",
	}
	newmap := map[string]string{
		"one": "uno",
		"two": "dos",
	}
	if got := ClearAndAppendMap(base, newmap); !reflect.DeepEqual(got, newmap) {
		t.Errorf("ClearAndAppendMap() = %v, want %v", got, newmap)
	}
}

func TestAppendMap(t *testing.T) {
	base := map[string]string{
		"one": "uno",
	}
	mapAppend := map[string]string{
		"two": "two",
	}
	result := map[string]string{
		"one": "uno",
		"two": "two",
	}
	if got := AppendMap(base, mapAppend); !reflect.DeepEqual(got, result) {
		t.Errorf("AppendMap() = %v, want %v", got, result)
	}
}

func TestGetOperatorAndLabel(t *testing.T) {
	type args struct {
		label string
	}
	tests := []struct {
		name  string
		args  args
		want0 string
		want1 string
	}{
		{
			name: "Test for empty input",
			args: args{
				"",
			},
			want0: "",
			want1: "",
		},
		{
			name: "Test for iptables not flag",
			args: args{
				"!test",
			},
			want0: "!",
			want1: "test",
		},
		{
			name: "Test for normal label",
			args: args{
				"test",
			},
			want0: "",
			want1: "test",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, got1 := GetOperatorAndLabel(tt.args.label)
			if got != tt.want0 {
				t.Errorf("GetOperatorAndLabel() got = %v, want %v", got, tt.want0)
			}
			if got1 != tt.want1 {
				t.Errorf("GetOperatorAndLabel() got1 = %v, want %v", got1, tt.want1)
			}
		})
	}
}

func TestGetLabelsWithoutOperators(t *testing.T) {
	want := []string{
		"res",
		"res2",
	}
	labels := []string{
		"!res",
		"res2",
	}
	if got := GetLabelsWithoutOperators(labels); !reflect.DeepEqual(want, got) {
		t.Errorf("GetLabelsWithoutOperators() got = %v, want %v", got, want)
	}
}

func TestGetSetsFromLabels(t *testing.T) {
	labels := map[string]string{
		"key": "val",
	}
	want := []string{
		"key",
		"key:val",
	}
	if got := GetSetsFromLabels(labels); !reflect.DeepEqual(want, got) {
		t.Errorf("GetSetsFromLabels() got = %v, want %v", got, want)
	}
}

func TestSliceToString(t *testing.T) {
	want := "test,test2"
	list := []string{
		"test",
		"test2",
	}
	if got := SliceToString(list); want != got {
		t.Errorf("SliceToString() got = %v, want %v, using delimiter %v", got, want, SetPolicyDelimiter)
	}
}

func TestNodeIP(t *testing.T) {
	_, err := NodeIP()
	require.Nil(t, err, "NodeIP() returned error")
}
