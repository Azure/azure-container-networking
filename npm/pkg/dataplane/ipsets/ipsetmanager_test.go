package ipsets

import (
	"os"
	"reflect"
	"testing"

	"github.com/Azure/azure-container-networking/npm/metrics"
	"github.com/Azure/azure-container-networking/npm/util"
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
	// creating twice
	iMgr.CreateIPSet(testSetName, NameSpace)

	if !iMgr.exists(testSetName) {
		t.Errorf("CreateIPSet() did not create set")
	}

	set := iMgr.GetIPSet(testSetName)
	if set == nil {
		t.Errorf("CreateIPSet() did not create set")
	} else {
		if set.Name != testSetName {
			t.Errorf("CreateIPSet() did not create set")
		}
		if set.HashedName != util.GetHashedName(testSetName) {
			t.Errorf("CreateIPSet() did not create set")
		}
	}
}

func TestAddToSet(t *testing.T) {
	iMgr := NewIPSetManager("azure")

	iMgr.CreateIPSet(testSetName, NameSpace)

	err := iMgr.AddToSet([]string{testSetName}, testPodIP, testPodKey)
	if err != nil {
		t.Errorf("AddToSet() returned error %s", err.Error())
	}

	err = iMgr.AddToSet([]string{testSetName}, "2001:db8:0:0:0:0:2:1", "newpod")
	if err == nil {
		t.Error("AddToSet() did not return error")
	}

	// same IP changed podkey
	err = iMgr.AddToSet([]string{testSetName}, testPodIP, "newpod")
	if err != nil {
		t.Errorf("AddToSet() returned error %s", err.Error())
	}

	iMgr.CreateIPSet("testipsetlist", KeyLabelOfNameSpace)
	err = iMgr.AddToSet([]string{"testipsetlist"}, testPodIP, testPodKey)
	if err == nil {
		t.Error("AddToSet() should have returned error while adding member to listset")
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

	set := iMgr.GetIPSet(testListName)
	if set == nil {
		t.Errorf("AddToList() did not create set")
	} else {
		if set.Name != testListName {
			t.Errorf("AddToList() did not create set")
		}
		if set.HashedName != util.GetHashedName(testListName) {
			t.Errorf("AddToList() did not create set")
		}
		if set.Type != KeyLabelOfNameSpace {
			t.Errorf("AddToList() did not create set")
		}
		if set.MemberIPSets[testSetName].Name != testSetName {
			t.Errorf("AddToList() did not add to list")
		}
		if len(set.MemberIPSets) == 0 {
			t.Errorf("AddToList() failed")
		}
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

	set := iMgr.GetIPSet(testListName)
	if set == nil {
		t.Errorf("AddToList() did not create set")
	} else {
		if set.Name != testListName {
			t.Errorf("AddToList() did not create set")
		}
		if set.HashedName != util.GetHashedName(testListName) {
			t.Errorf("AddToList() did not create set")
		}
		if set.Type != KeyLabelOfNameSpace {
			t.Errorf("AddToList() did not create set")
		}
		if set.MemberIPSets[testSetName].Name != testSetName {
			t.Errorf("AddToList() did not add to list")
		}
		if len(set.MemberIPSets) == 0 {
			t.Errorf("AddToList() failed")
		}
	}

	err = iMgr.RemoveFromList(testListName, []string{testSetName})
	if err != nil {
		t.Errorf("RemoveFromList() returned error %s", err.Error())
	}
	set = iMgr.GetIPSet(testListName)
	if set == nil {
		t.Errorf("RemoveFromList() failed")
	} else if len(set.MemberIPSets) != 0 {
		t.Errorf("RemoveFromList() failed")
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

	setsTocreate := map[string]SetType{
		"setNs1":  NameSpace,
		"setpod1": KeyLabelOfPod,
		"setpod2": KeyLabelOfPod,
		"setpod3": KeyValueLabelOfPod,
	}

	for k, v := range setsTocreate {
		iMgr.CreateIPSet(k, v)
	}

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

func TestAddDeleteSelectorReferences(t *testing.T) {
	iMgr := NewIPSetManager("azure")

	setsTocreate := map[string]SetType{
		"setNs1":  NameSpace,
		"setpod1": KeyLabelOfPod,
		"setpod2": KeyValueLabelOfPod,
		"setpod3": NestedLabelOfPod,
		"setpod4": KeyLabelOfPod,
	}
	networkPolicName := "testNetworkPolicy"

	for k := range setsTocreate {
		err := iMgr.AddReference(k, networkPolicName, SelectorType)
		if err == nil {
			t.Errorf("AddReference did not return error")
		}
	}
	for k, v := range setsTocreate {
		iMgr.CreateIPSet(k, v)
	}
	err := iMgr.AddToList("setpod3", []string{"setpod4"})
	if err != nil {
		t.Errorf("AddToList failed with error %s", err.Error())
	}

	for k := range setsTocreate {
		err = iMgr.AddReference(k, networkPolicName, SelectorType)
		if err != nil {
			t.Errorf("AddReference failed with error %s", err.Error())
		}
	}

	if len(iMgr.toAddOrUpdateCache) != 5 {
		t.Errorf("AddReference did not update toAddOrUpdateCache")
	}

	if len(iMgr.toDeleteCache) != 0 {
		t.Errorf("AddReference did not update toDeleteCache")
	}

	for k := range setsTocreate {
		err = iMgr.DeleteReference(k, networkPolicName, SelectorType)
		if err != nil {
			t.Errorf("DeleteReference failed with error %s", err.Error())
		}
	}

	if len(iMgr.toAddOrUpdateCache) != 0 {
		t.Errorf("DeleteReference did not update toAddOrUpdateCache")
	}

	if len(iMgr.toDeleteCache) != 5 {
		t.Errorf("DeleteReference did not update toDeleteCache")
	}

	for k := range setsTocreate {
		iMgr.DeleteIPSet(k)
	}

	// Above delete will not remove setpod3 and setpod4
	// because they are referencing each other
	if len(iMgr.setMap) != 2 {
		t.Errorf("DeleteIPSet did not remove deletable sets")
	}

	err = iMgr.RemoveFromList("setpod3", []string{"setpod4"})
	if err != nil {
		t.Errorf("RemoveFromList failed with error %s", err.Error())
	}

	for k := range setsTocreate {
		iMgr.DeleteIPSet(k)
	}

	for k := range setsTocreate {
		set := iMgr.GetIPSet(k)
		if set != nil {
			t.Errorf("DeleteIPSet did not delete %s IPSet", set.Name)
		}
	}
}

func TestAddDeleteNetPolReferences(t *testing.T) {
	iMgr := NewIPSetManager("azure")

	setsTocreate := map[string]SetType{
		"setNs1":  NameSpace,
		"setpod1": KeyLabelOfPod,
		"setpod2": KeyValueLabelOfPod,
		"setpod3": NestedLabelOfPod,
		"setpod4": KeyLabelOfPod,
	}
	networkPolicName := "testNetworkPolicy"
	for k, v := range setsTocreate {
		iMgr.CreateIPSet(k, v)
	}
	err := iMgr.AddToList("setpod3", []string{"setpod4"})
	if err != nil {
		t.Errorf("AddToList failed with error %s", err.Error())
	}

	for k := range setsTocreate {
		err = iMgr.AddReference(k, networkPolicName, NetPolType)
		if err != nil {
			t.Errorf("AddReference failed with error %s", err.Error())
		}
	}

	if len(iMgr.toAddOrUpdateCache) != 5 {
		t.Errorf("AddReference did not update toAddOrUpdateCache")
	}

	if len(iMgr.toDeleteCache) != 0 {
		t.Errorf("AddReference did not update toDeleteCache")
	}

	for k := range setsTocreate {
		err = iMgr.DeleteReference(k, networkPolicName, NetPolType)
		if err != nil {
			t.Errorf("DeleteReference failed with error %s", err.Error())
		}
	}

	if len(iMgr.toAddOrUpdateCache) != 0 {
		t.Errorf("DeleteReference did not update toAddOrUpdateCache")
	}

	if len(iMgr.toDeleteCache) != 5 {
		t.Errorf("DeleteReference did not update toDeleteCache")
	}

	for k := range setsTocreate {
		iMgr.DeleteIPSet(k)
	}

	// Above delete will not remove setpod3 and setpod4
	// because they are referencing each other
	if len(iMgr.setMap) != 2 {
		t.Errorf("DeleteIPSet did not remove deletable sets")
	}

	err = iMgr.RemoveFromList("setpod3", []string{"setpod4"})
	if err != nil {
		t.Errorf("RemoveFromList failed with error %s", err.Error())
	}

	for k := range setsTocreate {
		iMgr.DeleteIPSet(k)
	}

	for k := range setsTocreate {
		set := iMgr.GetIPSet(k)
		if set != nil {
			t.Errorf("DeleteIPSet did not delete %s IPSet", set.Name)
		}
	}

	for k := range setsTocreate {
		err = iMgr.DeleteReference(k, networkPolicName, NetPolType)
		if err == nil {
			t.Errorf("DeleteReference did not fail with error for ipset %s", k)
		}
	}
}

func TestMain(m *testing.M) {
	metrics.InitializeAll()

	exitCode := m.Run()

	os.Exit(exitCode)
}
