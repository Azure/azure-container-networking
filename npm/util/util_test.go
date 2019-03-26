package util

import (
	"testing"

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
