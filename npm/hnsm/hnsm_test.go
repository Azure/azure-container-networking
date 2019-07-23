// Copyright 2018 Microsoft. All rights reserved.
// MIT License
package hnsm

import (
	"testing"

	"github.com/Azure/azure-container-networking/npm/util"
	hns "github.com/Microsoft/hcsshim"
)

func TestTagSave(t *testing.T) {
	tMgr := NewTagManager()
	if err := tMgr.Save(util.TagTestConfigFile); err != nil {
		t.Errorf("TestTagSave failed @ tMgr.Save")
	}
}

func TestTagRestore(t *testing.T) {
	tMgr := NewTagManager()
	if err := tMgr.Save(util.TagTestConfigFile); err != nil {
		t.Errorf("TestTagRestore failed @ tMgr.Save")
	}

	if err := tMgr.Restore(util.TagTestConfigFile); err != nil {
		t.Errorf("TestTagRestore failed @ tMgr.Restore")
	}
}

func TestCreateNLTag(t *testing.T) {
	tMgr := NewTagManager()
	if err := tMgr.Save(util.TagTestConfigFile); err != nil {
		t.Errorf("TestCreateNLTag failed @ tMgr.Save")
	}

	defer func() {
		if err := tMgr.Restore(util.TagTestConfigFile); err != nil {
			t.Errorf("TestCreateNLTag failed @ tMgr.Restore")
		}
	}()

	if err := tMgr.CreateNLTag("test-nl-tag"); err != nil {
		t.Errorf("TestCreateNLTag failed @ tMgr.CreateNLTag")
	}
}

func TestDeleteNLTag(t *testing.T) {
	tMgr := NewTagManager()
	if err := tMgr.Save(util.TagTestConfigFile); err != nil {
		t.Errorf("TestDeleteNLTag failed @ tMgr.Save")
	}

	defer func() {
		if err := tMgr.Restore(util.TagTestConfigFile); err != nil {
			t.Errorf("TestDeleteNLTag failed @ tMgr.Restore")
		}
	}()

	if err := tMgr.CreateNLTag("test-nl-tag"); err != nil {
		t.Errorf("TestDeleteNLTag failed @ tMgr.CreateNLTag")
	}

	if err := tMgr.DeleteNLTag("test-nl-tag"); err != nil {
		t.Errorf("TestDeleteNLTag failed @ tMgr.DeleteNLTag")
	}
}

func TestAddToNLTag(t *testing.T) {
	tMgr := NewTagManager()
	if err := tMgr.Save(util.TagTestConfigFile); err != nil {
		t.Errorf("TestAddToNLTag failed @ tMgr.Save")
	}

	defer func() {
		if err := tMgr.Restore(util.TagTestConfigFile); err != nil {
			t.Errorf("TestAddToNLTag failed @ tMgr.Restore")
		}
	}()

	if err := tMgr.AddToNLTag("test-nl-tag", "test-tag"); err != nil {
		t.Errorf("TestAddToNLTag failed @ tMgr.AddToNLTag")
	}
}

func TestDeleteFromNLTag(t *testing.T) {
	tMgr := NewTagManager()
	if err := tMgr.Save(util.TagTestConfigFile); err != nil {
		t.Errorf("TestDeleteFromNLTag failed @ tMgr.Save")
	}

	defer func() {
		if err := tMgr.Restore(util.TagTestConfigFile); err != nil {
			t.Errorf("TestDeleteFromNLTag failed @ tMgr.Restore")
		}
	}()

	if err := tMgr.AddToNLTag("test-nl-tag", "test-tag"); err != nil {
		t.Errorf("TestDeleteFromNLTag failed @ tMgr.AddToNLTag")
	}

	if err := tMgr.DeleteFromNLTag("test-nl-tag", "test-tag"); err != nil {
		t.Errorf("TestDeleteFromNLTag failed @ tMgr.DeleteFromNLTag")
	}
}

func TestCreateTag(t *testing.T) {
	tMgr := NewTagManager()
	if err := tMgr.Save(util.TagTestConfigFile); err != nil {
		t.Errorf("TestCreateTag failed @ tMgr.Save")
	}

	defer func() {
		if err := tMgr.Restore(util.TagTestConfigFile); err != nil {
			t.Errorf("TestCreateTag failed @ tMgr.Restore")
		}
	}()

	if err := tMgr.CreateTag("test-tag"); err != nil {
		t.Errorf("TestCreateTag failed @ tMgr.CreateTag")
	}
}

func TestDeleteTag(t *testing.T) {
	tMgr := NewTagManager()
	if err := tMgr.Save(util.TagTestConfigFile); err != nil {
		t.Errorf("TestDeleteTag failed @ tMgr.Save")
	}

	defer func() {
		if err := tMgr.Restore(util.TagTestConfigFile); err != nil {
			t.Errorf("TestDeleteTag failed @ tMgr.Restore")
		}
	}()

	if err := tMgr.CreateTag("test-tag"); err != nil {
		t.Errorf("TestDeleteTag failed @ tMgr.CreateTag")
	}

	if err := tMgr.DeleteTag("test-tag"); err != nil {
		t.Errorf("TestDeleteTag failed @ tMgr.DeleteTag")
	}
}

