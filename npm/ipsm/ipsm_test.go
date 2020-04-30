// Copyright 2018 Microsoft. All rights reserved.
// MIT License
package ipsm

import (
	"os"
	"testing"

	"github.com/Azure/azure-container-networking/npm/util"
)

func TestSave(t *testing.T) {
	ipsMgr := NewIpsetManager()
	if err := ipsMgr.Save(util.IpsetTestConfigFile); err != nil {
		t.Errorf("TestSave failed @ ipsMgr.Save")
	}
}

func TestRestore(t *testing.T) {
	ipsMgr := NewIpsetManager()
	if err := ipsMgr.Save(util.IpsetTestConfigFile); err != nil {
		t.Errorf("TestRestore failed @ ipsMgr.Save")
	}

	if err := ipsMgr.Restore(util.IpsetTestConfigFile); err != nil {
		t.Errorf("TestRestore failed @ ipsMgr.Restore")
	}
}

func TestCreateList(t *testing.T) {
	ipsMgr := NewIpsetManager()
	if err := ipsMgr.Save(util.IpsetTestConfigFile); err != nil {
		t.Errorf("TestCreateList failed @ ipsMgr.Save")
	}

	defer func() {
		if err := ipsMgr.Restore(util.IpsetTestConfigFile); err != nil {
			t.Errorf("TestCreateList failed @ ipsMgr.Restore")
		}
	}()

	if err := ipsMgr.CreateList("test-list"); err != nil {
		t.Errorf("TestCreateList failed @ ipsMgr.CreateList")
	}
}

func TestDeleteList(t *testing.T) {
	ipsMgr := NewIpsetManager()
	if err := ipsMgr.Save(util.IpsetTestConfigFile); err != nil {
		t.Errorf("TestDeleteList failed @ ipsMgr.Save")
	}

	defer func() {
		if err := ipsMgr.Restore(util.IpsetTestConfigFile); err != nil {
			t.Errorf("TestDeleteList failed @ ipsMgr.Restore")
		}
	}()

	if err := ipsMgr.CreateList("test-list"); err != nil {
		t.Errorf("TestDeleteList failed @ ipsMgr.CreateList")
	}

	if err := ipsMgr.DeleteList("test-list"); err != nil {
		t.Errorf("TestDeleteList failed @ ipsMgr.DeleteList")
	}
}

func TestAddToList(t *testing.T) {
	ipsMgr := NewIpsetManager()
	if err := ipsMgr.Save(util.IpsetTestConfigFile); err != nil {
		t.Errorf("TestAddToList failed @ ipsMgr.Save")
	}

	defer func() {
		if err := ipsMgr.Restore(util.IpsetTestConfigFile); err != nil {
			t.Errorf("TestAddToList failed @ ipsMgr.Restore")
		}
	}()

	if err := ipsMgr.CreateSet("test-set", util.IpsetNetHashFlag); err != nil {
		t.Errorf("TestAddToList failed @ ipsMgr.CreateSet")
	}

	if err := ipsMgr.AddToList("test-list", "test-set"); err != nil {
		t.Errorf("TestAddToList failed @ ipsMgr.AddToList")
	}
}

func TestDeleteFromList(t *testing.T) {
	ipsMgr := NewIpsetManager()
	if err := ipsMgr.Save(util.IpsetTestConfigFile); err != nil {
		t.Errorf("TestDeleteFromList failed @ ipsMgr.Save")
	}

	defer func() {
		if err := ipsMgr.Restore(util.IpsetTestConfigFile); err != nil {
			t.Errorf("TestDeleteFromList failed @ ipsMgr.Restore")
		}
	}()

	if err := ipsMgr.CreateSet("test-set", util.IpsetNetHashFlag); err != nil {
		t.Errorf("TestDeleteFromList failed @ ipsMgr.CreateSet")
	}

	if err := ipsMgr.AddToList("test-list", "test-set"); err != nil {
		t.Errorf("TestDeleteFromList failed @ ipsMgr.AddToList")
	}

	if err := ipsMgr.DeleteFromList("test-list", "test-set"); err != nil {
		t.Errorf("TestDeleteFromList failed @ ipsMgr.DeleteFromList")
	}

	if err := ipsMgr.DeleteSet("test-set"); err != nil {
		t.Errorf("TestDeleteSet failed @ ipsMgr.DeleteSet")
	}
}

func TestCreateSet(t *testing.T) {
	ipsMgr := NewIpsetManager()
	if err := ipsMgr.Save(util.IpsetTestConfigFile); err != nil {
		t.Errorf("TestCreateSet failed @ ipsMgr.Save")
	}

	defer func() {
		if err := ipsMgr.Restore(util.IpsetTestConfigFile); err != nil {
			t.Errorf("TestCreateSet failed @ ipsMgr.Restore")
		}
	}()

	if err := ipsMgr.CreateSet("test-set", util.IpsetNetHashFlag); err != nil {
		t.Errorf("TestCreateSet failed @ ipsMgr.CreateSet")
	}
}

