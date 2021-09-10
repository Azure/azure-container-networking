package ipsets

import (
	"fmt"
	"os"
	"testing"

	"github.com/Azure/azure-container-networking/npm/metrics"
)

func TestCreateIPSet(t *testing.T) {
	iMgr := NewIPSetManager()
	set := NewIPSet("Test", NameSpace)

	err := iMgr.CreateIPSet(set)
	if err != nil {
		t.Errorf("CreateIPSet() returned error %s", err.Error())
	}
}

func TestAddToSet(t *testing.T) {
	iMgr := NewIPSetManager()
	set := NewIPSet("Test", NameSpace)

	fmt.Println(set.Name)
	err := iMgr.AddToSet([]*IPSet{set}, "10.0.0.0", "test")
	if err != nil {
		t.Errorf("AddToSet() returned error %s", err.Error())
	}
}

func TestRemoveFromSet(t *testing.T) {
	iMgr := NewIPSetManager()
	set := NewIPSet("Test", NameSpace)

	err := iMgr.AddToSet([]*IPSet{set}, "10.0.0.0", "test")
	if err != nil {
		t.Errorf("RemoveFromSet() returned error %s", err.Error())
	}
	err = iMgr.RemoveFromSet([]string{"Test"}, "10.0.0.0", "test")
	if err != nil {
		t.Errorf("RemoveFromSet() returned error %s", err.Error())
	}
}

func TestRemoveFromSetMissing(t *testing.T) {
	iMgr := NewIPSetManager()
	err := iMgr.RemoveFromSet([]string{"Test"}, "10.0.0.0", "test")
	if err == nil {
		t.Errorf("RemoveFromSet() did not return error")
	}
}

func TestAddToListMissing(t *testing.T) {
	iMgr := NewIPSetManager()
	err := iMgr.AddToList("test", []string{"newtest"})
	if err == nil {
		t.Errorf("AddToList() did not return error")
	}
}

func TestAddToList(t *testing.T) {
	iMgr := NewIPSetManager()
	set := NewIPSet("newtest", NameSpace)
	err := iMgr.CreateIPSet(set)
	if err != nil {
		t.Errorf("CreateIPSet() returned error %s", err.Error())
	}

	list := NewIPSet("test", KeyLabelOfNameSpace)
	err = iMgr.CreateIPSet(list)
	if err != nil {
		t.Errorf("CreateIPSet() returned error %s", err.Error())
	}

	err = iMgr.AddToList("test", []string{"newtest"})
	if err != nil {
		t.Errorf("AddToList() returned error %s", err.Error())
	}
}

func TestRemoveFromList(t *testing.T) {
	iMgr := NewIPSetManager()
	set := NewIPSet("newtest", NameSpace)
	err := iMgr.CreateIPSet(set)
	if err != nil {
		t.Errorf("CreateIPSet() returned error %s", err.Error())
	}

	list := NewIPSet("test", KeyLabelOfNameSpace)
	err = iMgr.CreateIPSet(list)
	if err != nil {
		t.Errorf("CreateIPSet() returned error %s", err.Error())
	}

	err = iMgr.AddToList("test", []string{"newtest"})
	if err != nil {
		t.Errorf("AddToList() returned error %s", err.Error())
	}

	err = iMgr.RemoveFromList("test", []string{"newtest"})
	if err != nil {
		t.Errorf("RemoveFromList() returned error %s", err.Error())
	}
}

func TestRemoveFromListMissing(t *testing.T) {
	iMgr := NewIPSetManager()
	err := iMgr.RemoveFromList("test", []string{"newtest"})
	if err == nil {
		t.Errorf("RemoveFromList() did not return error")
	}
}

func TestDeleteList(t *testing.T) {
	iMgr := NewIPSetManager()
	set := NewIPSet("Test", KeyValueLabelOfNameSpace)

	err := iMgr.CreateIPSet(set)
	if err != nil {
		t.Errorf("CreateIPSet() returned error %s", err.Error())
	}

	err = iMgr.DeleteList(set.Name)
	if err != nil {
		t.Errorf("DeleteList() returned error %s", err.Error())
	}
}

func TestDeleteSet(t *testing.T) {
	iMgr := NewIPSetManager()
	set := NewIPSet("Test", NameSpace)

	err := iMgr.CreateIPSet(set)
	if err != nil {
		t.Errorf("CreateIPSet() returned error %s", err.Error())
	}

	err = iMgr.DeleteSet(set.Name)
	if err != nil {
		t.Errorf("DeleteSet() returned error %s", err.Error())
	}
}

func TestMain(m *testing.M) {
	metrics.InitializeAll()

	exitCode := m.Run()

	os.Exit(exitCode)
}
