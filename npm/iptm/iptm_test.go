package iptm

import (
	"testing"

	"github.com/Azure/azure-container-networking/npm/util"
)

func TestSave(t *testing.T) {
	iptMgr := &IptablesManager{}
	if err := iptMgr.Save(util.IptablesTestConfigFile); err != nil {
		t.Errorf("TestSave failed @ iptMgr.Save")
	}
}

func TestRestore(t *testing.T) {
	iptMgr := &IptablesManager{}
	if err := iptMgr.Save(util.IptablesTestConfigFile); err != nil {
		t.Errorf("TestRestore failed @ iptMgr.Save")
	}

	if err := iptMgr.Restore(util.IptablesTestConfigFile); err != nil {
		t.Errorf("TestRestore failed @ iptMgr.Restore")
	}
}

func TestInitNpmChains(t *testing.T) {
	iptMgr := &IptablesManager{}

	if err := iptMgr.Save(util.IptablesTestConfigFile); err != nil {
		t.Errorf("TestInitNpmChains failed @ iptMgr.Save")
	}

	defer func() {
		if err := iptMgr.Restore(util.IptablesTestConfigFile); err != nil {
			t.Errorf("TestInitNpmChains failed @ iptMgr.Restore")
		}
	}()

	if err := iptMgr.InitNpmChains(); err != nil {
		t.Errorf("TestInitNpmChains @ iptMgr.InitNpmChains")
	}
}

func TestUninitNpmChains(t *testing.T) {
	iptMgr := &IptablesManager{}

	if err := iptMgr.Save(util.IptablesTestConfigFile); err != nil {
		t.Errorf("TestUninitNpmChains failed @ iptMgr.Save")
	}

	defer func() {
		if err := iptMgr.Restore(util.IptablesTestConfigFile); err != nil {
			t.Errorf("TestUninitNpmChains failed @ iptMgr.Restore")
		}
	}()

	if err := iptMgr.InitNpmChains(); err != nil {
		t.Errorf("TestUninitNpmChains @ iptMgr.InitNpmChains")
	}

	if err := iptMgr.UninitNpmChains(); err != nil {
		t.Errorf("TestUninitNpmChains @ iptMgr.UninitNpmChains")
	}
}

func TestExists(t *testing.T) {
	iptMgr := &IptablesManager{}
	if err := iptMgr.Save(util.IptablesTestConfigFile); err != nil {
		t.Errorf("TestExists failed @ iptMgr.Save")
	}

	defer func() {
		if err := iptMgr.Restore(util.IptablesTestConfigFile); err != nil {
			t.Errorf("TestExists failed @ iptMgr.Restore")
		}
	}()

	iptMgr.OperationFlag = util.IptablesCheckFlag
	entry := &IptEntry{
		Chain: util.IptablesForwardChain,
		Specs: []string{
			util.IptablesJumpFlag,
			util.IptablesAccept,
		},
	}
	if _, err := iptMgr.Exists(entry); err != nil {
		t.Errorf("TestExists failed @ iptMgr.Exists")
	}
}

func TestAddChain(t *testing.T) {
	iptMgr := &IptablesManager{}
	if err := iptMgr.Save(util.IptablesTestConfigFile); err != nil {
		t.Errorf("TestAddChain failed @ iptMgr.Save")
	}

	defer func() {
		if err := iptMgr.Restore(util.IptablesTestConfigFile); err != nil {
			t.Errorf("TestAddChain failed @ iptMgr.Restore")
		}
	}()

	if err := iptMgr.AddChain("TEST-CHAIN"); err != nil {
		t.Errorf("TestAddChain failed @ iptMgr.AddChain")
	}
}

func TestDeleteChain(t *testing.T) {
	iptMgr := &IptablesManager{}
	if err := iptMgr.Save(util.IptablesTestConfigFile); err != nil {
		t.Errorf("TestDeleteChain failed @ iptMgr.Save")
	}

	defer func() {
		if err := iptMgr.Restore(util.IptablesTestConfigFile); err != nil {
			t.Errorf("TestDeleteChain failed @ iptMgr.Restore")
		}
	}()

	if err := iptMgr.AddChain("TEST-CHAIN"); err != nil {
		t.Errorf("TestDeleteChain failed @ iptMgr.AddChain")
	}

	if err := iptMgr.DeleteChain("TEST-CHAIN"); err != nil {
		t.Errorf("TestDeleteChain failed @ iptMgr.DeleteChain")
	}
}

func TestAdd(t *testing.T) {
	iptMgr := &IptablesManager{}
	if err := iptMgr.Save(util.IptablesTestConfigFile); err != nil {
		t.Errorf("TestAdd failed @ iptMgr.Save")
	}

	defer func() {
		if err := iptMgr.Restore(util.IptablesTestConfigFile); err != nil {
			t.Errorf("TestAdd failed @ iptMgr.Restore")
		}
	}()

	entry := &IptEntry{
		Chain: util.IptablesForwardChain,
		Specs: []string{
			util.IptablesJumpFlag,
			util.IptablesReject,
		},
	}
	if err := iptMgr.Add(entry); err != nil {
		t.Errorf("TestAdd failed @ iptMgr.Add")
	}
}

func TestDelete(t *testing.T) {
	iptMgr := &IptablesManager{}
	if err := iptMgr.Save(util.IptablesTestConfigFile); err != nil {
		t.Errorf("TestDelete failed @ iptMgr.Save")
	}

	defer func() {
		if err := iptMgr.Restore(util.IptablesTestConfigFile); err != nil {
			t.Errorf("TestDelete failed @ iptMgr.Restore")
		}
	}()

	entry := &IptEntry{
		Chain: util.IptablesForwardChain,
		Specs: []string{
			util.IptablesJumpFlag,
			util.IptablesReject,
		},
	}
	if err := iptMgr.Add(entry); err != nil {
		t.Errorf("TestDelete failed @ iptMgr.Add")
	}

	if err := iptMgr.Delete(entry); err != nil {
		t.Errorf("TestDelete failed @ iptMgr.Delete")
	}
}

func TestRun(t *testing.T) {
	iptMgr := &IptablesManager{}
	if err := iptMgr.Save(util.IptablesTestConfigFile); err != nil {
		t.Errorf("TestRun failed @ iptMgr.Save")
	}

	defer func() {
		if err := iptMgr.Restore(util.IptablesTestConfigFile); err != nil {
			t.Errorf("TestRun failed @ iptMgr.Restore")
		}
	}()

	iptMgr.OperationFlag = util.IptablesChainCreationFlag
	entry := &IptEntry{
		Chain: "TEST-CHAIN",
	}
	if _, err := iptMgr.Run(entry); err != nil {
		t.Errorf("TestRun failed @ iptMgr.Run")
	}
}

func TestMain(m *testing.M) {
	iptMgr := NewIptablesManager()
	iptMgr.Save(util.IptablesConfigFile)

	m.Run()

	iptMgr.Restore(util.IptablesConfigFile)
}