func TestDeleteSet(t *testing.T) {
	ipsMgr := NewIpsetManager()
	if err := ipsMgr.Save(util.IpsetTestConfigFile); err != nil {
		t.Errorf("TestDeleteSet failed @ ipsMgr.Save")
	}

	defer func() {
		if err := ipsMgr.Restore(util.IpsetTestConfigFile); err != nil {
			t.Errorf("TestDeleteSet failed @ ipsMgr.Restore")
		}
	}()

	if err := ipsMgr.CreateSet("test-set", util.IpsetNetHashFlag); err != nil {
		t.Errorf("TestDeleteSet failed @ ipsMgr.CreateSet")
	}

	if err := ipsMgr.DeleteSet("test-set"); err != nil {
		t.Errorf("TestDeleteSet failed @ ipsMgr.DeleteSet")
	}
}

func TestAddToSet(t *testing.T) {
	ipsMgr := NewIpsetManager()
	if err := ipsMgr.Save(util.IpsetTestConfigFile); err != nil {
		t.Errorf("TestAddToSet failed @ ipsMgr.Save")
	}

	defer func() {
		if err := ipsMgr.Restore(util.IpsetTestConfigFile); err != nil {
			t.Errorf("TestAddToSet failed @ ipsMgr.Restore")
		}
	}()

	if err := ipsMgr.AddToSet("test-set", "1.2.3.4", util.IpsetNetHashFlag); err != nil {
		t.Errorf("TestAddToSet failed @ ipsMgr.AddToSet")
	}
}

func TestDeleteFromSet(t *testing.T) {
	ipsMgr := NewIpsetManager()
	if err := ipsMgr.Save(util.IpsetTestConfigFile); err != nil {
		t.Errorf("TestDeleteFromSet failed @ ipsMgr.Save")
	}

	defer func() {
		if err := ipsMgr.Restore(util.IpsetTestConfigFile); err != nil {
			t.Errorf("TestDeleteFromSet failed @ ipsMgr.Restore")
		}
	}()

	if err := ipsMgr.AddToSet("test-set", "1.2.3.4", util.IpsetNetHashFlag); err != nil {
		t.Errorf("TestDeleteFromSet failed @ ipsMgr.AddToSet")
	}

	if err := ipsMgr.DeleteFromSet("test-set", "1.2.3.4"); err != nil {
		t.Errorf("TestDeleteFromSet failed @ ipsMgr.DeleteFromSet")
	}
}

func TestClean(t *testing.T) {
	ipsMgr := NewIpsetManager()
	if err := ipsMgr.Save(util.IpsetTestConfigFile); err != nil {
		t.Errorf("TestClean failed @ ipsMgr.Save")
	}

	defer func() {
		if err := ipsMgr.Restore(util.IpsetTestConfigFile); err != nil {
			t.Errorf("TestClean failed @ ipsMgr.Restore")
		}
	}()

	if err := ipsMgr.CreateSet("test-set", util.IpsetNetHashFlag); err != nil {
		t.Errorf("TestClean failed @ ipsMgr.CreateSet")
	}

	if err := ipsMgr.Clean(); err != nil {
		t.Errorf("TestClean failed @ ipsMgr.Clean")
	}
}

func TestDestroy(t *testing.T) {
	ipsMgr := NewIpsetManager()
	if err := ipsMgr.Save(util.IpsetTestConfigFile); err != nil {
		t.Errorf("TestDestroy failed @ ipsMgr.Save")
	}

	defer func() {
		if err := ipsMgr.Restore(util.IpsetTestConfigFile); err != nil {
			t.Errorf("TestDestroy failed @ ipsMgr.Restore")
		}
	}()

	if err := ipsMgr.AddToSet("test-set", "1.2.3.4", util.IpsetNetHashFlag); err != nil {
		t.Errorf("TestDestroy failed @ ipsMgr.AddToSet")
	}

	if err := ipsMgr.Destroy(); err != nil {
		t.Errorf("TestDestroy failed @ ipsMgr.Destroy")
	}
}

func TestRun(t *testing.T) {
	ipsMgr := NewIpsetManager()
	if err := ipsMgr.Save(util.IpsetTestConfigFile); err != nil {
		t.Errorf("TestRun failed @ ipsMgr.Save")
	}

	defer func() {
		if err := ipsMgr.Restore(util.IpsetTestConfigFile); err != nil {
			t.Errorf("TestRun failed @ ipsMgr.Restore")
		}
	}()

	entry := &ipsEntry{
		operationFlag: util.IpsetCreationFlag,
		set:           "test-set",
		spec:          util.IpsetNetHashFlag,
	}
	if _, err := ipsMgr.Run(entry); err != nil {
		t.Errorf("TestRun failed @ ipsMgr.Run")
	}
}

func TestMain(m *testing.M) {
	ipsMgr := NewIpsetManager()
	ipsMgr.Save(util.IpsetConfigFile)

	exitCode := m.Run()

	ipsMgr.Restore(util.IpsetConfigFile)

	os.Exit(exitCode)
}
