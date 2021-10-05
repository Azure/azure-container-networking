package ipsets

import (
	"os"
	"reflect"
	"testing"

	"github.com/Azure/azure-container-networking/npm/metrics"
)

const (
	testSetName  = "test-set"
	testListName = "test-list"
	testPodKey   = "test-pod-key"
	testPodIP    = "10.0.0.0"
)

func TestCreateIPSet(t *testing.T) {
	iMgr := NewIPSetManager("azure")

	iMgr.CreateIPSet(testSetName, NameSpace)

	if !iMgr.exists(testSetName) {
		t.Errorf("CreateIPSet() did not create set")
	}
}

func TestAddToSet(t *testing.T) {
	iMgr := NewIPSetManager("azure")

	iMgr.CreateIPSet(testSetName, NameSpace)

	err := iMgr.AddToSet([]string{testSetName}, testPodIP, testPodKey)
	if err != nil {
		t.Errorf("AddToSet() returned error %s", err.Error())
	}
}

func TestRemoveFromSet(t *testing.T) {
	iMgr := NewIPSetManager("azure")

	iMgr.CreateIPSet(testSetName, NameSpace)
	err := iMgr.AddToSet([]string{testSetName}, testPodIP, testPodKey)
	if err != nil {
		t.Errorf("RemoveFromSet() returned error %s", err.Error())
	}
	err = iMgr.RemoveFromSet([]string{testSetName}, testPodIP, testPodKey)
	if err != nil {
		t.Errorf("RemoveFromSet() returned error %s", err.Error())
	}
}

func TestRemoveFromSetMissing(t *testing.T) {
	iMgr := NewIPSetManager("azure")
	err := iMgr.RemoveFromSet([]string{testSetName}, testPodIP, testPodKey)
	if err == nil {
		t.Errorf("RemoveFromSet() did not return error")
	}
}

func TestAddToListMissing(t *testing.T) {
	iMgr := NewIPSetManager("azure")
	err := iMgr.AddToList(testPodKey, []string{"newtest"})
	if err == nil {
		t.Errorf("AddToList() did not return error")
	}
}

func TestAddToList(t *testing.T) {
	iMgr := NewIPSetManager("azure")
	iMgr.CreateIPSet(testSetName, NameSpace)
	iMgr.CreateIPSet(testListName, KeyLabelOfNameSpace)

	err := iMgr.AddToList(testListName, []string{testSetName})
	if err != nil {
		t.Errorf("AddToList() returned error %s", err.Error())
	}
}

func TestRemoveFromList(t *testing.T) {
	iMgr := NewIPSetManager("azure")
	iMgr.CreateIPSet(testSetName, NameSpace)
	iMgr.CreateIPSet(testListName, KeyLabelOfNameSpace)

	err := iMgr.AddToList(testListName, []string{testSetName})
	if err != nil {
		t.Errorf("AddToList() returned error %s", err.Error())
	}

	err = iMgr.RemoveFromList(testListName, []string{testSetName})
	if err != nil {
		t.Errorf("RemoveFromList() returned error %s", err.Error())
	}
}

func TestRemoveFromListMissing(t *testing.T) {
	iMgr := NewIPSetManager("azure")

	iMgr.CreateIPSet(testListName, KeyLabelOfNameSpace)

	err := iMgr.RemoveFromList(testListName, []string{testSetName})
	if err == nil {
		t.Errorf("RemoveFromList() did not return error")
	}
}

func TestDeleteIPSet(t *testing.T) {
	iMgr := NewIPSetManager("azure")
	iMgr.CreateIPSet(testSetName, NameSpace)

	iMgr.DeleteIPSet(testSetName)
	// TODO add cache check
}

func TestGetIPsFromSelectorIPSets(t *testing.T) {
	iMgr := NewIPSetManager("azure")
	iMgr.CreateIPSet("setNs1", NameSpace)
	iMgr.CreateIPSet("setpod1", KeyLabelOfPod)
	iMgr.CreateIPSet("setpod2", KeyLabelOfPod)
	iMgr.CreateIPSet("setpod3", KeyValueLabelOfPod)

	err := iMgr.AddToSet([]string{"setNs1", "setpod1", "setpod2", "setpod3"}, "10.0.0.1", "test")
	if err != nil {
		t.Errorf("AddToSet() returned error %s", err.Error())
	}

	err = iMgr.AddToSet([]string{"setNs1", "setpod1", "setpod2", "setpod3"}, "10.0.0.2", "test1")
	if err != nil {
		t.Errorf("AddToSet() returned error %s", err.Error())
	}

	err = iMgr.AddToSet([]string{"setNs1", "setpod2", "setpod3"}, "10.0.0.3", "test3")
	if err != nil {
		t.Errorf("AddToSet() returned error %s", err.Error())
	}

	ipsetList := map[string]struct{}{
		"setNs1":  {},
		"setpod1": {},
		"setpod2": {},
		"setpod3": {},
	}
	ips, err := iMgr.GetIPsFromSelectorIPSets(ipsetList)
	if err != nil {
		t.Errorf("GetIPsFromSelectorIPSets() returned error %s", err.Error())
	}

	if len(ips) != 2 {
		t.Errorf("GetIPsFromSelectorIPSets() returned wrong number of IPs %d", len(ips))
		t.Error(ips)
	}

	expectedintersection := map[string]struct{}{
		"10.0.0.1": {},
		"10.0.0.2": {},
	}

	if reflect.DeepEqual(ips, expectedintersection) == false {
		t.Errorf("GetIPsFromSelectorIPSets() returned wrong IPs")
	}
}

func TestMain(m *testing.M) {
	metrics.InitializeAll()

	exitCode := m.Run()

	os.Exit(exitCode)
}
