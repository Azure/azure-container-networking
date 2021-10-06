package dataplane

import (
	"testing"

	"github.com/Azure/azure-container-networking/npm/metrics"
	"github.com/Azure/azure-container-networking/npm/pkg/dataplane/ipsets"
)

func TestNewDataPlane(t *testing.T) {
	metrics.InitializeAll()
	dp := NewDataPlane("testnode")

	if dp == nil {
		t.Error("NewDataPlane() returned nil")
	}

	dp.CreateIPSet("test", ipsets.NameSpace)
}

func TestInitializeDataPlane(t *testing.T) {
	metrics.InitializeAll()
	dp := NewDataPlane("testnode")

	if dp == nil {
		t.Error("NewDataPlane() returned nil")
	}

	err := dp.InitializeDataPlane()
	if err != nil {
		t.Errorf("InitializeDataPlane() returned error %v", err)
	}
}

func TestResetDataPlane(t *testing.T) {
	metrics.InitializeAll()
	dp := NewDataPlane("testnode")

	if dp == nil {
		t.Error("NewDataPlane() returned nil")
	}

	err := dp.InitializeDataPlane()
	if err != nil {
		t.Errorf("InitializeDataPlane() returned error %v", err)
	}
	err = dp.ResetDataPlane()
	if err != nil {
		t.Errorf("ResetDataPlane() returned error %v", err)
	}
}

func TestCreateAndDeleteIpSets(t *testing.T) {
	metrics.InitializeAll()
	dp := NewDataPlane("testnode")

	setsTocreate := map[string]ipsets.SetType{
		"test":  ipsets.NameSpace,
		"test1": ipsets.NameSpace,
	}

	for k, v := range setsTocreate {
		dp.CreateIPSet(k, v)
	}

	// Creating again to see if duplicates get created
	for k, v := range setsTocreate {
		dp.CreateIPSet(k, v)
	}

	for k := range setsTocreate {
		set := dp.ipsetMgr.GetIPSet(k)
		if set == nil {
			t.Errorf("GetIPSet() for %s returned nil", k)
		}
	}

	for k := range setsTocreate {
		dp.DeleteIPSet(k)
	}

	for k := range setsTocreate {
		set := dp.ipsetMgr.GetIPSet(k)
		if set != nil {
			t.Errorf("GetIPSet() for %s returned nil", k)
		}
	}
}

func TestAddToSet(t *testing.T) {
	metrics.InitializeAll()
	dp := NewDataPlane("testnode")

	setsTocreate := map[string]ipsets.SetType{
		"test":  ipsets.NameSpace,
		"test1": ipsets.NameSpace,
	}

	for k, v := range setsTocreate {
		dp.CreateIPSet(k, v)
	}

	for k := range setsTocreate {
		set := dp.ipsetMgr.GetIPSet(k)
		if set == nil {
			t.Errorf("GetIPSet() for %s returned nil", k)
		}
	}
	setNames := make([]string, len(setsTocreate))
	i := 0
	for k := range setsTocreate {
		setNames[i] = k
		i++
	}

	err := dp.AddToSet(setNames, "10.0.0.1", "testns/a")
	if err != nil {
		t.Errorf("AddToSet() returned error %v", err)
	}

	// Test IPV6 addess it should error out
	err = dp.AddToSet(setNames, "2001:db8:0:0:0:0:2:1", "testns/a")
	if err == nil {
		t.Error("AddToSet() ipv6 did not return error")
	}

	for k := range setsTocreate {
		dp.DeleteIPSet(k)
	}

	for k := range setsTocreate {
		set := dp.ipsetMgr.GetIPSet(k)
		if set == nil {
			t.Errorf("GetIPSet() for %s returned nil", k)
		}
	}

	err = dp.RemoveFromSet(setNames, "10.0.0.1", "testns/a")
	if err != nil {
		t.Errorf("RemoveFromSet() returned error %v", err)
	}

	for k := range setsTocreate {
		dp.DeleteIPSet(k)
	}

	for k := range setsTocreate {
		set := dp.ipsetMgr.GetIPSet(k)
		if set != nil {
			t.Errorf("GetIPSet() for %s returned nil", k)
		}
	}
}
