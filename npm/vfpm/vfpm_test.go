// Copyright 2018 Microsoft. All rights reserved.
// MIT License
package vfpm

import (
	"os/exec"
	"strings"
	"testing"
	"unicode"

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

	ports, err := GetPorts()
	if err != nil {
		t.Errorf("TestCreateNLTag failed @ GetPorts")
	}
	if len(ports) == 0 {
		t.Logf("No container ports found.")
		return
	}

	if err := tMgr.CreateNLTag("test-nl-tag", ports[len(ports)-1]); err != nil {
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

	ports, err := GetPorts()
	if err != nil {
		t.Errorf("TestDeleteNLTag failed @ GetPorts")
	}
	if len(ports) == 0 {
		t.Logf("No container ports found.")
		return
	}

	if err := tMgr.CreateNLTag("test-nl-tag", ports[len(ports)-1]); err != nil {
		t.Errorf("TestDeleteNLTag failed @ tMgr.CreateNLTag")
	}

	if err := tMgr.DeleteNLTag("test-nl-tag", ports[len(ports)-1]); err != nil {
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

	ports, err := GetPorts()
	if err != nil {
		t.Errorf("TestAddToNLTag failed @ GetPorts")
	}
	if len(ports) == 0 {
		t.Logf("No container ports found.")
		return
	}

	if err := tMgr.AddToNLTag("test-nl-tag", "test-tag", ports[len(ports)-1]); err != nil {
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

	ports, err := GetPorts()
	if err != nil {
		t.Errorf("TestDeleteFromTag failed @ GetPorts")
	}
	if len(ports) == 0 {
		t.Logf("No container ports found.")
		return
	}

	if err := tMgr.AddToNLTag("test-nl-tag", "test-tag", ports[len(ports)-1]); err != nil {
		t.Errorf("TestDeleteFromNLTag failed @ tMgr.AddToNLTag")
	}

	if err := tMgr.DeleteFromNLTag("test-nl-tag", "test-tag", ports[len(ports)-1]); err != nil {
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

	ports, err := GetPorts()
	if err != nil {
		t.Errorf("TestCreateTag failed @ GetPorts")
	}
	if len(ports) == 0 {
		t.Logf("No container ports found.")
		return
	}

	if err := tMgr.CreateTag("test-tag", ports[len(ports)-1]); err != nil {
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

	ports, err := GetPorts()
	if err != nil {
		t.Errorf("TestDeleteTag failed @ GetPorts")
	}
	if len(ports) == 0 {
		t.Logf("No container ports found.")
		return
	}

	if err := tMgr.CreateTag("test-tag", ports[len(ports)-1]); err != nil {
		t.Errorf("TestDeleteTag failed @ tMgr.CreateTag")
	}

	if err := tMgr.DeleteTag("test-tag", ports[len(ports)-1]); err != nil {
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

	ports, err := GetPorts()
	if err != nil {
		t.Errorf("TestAddToTag failed @ GetPorts")
	}
	if len(ports) == 0 {
		t.Logf("No container ports found.")
		return
	}

	if err := tMgr.AddToTag("test-tag", "1.2.3.4", ports[len(ports)-1]); err != nil {
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

	ports, err := GetPorts()
	if err != nil {
		t.Errorf("TestDeleteFromTag failed @ GetPorts")
	}
	if len(ports) == 0 {
		t.Logf("No container ports found.")
		return
	}

	if err := tMgr.AddToTag("test-tag", "1.2.3.4", ports[len(ports)-1]); err != nil {
		t.Errorf("TestDeleteFromTag failed @ tMgr.AddToTag")
	}

	if err := tMgr.DeleteFromTag("test-tag", "1.2.3.4", ports[len(ports)-1]); err != nil {
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

	ports, err := GetPorts()
	if err != nil {
		t.Errorf("TestTagClean failed @ GetPorts")
	}
	if len(ports) == 0 {
		t.Logf("No container ports found.")
		return
	}

	if err := tMgr.CreateTag("test-tag", ports[len(ports)-1]); err != nil {
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

	ports, err := GetPorts()
	if err != nil {
		t.Errorf("TestTagDestroy failed @ GetPorts")
	}
	if len(ports) == 0 {
		t.Logf("No container ports found.")
		return
	}

	if err := tMgr.AddToTag("test-tag", "1.2.3.4", ports[len(ports)-1]); err != nil {
		t.Errorf("TestTagDestroy failed @ tMgr.AddToTag")
	}

	if err := tMgr.Destroy(); err != nil {
		t.Errorf("TestTagDestroy failed @ tMgr.Destroy")
	}

	tags, _, err := GetTags(ports[len(ports)-1])
	if err != nil {
		t.Errorf("TestTagDestroy failed @ GetTags")
	}
	if len(tags) != 0 {
		t.Errorf("TestTagDestroy failed @ tMgr.Destroy")
	}
}

func TestGetPortByMAC(t *testing.T) {
	// First, get MAC address and port name.
	listPortCmd := exec.Command(util.VFPCmd, util.ListPortCmd)
	out, err := listPortCmd.Output()
	if err != nil {
		t.Errorf("TestGetPortByMAC failed @ listing ports")
	}
	outStr := string(out)

	separated := strings.Split(outStr, util.PortSplit)
	if len(separated) == 0 {
		t.Errorf("TestGetPortByMAC failed because list ports returned empty")
	}
	portStr := separated[len(separated)-1]

	idx := strings.Index(portStr, ":")
	if idx == -1 {
		t.Errorf("TestGetPortByMAC failed @ finding start of portName")
	}

	portName := portStr[idx+2 : idx+2+util.GUIDLength]

	idx = strings.Index(portStr, util.MACAddress)
	for idx != -1 && idx < len(portStr) && portStr[idx] != ':' {
		idx++
	}
	if idx == -1 || idx == len(portStr) {
		t.Errorf("TestGetPortByMAC failed @ finding start of MAC address")
	}
	idx += 2

	var builder strings.Builder
	for idx < len(portStr) && !unicode.IsSpace(rune(portStr[idx])) {
		builder.WriteByte(portStr[idx])
		idx++
	}
	MACAddress := builder.String()

	// Test GetPortByMAC
	port, err := GetPortByMAC(MACAddress)
	if err != nil || port != portName {
		t.Errorf("TestGetPortByMAC failed @ GetPortByMAC")
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

	ports, err := GetPorts()
	if err != nil {
		t.Errorf("TestRuleExists failed @ GetPorts")
	}
	if len(ports) == 0 {
		t.Logf("No container ports found.")
		return
	}

	rule := &Rule{
		Name:     "test",
		Group:    util.NPMIngressGroup,
		SrcIPs:   "1.1.1.1",
		DstIPs:   "2.2.2.2",
		Priority: 0,
		Action:   "allow",
	}

	if _, err := rMgr.Exists(rule, ports[len(ports)-1]); err != nil {
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

	ports, err := GetPorts()
	if err != nil {
		t.Errorf("TestAdd failed @ GetPorts")
	}
	if len(ports) == 0 {
		t.Logf("No container ports found.")
		return
	}

	if err = rMgr.InitAzureNPMLayer(ports[len(ports)-1]); err != nil {
		t.Errorf("TestAdd failed @ rMgr.InitAzureNPMLayer")
	}

	rule := &Rule{
		Name:     "test",
		Group:    util.NPMIngressGroup,
		SrcIPs:   "1.1.1.1",
		DstIPs:   "2.2.2.2",
		Priority: 0,
		Action:   "allow",
	}

	if err := rMgr.Add(rule, ports[len(ports)-1]); err != nil {
		t.Errorf("TestAdd failed @ rMgr.Add")
	}

	if err = rMgr.UnInitAzureNPMLayer(ports[len(ports)-1]); err != nil {
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

	ports, err := GetPorts()
	if err != nil {
		t.Errorf("TestDelete failed @ GetPorts")
	}
	if len(ports) == 0 {
		t.Logf("No container ports found.")
		return
	}

	if err = rMgr.InitAzureNPMLayer(ports[len(ports)-1]); err != nil {
		t.Errorf("TestDelete failed @ rMgr.InitAzureNPMLayer")
	}

	rule := &Rule{
		Name:     "test",
		Group:    util.NPMIngressGroup,
		SrcIPs:   "1.1.1.1",
		DstIPs:   "2.2.2.2",
		Priority: 0,
		Action:   "allow",
	}

	if err := rMgr.Add(rule, ports[len(ports)-1]); err != nil {
		t.Errorf("TestDelete failed @ rMgr.Add")
	}

	if err := rMgr.Delete(rule, ports[len(ports)-1]); err != nil {
		t.Errorf("TestDelete failed @ rMgr.Delete")
	}

	if err = rMgr.UnInitAzureNPMLayer(ports[len(ports)-1]); err != nil {
		t.Errorf("TestDelete failed @ rMgr.UnInitAzureNPMLayer")
	}
}
