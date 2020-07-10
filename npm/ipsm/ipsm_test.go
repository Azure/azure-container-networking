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

	if err := ipsMgr.CreateSet("test-set", append([]string{util.IpsetNetHashFlag})); err != nil {
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

	if err := ipsMgr.CreateSet("test-set", append([]string{util.IpsetNetHashFlag})); err != nil {
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

	if err := ipsMgr.CreateSet("test-set", []string{util.IpsetNetHashFlag}); err != nil {
		t.Errorf("TestCreateSet failed @ ipsMgr.CreateSet")
	}

	spec := append([]string{util.IpsetNetHashFlag, util.IpsetMaxelemName, util.IpsetMaxelemNum})
	if err := ipsMgr.CreateSet("test-set-with-maxelem", spec); err != nil {
		t.Errorf("TestCreateSet failed @ ipsMgr.CreateSet when set maxelem")
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

	if err := ipsMgr.CreateSet("test-set", append([]string{util.IpsetNetHashFlag})); err != nil {
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

	if err := ipsMgr.AddToSet("test-set", "1.2.3.4", util.IpsetNetHashFlag, ""); err != nil {
		t.Errorf("TestAddToSet failed @ ipsMgr.AddToSet")
	}

	if err := ipsMgr.AddToSet("test-set", "1.2.3.4/nomatch", util.IpsetNetHashFlag, ""); err != nil {
		t.Errorf("TestAddToSet with nomatch failed @ ipsMgr.AddToSet")
	}
}

func TestAddToSetWithCachePodInfo(t *testing.T) {
	ipsMgr := NewIpsetManager()
	if err := ipsMgr.Save(util.IpsetTestConfigFile); err != nil {
		t.Errorf("TestAddToSetWithCachePodInfo failed @ ipsMgr.Save")
	}

	defer func() {
		if err := ipsMgr.Restore(util.IpsetTestConfigFile); err != nil {
			t.Errorf("TestAddToSetWithCachePodInfo failed @ ipsMgr.Restore")
		}
	}()

	var pod1 = "pod1"
	var setname = "test-podcache_new"
	var ip = "10.0.2.7"
	if err := ipsMgr.AddToSet(setname, ip, util.IpsetNetHashFlag, pod1); err != nil {
		t.Errorf("TestAddToSetWithCachePodInfo with pod1 failed @ ipsMgr.AddToSet, setname: %s, hashedname: %s", setname, util.GetHashedName(setname))
	}

	// validate if Pod1 exists
	cachedPodUid := ipsMgr.setMap[setname].elements[ip]
	if cachedPodUid != pod1 {
		t.Errorf("setname: %s, hashedname: %s is added with wrong podUid: %s, expected: %s", setname, util.GetHashedName(setname), cachedPodUid, pod1)
	}

	// now add pod2 with the same ip. This is possible if DeletePod1 is handled after AddPod2 event callback.
	var pod2 = "pod2"
	if err := ipsMgr.AddToSet(setname, ip, util.IpsetNetHashFlag, pod2); err != nil {
		t.Errorf("TestAddToSetWithCachePodInfo with pod2 failed @ ipsMgr.AddToSet")
	}

	cachedPodUid = ipsMgr.setMap[setname].elements[ip]
	if cachedPodUid != pod2 {
		t.Errorf("setname: %s, hashedname: %s is added with wrong podUid: %s, expected: %s", setname, util.GetHashedName(setname), cachedPodUid, pod2)
	}

	// Delete from set, it will delete the set if this is the last member
	ipsMgr.DeleteFromSet(setname, ip, pod2)
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

	if err := ipsMgr.AddToSet("test-set", "1.2.3.4", util.IpsetNetHashFlag, ""); err != nil {
		t.Errorf("TestDeleteFromSet failed @ ipsMgr.AddToSet")
	}

	if len(ipsMgr.setMap["test-set"].elements) != 1 {
		t.Errorf("TestDeleteFromSet failed @ ipsMgr.AddToSet")
	}

	if err := ipsMgr.DeleteFromSet("test-set", "1.2.3.4", ""); err != nil {
		t.Errorf("TestDeleteFromSet failed @ ipsMgr.DeleteFromSet")
	}

	// After deleting the only entry, "1.2.3.4" from "test-set", "test-set" ipset won't exist
	if _, exists := ipsMgr.setMap["test-set"]; exists {
		t.Errorf("TestDeleteFromSet failed @ ipsMgr.DeleteFromSet")
	}
}

func TestDeleteFromSetWithPodCache(t *testing.T) {
	ipsMgr := NewIpsetManager()
	if err := ipsMgr.Save(util.IpsetTestConfigFile); err != nil {
		t.Errorf("TestDeleteFromSetWithPodCache failed @ ipsMgr.Save")
	}

	defer func() {
		if err := ipsMgr.Restore(util.IpsetTestConfigFile); err != nil {
			t.Errorf("TestDeleteFromSetWithPodCache failed @ ipsMgr.Restore")
		}
	}()

	var setname = "test-deleteset-withcache"
	var ip = "10.0.2.8"
	var pod1 = "pod1"
	if err := ipsMgr.AddToSet(setname, ip, util.IpsetNetHashFlag, pod1); err != nil {
		t.Errorf("TestDeleteFromSetWithPodCache failed for pod1 @ ipsMgr.AddToSet")
	}

	if len(ipsMgr.setMap[setname].elements) != 1 {
		t.Errorf("TestDeleteFromSetWithPodCache failed @ ipsMgr.AddToSet")
	}

	if err := ipsMgr.DeleteFromSet(setname, ip, pod1); err != nil {
		t.Errorf("TestDeleteFromSetWithPodCache for pod1 failed @ ipsMgr.DeleteFromSet")
	}

	// now add the set again and then replace it with pod2
	var pod2 = "pod2"
	if err := ipsMgr.AddToSet(setname, ip, util.IpsetNetHashFlag, pod1); err != nil {
		t.Errorf("TestDeleteFromSetWithPodCache failed for pod1 @ ipsMgr.AddToSet")
	}

	// Add Pod2 with same ip (This could happen if AddPod2 is served before DeletePod1)
	if err := ipsMgr.AddToSet(setname, ip, util.IpsetNetHashFlag, pod2); err != nil {
		t.Errorf("TestDeleteFromSetWithPodCache failed for pod2 @ ipsMgr.AddToSet")
	}

	// Process DeletePod1
	if err := ipsMgr.DeleteFromSet(setname, ip, pod1); err != nil {
		t.Errorf("TestDeleteFromSetWithPodCache for pod1 failed @ ipsMgr.DeleteFromSet")
	}

	// note the set will stil exist with pod ip
	cachedPodUid := ipsMgr.setMap[setname].elements[ip]
	if cachedPodUid != pod2 {
		t.Errorf("setname: %s, hashedname: %s is added with wrong podUid: %s, expected: %s", setname, util.GetHashedName(setname), cachedPodUid, pod2)
	}

	// Now cleanup and delete pod2
	if err := ipsMgr.DeleteFromSet(setname, ip, pod2); err != nil {
		t.Errorf("TestDeleteFromSetWithPodCache for pod2 failed @ ipsMgr.DeleteFromSet")
	}

	if _, exists := ipsMgr.setMap[setname]; exists {
		t.Errorf("TestDeleteFromSetWithPodCache failed @ ipsMgr.DeleteFromSet")
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

	if err := ipsMgr.CreateSet("test-set", append([]string{util.IpsetNetHashFlag})); err != nil {
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

	if err := ipsMgr.AddToSet("test-set", "1.2.3.4", util.IpsetNetHashFlag, ""); err != nil {
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
		spec:          append([]string{util.IpsetNetHashFlag}),
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
