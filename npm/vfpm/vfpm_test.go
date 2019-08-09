// Copyright 2018 Microsoft. All rights reserved.
// MIT License
package vfpm

import (
	"testing"

	"github.com/kalebmorris/azure-container-networking/npm/util"
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

	ports, err := getPorts()
	if err != nil {
		t.Errorf("TestCreateNLTag failed @ getPorts")
	}

	if err := tMgr.CreateNLTag("test-nl-tag", ports[0]); err != nil {
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

	ports, err := getPorts()
	if err != nil {
		t.Errorf("TestDeleteNLTag failed @ getPorts")
	}

	if err := tMgr.CreateNLTag("test-nl-tag", ports[0]); err != nil {
		t.Errorf("TestDeleteNLTag failed @ tMgr.CreateNLTag")
	}

	if err := tMgr.DeleteNLTag("test-nl-tag", ports[0]); err != nil {
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

	ports, err := getPorts()
	if err != nil {
		t.Errorf("TestAddToNLTag failed @ getPorts")
	}

	if err := tMgr.AddToNLTag("test-nl-tag", "test-tag", ports[0]); err != nil {
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

	ports, err := getPorts()
	if err != nil {
		t.Errorf("TestDeleteFromTag failed @ getPorts")
	}

	if err := tMgr.AddToNLTag("test-nl-tag", "test-tag", ports[0]); err != nil {
		t.Errorf("TestDeleteFromNLTag failed @ tMgr.AddToNLTag")
	}

	if err := tMgr.DeleteFromNLTag("test-nl-tag", "test-tag", ports[0]); err != nil {
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

	ports, err := getPorts()
	if err != nil {
		t.Errorf("TestCreateTag failed @ getPorts")
	}

	if err := tMgr.CreateTag("test-tag", ports[0]); err != nil {
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

	ports, err := getPorts()
	if err != nil {
		t.Errorf("TestDeleteTag failed @ getPorts")
	}

	if err := tMgr.CreateTag("test-tag", ports[0]); err != nil {
		t.Errorf("TestDeleteTag failed @ tMgr.CreateTag")
	}

	if err := tMgr.DeleteTag("test-tag", ports[0]); err != nil {
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

	ports, err := getPorts()
	if err != nil {
		t.Errorf("TestAddToTag failed @ getPorts")
	}

	if err := tMgr.AddToTag("test-tag", "1.2.3.4", ports[0]); err != nil {
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

	ports, err := getPorts()
	if err != nil {
		t.Errorf("TestDeleteFromTag failed @ getPorts")
	}

	if err := tMgr.AddToTag("test-tag", "1.2.3.4", ports[0]); err != nil {
		t.Errorf("TestDeleteFromTag failed @ tMgr.AddToTag")
	}

	if err := tMgr.DeleteFromTag("test-tag", "1.2.3.4", ports[0]); err != nil {
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

	ports, err := getPorts()
	if err != nil {
		t.Errorf("TestTagClean failed @ getPorts")
	}

	if err := tMgr.CreateTag("test-tag", ports[0]); err != nil {
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

	ports, err := getPorts()
	if err != nil {
		t.Errorf("TestTagDestroy failed @ getPorts")
	}

	if err := tMgr.AddToTag("test-tag", "1.2.3.4", ports[0]); err != nil {
		t.Errorf("TestTagDestroy failed @ tMgr.AddToTag")
	}

	if err := tMgr.Destroy(); err != nil {
		t.Errorf("TestTagDestroy failed @ tMgr.Destroy")
	}
}

func TestRuleSave(t *testing.T) {
	rMgr := &RuleManager{}
	if err := rMgr.Save(util.RuleTestConfigFile); err != nil {
		t.Errorf("TestRuleSave failed @ rMgr.Save")
	}
}

func TestRuleRestore(t *testing.T) {
	rMgr := &RuleManager{}
	if err := rMgr.Save(util.RuleTestConfigFile); err != nil {
		t.Errorf("TestRuleRestore failed @ rMgr.Save")
	}

	if err := rMgr.Restore(util.RuleTestConfigFile); err != nil {
		t.Errorf("TestRuleRestore failed @ rMgr.Restore")
	}
}

func TestRuleExists(t *testing.T) {
	rMgr := &RuleManager{}
	if err := rMgr.Save(util.RuleTestConfigFile); err != nil {
		t.Errorf("TestRuleExists failed @ rMgr.Save")
	}

	defer func() {
		if err := rMgr.Restore(util.RuleTestConfigFile); err != nil {
			t.Errorf("TestRuleExists failed @ rMgr.Restore")
		}
	}()

	ports, err := getPorts()
	if err != nil {
		t.Errorf("TestRuleExists failed @ getPorts")
	}

	rule := &Rule{
		name: "test",
		group: util.NPMIngressGroup,
		srcIPs: "1.1.1.1",
		dstIPs: "2.2.2.2",
		priority: "0",
		action: "allow",
	}

	if _, err := rMgr.Exists(rule, ports[0]); err != nil {
		t.Errorf("TestRuleExists failed @ rMgr.Exists")
	}
}

func TestAdd(t *testing.T) {
	rMgr := &RuleManager{}
	if err := rMgr.Save(util.RuleTestConfigFile); err != nil {
		t.Errorf("TestAdd failed @ rMgr.Save")
	}

	defer func() {
		if err := rMgr.Restore(util.RuleTestConfigFile); err != nil {
			t.Errorf("TestAdd failed @ rMgr.Restore")
		}
	}()

	ports, err := getPorts()
	if err != nil {
		t.Errorf("TestAdd failed @ getPorts")
	}

	if err = rMgr.InitAzureNPMLayer(ports[0]); err != nil {
		t.Errorf("TestAdd failed @ rMgr.InitAzureNPMLayer")
	}

	rule := &Rule{
		name: "test",
		group: util.NPMIngressGroup,
		srcIPs: "1.1.1.1",
		dstIPs: "2.2.2.2",
		priority: "0",
		action: "allow",
	}

	if err := rMgr.Add(rule, ports[0]); err != nil {
		t.Errorf("TestAdd failed @ rMgr.Add")
	}

	if err = rMgr.UnInitAzureNPMLayer(ports[0]); err != nil {
		t.Errorf("TestAdd failed @ rMgr.UnInitAzureNPMLayer")
	}
}

func TestDelete(t *testing.T) {
	rMgr := &RuleManager{}
	if err := rMgr.Save(util.RuleTestConfigFile); err != nil {
		t.Errorf("TestDelete failed @ rMgr.Save")
	}

	defer func() {
		if err := rMgr.Restore(util.RuleTestConfigFile); err != nil {
			t.Errorf("TestDelete failed @ rMgr.Restore")
		}
	}()

	ports, err := getPorts()
	if err != nil {
		t.Errorf("TestDelete failed @ getPorts")
	}

	if err = rMgr.InitAzureNPMLayer(ports[0]); err != nil {
		t.Errorf("TestDelete failed @ rMgr.InitAzureNPMLayer")
	}

	rule := &Rule{
		name: "test",
		group: util.NPMIngressGroup,
		srcIPs: "1.1.1.1",
		dstIPs: "2.2.2.2",
		priority: "0",
		action: "allow",
	}

	if err := rMgr.Add(rule, ports[0]); err != nil {
		t.Errorf("TestDelete failed @ rMgr.Add")
	}

	if err := rMgr.Delete(rule, ports[0]); err != nil {
		t.Errorf("TestDelete failed @ rMgr.Delete")
	}
	
	if err = rMgr.UnInitAzureNPMLayer(ports[0]); err != nil {
		t.Errorf("TestDelete failed @ rMgr.UnInitAzureNPMLayer")
	}
}