func TestAddToTag(t *testing.T) {
	tMgr := NewTagManager()
	if err := tMgr.Save(util.TagTestConfigFile); err != nil {
		t.Errorf("TestAddToTag failed @ tMgr.Save")
	}

	defer func() {
		if err := tMgr.Restore(util.TagTestConfigFile); err != nil {
			t.Errorf("TestAddToTag failed @ tMgr.Restore")
		}
	}()

	if err := tMgr.AddToTag("test-tag", "1.2.3.4"); err != nil {
		t.Errorf("TestAddToTag failed @ tMgr.AddToTag")
	}
}

func TestDeleteFromTag(t *testing.T) {
	tMgr := NewTagManager()
	if err := tMgr.Save(util.TagTestConfigFile); err != nil {
		t.Errorf("TestDeleteFromTag failed @ tMgr.Save")
	}

	defer func() {
		if err := tMgr.Restore(util.TagTestConfigFile); err != nil {
			t.Errorf("TestDeleteFromTag failed @ tMgr.Restore")
		}
	}()

	if err := tMgr.AddToTag("test-tag", "1.2.3.4"); err != nil {
		t.Errorf("TestDeleteFromTag failed @ tMgr.AddToTag")
	}

	if err := tMgr.DeleteFromTag("test-tag", "1.2.3.4"); err != nil {
		t.Errorf("TestDeleteFromTag failed @ tMgr.DeleteFromTag")
	}
}

func TestTagClean(t *testing.T) {
	tMgr := NewTagManager()
	if err := tMgr.Save(util.TagTestConfigFile); err != nil {
		t.Errorf("TestTagClean failed @ tMgr.Save")
	}

	defer func() {
		if err := tMgr.Restore(util.TagTestConfigFile); err != nil {
			t.Errorf("TestTagClean failed @ tMgr.Restore")
		}
	}()

	if err := tMgr.CreateTag("test-tag"); err != nil {
		t.Errorf("TestTagClean failed @ tMgr.CreateTag")
	}

	if err := tMgr.Clean(); err != nil {
		t.Errorf("TestTagClean failed @ tMgr.Clean")
	}
}

func TestTagDestroy(t *testing.T) {
	tMgr := NewTagManager()
	if err := tMgr.Save(util.TagTestConfigFile); err != nil {
		t.Errorf("TestTagDestroy failed @ tMgr.Save")
	}

	defer func() {
		if err := tMgr.Restore(util.TagTestConfigFile); err != nil {
			t.Errorf("TestTagDestroy failed @ tMgr.Restore")
		}
	}()

	if err := tMgr.AddToTag("test-tag", "1.2.3.4"); err != nil {
		t.Errorf("TestTagDestroy failed @ tMgr.AddToTag")
	}

	if err := tMgr.Destroy(); err != nil {
		t.Errorf("TestTagDestroy failed @ tMgr.Destroy")
	}
}

func TestACLSave(t *testing.T) {
	aclMgr := &ACLPolicyManager{}
	if err := aclMgr.Save(util.ACLTestConfigFile); err != nil {
		t.Errorf("TestACLSave failed @ aclMgr.Save")
	}
}

func TestACLRestore(t *testing.T) {
	aclMgr := &ACLPolicyManager{}
	if err := aclMgr.Save(util.ACLTestConfigFile); err != nil {
		t.Errorf("TestACLRestore failed @ aclMgr.Save")
	}

	if err := aclMgr.Restore(util.ACLTestConfigFile); err != nil {
		t.Errorf("TestACLRestore failed @ aclMgr.Restore")
	}
}

func TestACLExists(t *testing.T) {
	aclMgr := &ACLPolicyManager{}
	if err := aclMgr.Save(util.ACLTestConfigFile); err != nil {
		t.Errorf("TestACLExists failed @ aclMgr.Save")
	}

	defer func() {
		if err := aclMgr.Restore(util.ACLTestConfigFile); err != nil {
			t.Errorf("TestACLExists failed @ aclMgr.Restore")
		}
	}()

	policy := &hns.ACLPolicy{
		Id: "test-acl",
	}
	if _, err := aclMgr.Exists(policy); err != nil {
		t.Errorf("TestACLExists failed @ aclMgr.Exists")
	}
}

func TestAdd(t *testing.T) {
	aclMgr := &ACLPolicyManager{}
	if err := aclMgr.Save(util.ACLTestConfigFile); err != nil {
		t.Errorf("TestAdd failed @ aclMgr.Save")
	}

	defer func() {
		if err := aclMgr.Restore(util.ACLTestConfigFile); err != nil {
			t.Errorf("TestAdd failed @ aclMgr.Restore")
		}
	}()

	policy := &hns.ACLPolicy{
		Id: "test-acl",
	}
	if err := aclMgr.Add(policy); err != nil {
		t.Errorf("TestAdd failed @ aclMgr.Add")
	}
}

func TestDelete(t *testing.T) {
	aclMgr := &ACLPolicyManager{}
	if err := aclMgr.Save(util.ACLTestConfigFile); err != nil {
		t.Errorf("TestDelete failed @ aclMgr.Save")
	}

	defer func() {
		if err := aclMgr.Restore(util.ACLTestConfigFile); err != nil {
			t.Errorf("TestDelete failed @ aclMgr.Restore")
		}
	}()

	policy := &hns.ACLPolicy{
		Id: "test-acl",
	}
	if err := aclMgr.Add(policy); err != nil {
		t.Errorf("TestDelete failed @ aclMgr.Add")
	}

	if err := aclMgr.Delete(policy); err != nil {
		t.Errorf("TestDelete failed @ aclMgr.Delete")
	}
}
