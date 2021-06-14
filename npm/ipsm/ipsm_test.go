// Copyright 2018 Microsoft. All rights reserved.
// MIT License
package ipsm

import (
	"fmt"
	"log"
	"os"
	"testing"

	"github.com/Azure/azure-container-networking/npm/iptm"
	"github.com/Azure/azure-container-networking/npm/metrics"
	"github.com/Azure/azure-container-networking/npm/metrics/promutil"
	"github.com/Azure/azure-container-networking/npm/util"
	testutils "github.com/Azure/azure-container-networking/test/utils"
	"github.com/stretchr/testify/require"
	"k8s.io/utils/exec"
)

func TestSave(t *testing.T) {
	ipsMgr := NewIpsetManager(exec.New())
	if err := ipsMgr.Save(util.IpsetTestConfigFile); err != nil {
		t.Errorf("TestSave failed @ ipsMgr.Save")
	}
}

func TestRestore(t *testing.T) {
	ipsMgr := NewIpsetManager(exec.New())

	if err := ipsMgr.Save(util.IpsetTestConfigFile); err != nil {
		t.Errorf("TestRestore failed @ ipsMgr.Save with err %v", err)
	}

	if err := ipsMgr.Restore(util.IpsetTestConfigFile); err != nil {
		t.Errorf("TestRestore failed @ ipsMgr.Restore with err %v", err)
	}
}

func TestCreateList(t *testing.T) {
	ipsMgr := NewIpsetManager(exec.New())
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
	ipsMgr := NewIpsetManager(exec.New())
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
	ipsMgr := NewIpsetManager(exec.New())
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
	var calls = []testutils.TestCmd{
		{Cmd: []string{"ipset", "-N", "-exist", util.GetHashedName("test-set"), "nethash"}},
		{Cmd: []string{"ipset", "list", "-exist", util.GetHashedName("test-set")}},
		{Cmd: []string{"ipset", "-N", "-exist", util.GetHashedName("test-list"), "setlist"}},
		{Cmd: []string{"ipset", "-A", "-exist", util.GetHashedName("test-list"), util.GetHashedName("test-set")}},
		{Cmd: []string{"ipset", "test", "-exist", util.GetHashedName("test-list"), util.GetHashedName("test-set")}},
		{Cmd: []string{"ipset", "-D", "-exist", util.GetHashedName("test-list"), util.GetHashedName("test-set")}},
		{Cmd: []string{"ipset", "-X", "-exist", util.GetHashedName("test-list")}},
		{Cmd: []string{"ipset", "test", "-exist", util.GetHashedName("test-list"), util.GetHashedName("test-set")}, Stdout: "ipset still exists", ExitCode: 2},
		{Cmd: []string{"ipset", "list", "-exist", util.GetHashedName("test-list")}, Stdout: "ipset still exists", ExitCode: 2},
		{Cmd: []string{"ipset", "-X", "-exist", util.GetHashedName("test-set")}},
		{Cmd: []string{"ipset", "list", "-exist", util.GetHashedName("test-set")}, Stdout: "ipset still exists", ExitCode: 2},
	}

	fexec, fcmd := testutils.GetFakeExecWithScripts(calls)

	ipsMgr := NewIpsetManager(fexec)

	// Create set and validate set is created.
	setName := "test-set"
	if err := ipsMgr.CreateSet(setName, append([]string{util.IpsetNetHashFlag})); err != nil {
		t.Errorf("TestDeleteFromList failed @ ipsMgr.CreateSet")
	}

	entry := &ipsEntry{
		operationFlag: util.IPsetCheckListFlag,
		set:           util.GetHashedName(setName),
	}

	if _, err := ipsMgr.Run(entry); err != nil {
		t.Errorf("TestDeleteFromList failed @ ipsMgr.CreateSet since %s not exist in kernel", setName)
	}

	// Create list, add set to list and validate set is in the list.
	listName := "test-list"
	if err := ipsMgr.AddToList(listName, setName); err != nil {
		t.Errorf("TestDeleteFromList failed @ ipsMgr.AddToList")
	}

	entry = &ipsEntry{
		operationFlag: util.IpsetTestFlag,
		set:           util.GetHashedName(listName),
		spec:          append([]string{util.GetHashedName(setName)}),
	}

	if _, err := ipsMgr.Run(entry); err != nil {
		t.Errorf("TestDeleteFromList failed @ ipsMgr.AddToList since %s not exist in %s set", listName, setName)
	}

	// Delete set from list and validate set is not in list anymore.
	if err := ipsMgr.DeleteFromList(listName, setName); err != nil {
		t.Errorf("TestDeleteFromList failed @ ipsMgr.DeleteFromList %v", err)
	}

	// Delete set from list and validate set is not in list anymore.
	if err := ipsMgr.DeleteFromList(listName, "nonexistentsetname"); err != nil {
		t.Errorf("TestDeleteFromList failed @ ipsMgr.DeleteFromList %v", err)
	}

	// Delete set from list, but list isn't of list type
	if err := ipsMgr.DeleteFromList(setName, setName); err != nil {
		t.Errorf("TestDeleteFromList failed @ ipsMgr.DeleteFromList %v", err)
	}

	entry = &ipsEntry{
		operationFlag: util.IpsetTestFlag,
		set:           util.GetHashedName(listName),
		spec:          append([]string{util.GetHashedName(setName)}),
	}

	if _, err := ipsMgr.Run(entry); err == nil {
		t.Errorf("TestDeleteFromList failed @ ipsMgr.DeleteFromList since %s still exist in %s set", listName, setName)
	}

	// Delete List and validate list is not exist.

	if err := ipsMgr.DeleteSet(listName); err != nil {
		t.Errorf("TestDeleteSet failed @ ipsMgr.DeleteSet")
	}

	entry = &ipsEntry{
		operationFlag: util.IPsetCheckListFlag,
		set:           util.GetHashedName(listName),
	}

	if _, err := ipsMgr.Run(entry); err == nil {
		t.Errorf("TestDeleteFromList failed @ ipsMgr.DeleteSet since %s still exist in kernel", listName)
	}

	// Delete set and validate set is not exist.
	if err := ipsMgr.DeleteSet(setName); err != nil {
		t.Errorf("TestDeleteSet failed @ ipsMgr.DeleteSet")
	}

	entry = &ipsEntry{
		operationFlag: util.IPsetCheckListFlag,
		set:           util.GetHashedName(setName),
	}

	if _, err := ipsMgr.Run(entry); err == nil {
		t.Errorf("TestDeleteFromList failed @ ipsMgr.DeleteSet since %s still exist in kernel", setName)
	}

	testutils.VerifyCallsMatch(t, calls, fexec, fcmd)
}

func TestCreateSet(t *testing.T) {
	metrics.NumIPSetEntries.Set(0)
	ipsMgr := NewIpsetManager(exec.New())
	if err := ipsMgr.Save(util.IpsetTestConfigFile); err != nil {
		t.Errorf("TestCreateSet failed @ ipsMgr.Save")
	}

	defer func() {
		if err := ipsMgr.Restore(util.IpsetTestConfigFile); err != nil {
			t.Errorf("TestCreateSet failed @ ipsMgr.Restore")
		}
	}()

	gaugeVal, err1 := promutil.GetValue(metrics.NumIPSets)
	countVal, err2 := promutil.GetCountValue(metrics.AddIPSetExecTime)

	testSet1Name := "test-set"
	if err := ipsMgr.CreateSet(testSet1Name, []string{util.IpsetNetHashFlag}); err != nil {
		t.Errorf("TestCreateSet failed @ ipsMgr.CreateSet")
	}

	testSet2Name := "test-set-with-maxelem"
	spec := append([]string{util.IpsetNetHashFlag, util.IpsetMaxelemName, util.IpsetMaxelemNum})
	if err := ipsMgr.CreateSet(testSet2Name, spec); err != nil {
		t.Errorf("TestCreateSet failed @ ipsMgr.CreateSet when set maxelem")
	}

	testSet3Name := "test-set-with-port"
	spec = append([]string{util.IpsetIPPortHashFlag})
	if err := ipsMgr.CreateSet(testSet3Name, spec); err != nil {
		t.Errorf("TestCreateSet failed @ ipsMgr.CreateSet when creating port set")
	}
	if err := ipsMgr.AddToSet(testSet3Name, fmt.Sprintf("%s,%s%d", "1.1.1.1", "tcp", 8080), util.IpsetIPPortHashFlag, "0"); err != nil {
		t.Errorf("AddToSet failed @ ipsMgr.CreateSet when set port")
	}

	newGaugeVal, err3 := promutil.GetValue(metrics.NumIPSets)
	newCountVal, err4 := promutil.GetCountValue(metrics.AddIPSetExecTime)
	testSet1Count, err5 := promutil.GetVecValue(metrics.IPSetInventory, metrics.GetIPSetInventoryLabels(testSet1Name))
	testSet2Count, err6 := promutil.GetVecValue(metrics.IPSetInventory, metrics.GetIPSetInventoryLabels(testSet2Name))
	testSet3Count, err7 := promutil.GetVecValue(metrics.IPSetInventory, metrics.GetIPSetInventoryLabels(testSet3Name))
	entryCount, err8 := promutil.GetValue(metrics.NumIPSetEntries)
	promutil.NotifyIfErrors(t, err1, err2, err3, err4, err5, err6, err7, err8)
	if newGaugeVal != gaugeVal+3 {
		t.Errorf("Change in ipset number didn't register in Prometheus")
	}
	if newCountVal != countVal+3 {
		t.Errorf("Execution time didn't register in Prometheus")
	}
	if testSet1Count != 0 || testSet2Count != 0 || testSet3Count != 1 || entryCount != 1 {
		t.Errorf("Prometheus IPSet count has incorrect number of entries")
	}
}

func TestDeleteSet(t *testing.T) {
	metrics.NumIPSetEntries.Set(0)
	ipsMgr := NewIpsetManager(exec.New())
	if err := ipsMgr.Save(util.IpsetTestConfigFile); err != nil {
		t.Errorf("TestDeleteSet failed @ ipsMgr.Save")
	}

	defer func() {
		if err := ipsMgr.Restore(util.IpsetTestConfigFile); err != nil {
			t.Errorf("TestDeleteSet failed @ ipsMgr.Restore")
		}
	}()

	testSetName := "test-delete-set"
	if err := ipsMgr.CreateSet(testSetName, append([]string{util.IpsetNetHashFlag})); err != nil {
		t.Errorf("TestDeleteSet failed @ ipsMgr.CreateSet")
	}

	gaugeVal, err1 := promutil.GetValue(metrics.NumIPSets)

	if err := ipsMgr.DeleteSet(testSetName); err != nil {
		t.Errorf("TestDeleteSet failed @ ipsMgr.DeleteSet")
	}

	newGaugeVal, err2 := promutil.GetValue(metrics.NumIPSets)
	testSetCount, err3 := promutil.GetVecValue(metrics.IPSetInventory, metrics.GetIPSetInventoryLabels(testSetName))
	entryCount, err4 := promutil.GetValue(metrics.NumIPSetEntries)
	promutil.NotifyIfErrors(t, err1, err2, err3, err4)
	if newGaugeVal != gaugeVal-1 {
		t.Errorf("Change in ipset number didn't register in prometheus")
	}
	if testSetCount != 0 || entryCount != 0 {
		t.Errorf("Prometheus IPSet count has incorrect number of entries")
	}
}

func TestAddToSet(t *testing.T) {
	metrics.NumIPSetEntries.Set(0)
	ipsMgr := NewIpsetManager(exec.New())
	if err := ipsMgr.Save(util.IpsetTestConfigFile); err != nil {
		t.Fatalf("TestAddToSet failed @ ipsMgr.Save")
	}

	defer func() {
		if err := ipsMgr.Restore(util.IpsetTestConfigFile); err != nil {
			t.Fatalf("TestAddToSet failed @ ipsMgr.Restore")
		}
	}()

	testSetName := "test-set"
	if err := ipsMgr.AddToSet(testSetName, "1.2.3.4", util.IpsetNetHashFlag, ""); err != nil {
		t.Fatalf("TestAddToSet failed @ ipsMgr.AddToSet")
	}

	if err := ipsMgr.AddToSet(testSetName, "1.2.3.4/nomatch", util.IpsetNetHashFlag, ""); err != nil {
		t.Fatalf("TestAddToSet with nomatch failed @ ipsMgr.AddToSet %v", err)
	}

	if err := ipsMgr.AddToSet(testSetName, fmt.Sprintf("%s,%s:%d", "1.1.1.1", "tcp", 8080), util.IpsetIPPortHashFlag, "0"); err != nil {
		t.Errorf("AddToSet failed @ ipsMgr.AddToSet when set port: %v", err)
	}

	if err := ipsMgr.AddToSet(testSetName, fmt.Sprintf("%s,:", "1.1.1.1"), util.IpsetIPPortHashFlag, "0"); err != nil {
		t.Errorf("AddToSet failed @ ipsMgr.AddToSet when set port is empty: %v", err)
	}

	if err := ipsMgr.AddToSet(testSetName, fmt.Sprintf("%s,%s:%d", "", "tcp", 8080), util.IpsetIPPortHashFlag, "0"); err == nil {
		t.Errorf("AddToSet failed @ ipsMgr.AddToSet when port is specified but ip is empty: %v", err)
	}

	if err := ipsMgr.AddToSet(testSetName, fmt.Sprintf("%s", "1.1.1.1"), util.IpsetIPPortHashFlag, "0"); err != nil {
		t.Errorf("AddToSet failed @ ipsMgr.AddToSet when only ip is specified: %v", err)
	}

	if err := ipsMgr.AddToSet(testSetName, fmt.Sprintf(""), util.IpsetIPPortHashFlag, "0"); err == nil {
		t.Errorf("AddToSet failed @ ipsMgr.AddToSet when no ip is specified: %v", err)
	}

	testSetCount, err1 := promutil.GetVecValue(metrics.IPSetInventory, metrics.GetIPSetInventoryLabels(testSetName))
	entryCount, err2 := promutil.GetValue(metrics.NumIPSetEntries)
	promutil.NotifyIfErrors(t, err1, err2)
	if testSetCount != 5 || entryCount != 5 {
		t.Fatalf("Prometheus IPSet count has incorrect number of entries, testSetCount %d, entryCount %d", testSetCount, entryCount)
	}
}

func TestAddToSetWithCachePodInfo(t *testing.T) {
	ipsMgr := NewIpsetManager(exec.New())
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
	cachedPodUid := ipsMgr.SetMap[setname].elements[ip]
	if cachedPodUid != pod1 {
		t.Errorf("setname: %s, hashedname: %s is added with wrong podUid: %s, expected: %s", setname, util.GetHashedName(setname), cachedPodUid, pod1)
	}

	// now add pod2 with the same ip. This is possible if DeletePod1 is handled after AddPod2 event callback.
	var pod2 = "pod2"
	if err := ipsMgr.AddToSet(setname, ip, util.IpsetNetHashFlag, pod2); err != nil {
		t.Errorf("TestAddToSetWithCachePodInfo with pod2 failed @ ipsMgr.AddToSet")
	}

	cachedPodUid = ipsMgr.SetMap[setname].elements[ip]
	if cachedPodUid != pod2 {
		t.Errorf("setname: %s, hashedname: %s is added with wrong podUid: %s, expected: %s", setname, util.GetHashedName(setname), cachedPodUid, pod2)
	}

	// Delete from set, it will delete the set if this is the last member
	ipsMgr.DeleteFromSet(setname, ip, pod2)
}

func TestDeleteFromSet(t *testing.T) {
	metrics.NumIPSetEntries.Set(0)
	ipsMgr := NewIpsetManager(exec.New())
	if err := ipsMgr.Save(util.IpsetTestConfigFile); err != nil {
		t.Errorf("TestDeleteFromSet failed @ ipsMgr.Save")
	}

	defer func() {
		if err := ipsMgr.Restore(util.IpsetTestConfigFile); err != nil {
			t.Errorf("TestDeleteFromSet failed @ ipsMgr.Restore")
		}
	}()

	testSetName := "test-delete-from-set"
	if err := ipsMgr.AddToSet(testSetName, "1.2.3.4", util.IpsetNetHashFlag, ""); err != nil {
		t.Errorf("TestDeleteFromSet failed @ ipsMgr.AddToSet")
	}

	if len(ipsMgr.SetMap[testSetName].elements) != 1 {
		t.Errorf("TestDeleteFromSet failed @ ipsMgr.AddToSet")
	}

	if err := ipsMgr.DeleteFromSet(testSetName, "1.2.3.4", ""); err != nil {
		t.Errorf("TestDeleteFromSet failed @ ipsMgr.DeleteFromSet")
	}

	// After deleting the only entry, "1.2.3.4" from "test-set", "test-set" ipset won't exist
	if _, exists := ipsMgr.SetMap[testSetName]; exists {
		t.Errorf("TestDeleteFromSet failed @ ipsMgr.DeleteFromSet")
	}

	testSetCount, err1 := promutil.GetVecValue(metrics.IPSetInventory, metrics.GetIPSetInventoryLabels(testSetName))
	entryCount, err2 := promutil.GetValue(metrics.NumIPSetEntries)
	promutil.NotifyIfErrors(t, err1, err2)
	if testSetCount != 0 || entryCount != 0 {
		t.Errorf("Prometheus IPSet count has incorrect number of entries %v", entryCount)
	}
}

func TestDeleteFromSetWithPodCache(t *testing.T) {
	ipsMgr := NewIpsetManager(exec.New())
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
		t.Errorf("TestDeleteFromSetWithPodCache failed for pod1 @ ipsMgr.AddToSet with err %+v", err)
	}

	if len(ipsMgr.SetMap[setname].elements) != 1 {
		t.Errorf("TestDeleteFromSetWithPodCache failed @ ipsMgr.AddToSet")
	}

	if err := ipsMgr.DeleteFromSet(setname, ip, pod1); err != nil {
		t.Errorf("TestDeleteFromSetWithPodCache for pod1 failed @ ipsMgr.DeleteFromSet with err %+v", err)
	}

	// now add the set again and then replace it with pod2
	var pod2 = "pod2"
	if err := ipsMgr.AddToSet(setname, ip, util.IpsetNetHashFlag, pod1); err != nil {
		t.Errorf("TestDeleteFromSetWithPodCache failed for pod1 @ ipsMgr.AddToSet with err %+v", err)
	}

	// Add Pod2 with same ip (This could happen if AddPod2 is served before DeletePod1)
	if err := ipsMgr.AddToSet(setname, ip, util.IpsetNetHashFlag, pod2); err != nil {
		t.Errorf("TestDeleteFromSetWithPodCache failed for pod2 @ ipsMgr.AddToSet with err %+v", err)
	}

	// Process DeletePod1
	if err := ipsMgr.DeleteFromSet(setname, ip, pod1); err != nil {
		t.Errorf("TestDeleteFromSetWithPodCache for pod1 failed @ ipsMgr.DeleteFromSet with err %+v", err)
	}

	// note the set will stil exist with pod ip
	cachedPodUid := ipsMgr.SetMap[setname].elements[ip]
	if cachedPodUid != pod2 {
		t.Errorf("setname: %s, hashedname: %s is added with wrong podUid: %s, expected: %s", setname, util.GetHashedName(setname), cachedPodUid, pod2)
	}

	// Now cleanup and delete pod2
	if err := ipsMgr.DeleteFromSet(setname, ip, pod2); err != nil {
		t.Errorf("TestDeleteFromSetWithPodCache for pod2 failed @ ipsMgr.DeleteFromSet with err %+v", err)
	}

	if _, exists := ipsMgr.SetMap[setname]; exists {
		t.Errorf("TestDeleteFromSetWithPodCache failed @ ipsMgr.DeleteFromSet")
	}
}

func TestClean(t *testing.T) {
	ipsMgr := NewIpsetManager(exec.New())
	if err := ipsMgr.Save(util.IpsetTestConfigFile); err != nil {
		t.Errorf("TestClean failed @ ipsMgr.Save")
	}

	defer func() {
		if err := ipsMgr.Restore(util.IpsetTestConfigFile); err != nil {
			t.Errorf("TestClean failed @ ipsMgr.Restore")
		}
	}()

	if err := ipsMgr.CreateSet("test-set", append([]string{util.IpsetNetHashFlag})); err != nil {
		t.Errorf("TestClean failed @ ipsMgr.CreateSet with err %+v", err)
	}

	if err := ipsMgr.Clean(); err != nil {
		t.Errorf("TestClean failed @ ipsMgr.Clean")
	}
}

func TestDestroy(t *testing.T) {
	ipsMgr := NewIpsetManager(exec.New())
	if err := ipsMgr.Save(util.IpsetTestConfigFile); err != nil {
		t.Errorf("TestDestroy failed @ ipsMgr.Save")
	}

	defer func() {
		if err := ipsMgr.Restore(util.IpsetTestConfigFile); err != nil {
			t.Errorf("TestDestroy failed @ ipsMgr.Restore")
		}
	}()

	setName := "test-destroy"
	testIP := "1.2.3.4"
	if err := ipsMgr.AddToSet(setName, testIP, util.IpsetNetHashFlag, ""); err != nil {
		t.Errorf("TestDestroy failed @ ipsMgr.AddToSet with err %+v", err)
	}

	// Call Destroy and validate. Destroy can only work when no ipset is referenced from iptables.
	if err := ipsMgr.Destroy(); err == nil {
		// Validate ipset is not exist when destroy can happen.
		entry := &ipsEntry{
			operationFlag: util.IPsetCheckListFlag,
			set:           util.GetHashedName(setName),
		}

		if _, err := ipsMgr.Run(entry); err == nil {
			t.Errorf("TestDestroy failed @ ipsMgr.Destroy since %s still exist in kernel with err %+v", setName, err)
		}
	} else {
		// Validate ipset entries are gone from flush command when destroy can not happen.
		entry := &ipsEntry{
			operationFlag: util.IpsetTestFlag,
			set:           util.GetHashedName(setName),
			spec:          append([]string{testIP}),
		}

		if _, err := ipsMgr.Run(entry); err == nil {
			t.Errorf("TestDestroy failed @ ipsMgr.Destroy since %s still exist in ipset with err %+v", testIP, err)
		}
	}
}

func TestRun(t *testing.T) {
	ipsMgr := NewIpsetManager(exec.New())
	if err := ipsMgr.Save(util.IpsetTestConfigFile); err != nil {
		t.Errorf("TestRun failed @ ipsMgr.Save with err %v", err)
	}

	defer func() {
		if err := ipsMgr.Restore(util.IpsetTestConfigFile); err != nil {
			t.Errorf("TestRun failed @ ipsMgr.Restore with err %v", err)
		}
	}()

	entry := &ipsEntry{
		operationFlag: util.IpsetCreationFlag,
		set:           "test-set",
		spec:          append([]string{util.IpsetNetHashFlag}),
	}
	if _, err := ipsMgr.Run(entry); err != nil {
		t.Errorf("TestRun failed @ ipsMgr.Run with err %+v", err)
	}
}

func TestRunError(t *testing.T) {
	setname := "test-set"

	var calls = []testutils.TestCmd{
		{Cmd: []string{"ipset", "-N", "-exist", util.GetHashedName(setname), "nethash"}, Stdout: "test failure", ExitCode: 2},
	}

	fexec, fcmd := testutils.GetFakeExecWithScripts(calls)

	ipsMgr := NewIpsetManager(fexec)
	entry := &ipsEntry{
		operationFlag: util.IpsetCreationFlag,
		set:           util.GetHashedName(setname),
		spec:          append([]string{util.IpsetNetHashFlag}),
	}
	if _, err := ipsMgr.Run(entry); err != nil {
		require.Error(t, err)
	}

	testutils.VerifyCallsMatch(t, calls, fexec, fcmd)
}

func TestDestroyNpmIpsets(t *testing.T) {
	ipsMgr := NewIpsetManager(exec.New())

	err := ipsMgr.CreateSet("azure-npm-123456", []string{"nethash"})
	if err != nil {
		t.Errorf("TestDestroyNpmIpsets failed @ ipsMgr.CreateSet")
		t.Errorf(err.Error())
	}

	err = ipsMgr.CreateSet("azure-npm-56543", []string{"nethash"})
	if err != nil {
		t.Errorf("TestDestroyNpmIpsets failed @ ipsMgr.CreateSet")
		t.Errorf(err.Error())
	}

	err = ipsMgr.DestroyNpmIpsets()
	if err != nil {
		t.Errorf("TestDestroyNpmIpsets failed @ ipsMgr.DestroyNpmIpsets")
		t.Errorf(err.Error())
	}
}

// Enable these tests once the the changes for ipsm are enabled
/*
const letters = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ"

func GetIPSetName() string {
	b := make([]byte, 8)

	for i := range b {
		b[i] = letters[rand.Intn(len(letters))]
	}

	return "npm-test-" + string(b)
}

// "Set cannot be destroyed: it is in use by a kernel component"
func TestSetCannotBeDestroyed(t *testing.T) {
	ipsMgr := NewIpsetManager(exec.New())
	if err := ipsMgr.Save(util.IpsetTestConfigFile); err != nil {
		t.Errorf("TestAddToList failed @ ipsMgr.Save")
	}

	defer func() {
		if err := ipsMgr.Restore(util.IpsetTestConfigFile); err != nil {
			t.Errorf("TestAddToList failed @ ipsMgr.Restore")
		}
	}()

	testset1 := GetIPSetName()
	testlist1 := GetIPSetName()

	if err := ipsMgr.CreateSet(testset1, append([]string{util.IpsetNetHashFlag})); err != nil {
		t.Errorf("Failed to create set with err %v", err)
	}

	if err := ipsMgr.AddToSet(testset1, fmt.Sprintf("%s", "1.1.1.1"), util.IpsetIPPortHashFlag, "0"); err != nil {
		t.Errorf("Failed to add to set with err %v", err)
	}

	if err := ipsMgr.AddToList(testlist1, testset1); err != nil {
		t.Errorf("Failed to add to list with err %v", err)
	}

	// Delete set and validate set is not exist.
	if err := ipsMgr.DeleteSet(testset1); err != nil {
		if err.ErrID != npmerr.SetCannotBeDestroyedInUseByKernelComponent {
			t.Errorf("Expected to error with ipset in use by kernel component")
		}
	}
}

func TestElemSeparatorSupportsNone(t *testing.T) {
	ipsMgr := NewIpsetManager(exec.New())
	if err := ipsMgr.Save(util.IpsetTestConfigFile); err != nil {
		t.Errorf("TestAddToList failed @ ipsMgr.Save")
	}

	defer func() {
		if err := ipsMgr.Restore(util.IpsetTestConfigFile); err != nil {
			t.Errorf("TestAddToList failed @ ipsMgr.Restore")
		}
	}()

	testset1 := GetIPSetName()

	if err := ipsMgr.CreateSet(testset1, append([]string{util.IpsetNetHashFlag})); err != nil {
		t.Errorf("TestAddToList failed @ ipsMgr.CreateSet")
	}

	entry := &ipsEntry{
		operationFlag: util.IpsetTestFlag,
		set:           util.GetHashedName(testset1),
		spec:          append([]string{fmt.Sprintf("10.104.7.252,3000")}),
	}

	if _, err := ipsMgr.Run(entry); err == nil || err.ErrID != ElemSeperatorNotSupported {
		t.Errorf("Expected elem seperator error: %+v", err)
	}
}

func TestIPSetWithGivenNameDoesNotExist(t *testing.T) {
	ipsMgr := NewIpsetManager(exec.New())
	if err := ipsMgr.Save(util.IpsetTestConfigFile); err != nil {
		t.Errorf("TestAddToList failed @ ipsMgr.Save with err %+v", err)
	}

	defer func() {
		if err := ipsMgr.Restore(util.IpsetTestConfigFile); err != nil {
			t.Errorf("TestAddToList failed @ ipsMgr.Restore with err %+v", err)
		}
	}()

	testset1 := GetIPSetName()
	testset2 := GetIPSetName()

	entry := &ipsEntry{
		operationFlag: util.IpsetAppendFlag,
		set:           util.GetHashedName(testset1),
		spec:          append([]string{util.GetHashedName(testset2)}),
	}

	var err *NPMError
	if _, err = ipsMgr.Run(entry); err == nil || err.ErrID != SetWithGivenNameDoesNotExist {
		t.Errorf("Expected set to not exist when adding to nonexistent set %+v", err)
	}
}

func TestIPSetWithGivenNameAlreadyExists(t *testing.T) {
	ipsMgr := NewIpsetManager(exec.New())
	if err := ipsMgr.Save(util.IpsetTestConfigFile); err != nil {
		t.Errorf("TestAddToList failed @ ipsMgr.Save with err %+v", err)
	}

	defer func() {
		if err := ipsMgr.Restore(util.IpsetTestConfigFile); err != nil {
			t.Errorf("TestAddToList failed @ ipsMgr.Restore with err %+v", err)
		}
	}()

	testset1 := GetIPSetName()

	entry := &ipsEntry{
		name:          testset1,
		operationFlag: util.IpsetCreationFlag,
		// Use hashed string for set name to avoid string length limit of ipset.
		set:  util.GetHashedName(testset1),
		spec: append([]string{util.IpsetNetHashFlag}),
	}

	if errCode, err := ipsMgr.Run(entry); err != nil && errCode != 1 {
		t.Errorf("Expected err")
	}

	entry = &ipsEntry{
		name:          testset1,
		operationFlag: util.IpsetCreationFlag,
		// Use hashed string for set name to avoid string length limit of ipset.
		set:  util.GetHashedName(testset1),
		spec: append([]string{util.IpsetSetListFlag}),
	}

	if _, err := ipsMgr.Run(entry); err == nil || err.ErrID != IPSetWithGivenNameAlreadyExists {
		t.Errorf("Expected error code to match when set does not exist: %+v", err)
	}
}

func TestIPSetSecondElementIsMissingWhenAddingIpWithNoPort(t *testing.T) {
	ipsMgr := NewIpsetManager(exec.New())
	if err := ipsMgr.Save(util.IpsetTestConfigFile); err != nil {
		t.Errorf("TestAddToList failed @ ipsMgr.Save with err: %+v", err)
	}

	defer func() {
		if err := ipsMgr.Restore(util.IpsetTestConfigFile); err != nil {
			t.Errorf("TestAddToList failed @ ipsMgr.Restore")
		}
	}()

	testset1 := GetIPSetName()

	spec := append([]string{util.IpsetIPPortHashFlag})
	if err := ipsMgr.CreateSet(testset1, spec); err != nil {
		t.Errorf("TestCreateSet failed @ ipsMgr.CreateSet when creating port set")
	}

	entry := &ipsEntry{
		operationFlag: util.IpsetAppendFlag,
		set:           util.GetHashedName(testset1),
		spec:          append([]string{fmt.Sprintf("%s", "1.1.1.1")}),
	}

	if _, err := ipsMgr.Run(entry); err == nil || err.ErrID != SecondElementIsMissing {
		t.Errorf("Expected to fail when adding ip with no port to set that requires port: %+v", err)
	}
}

func TestIPSetMissingSecondMandatoryArgument(t *testing.T) {
	ipsMgr := NewIpsetManager(exec.New())
	if err := ipsMgr.Save(util.IpsetTestConfigFile); err != nil {
		t.Errorf("TestAddToList failed @ ipsMgr.Save")
	}

	defer func() {
		if err := ipsMgr.Restore(util.IpsetTestConfigFile); err != nil {
			t.Errorf("TestAddToList failed @ ipsMgr.Restore")
		}
	}()

	testset1 := GetIPSetName()

	spec := append([]string{util.IpsetIPPortHashFlag})
	if err := ipsMgr.CreateSet(testset1, spec); err != nil {
		t.Errorf("TestCreateSet failed @ ipsMgr.CreateSet when creating port set")
	}

	entry := &ipsEntry{
		operationFlag: util.IpsetAppendFlag,
		set:           util.GetHashedName(testset1),
		spec:          append([]string{}),
	}

	if _, err := ipsMgr.Run(entry); err == nil || err.ErrID != MissingSecondMandatoryArgument {
		t.Errorf("Expected to fail when running ipset command with no second argument: %+v", err)
	}
}

func TestIPSetCannotBeAddedAsElementDoesNotExist(t *testing.T) {
	ipsMgr := NewIpsetManager(exec.New())
	if err := ipsMgr.Save(util.IpsetTestConfigFile); err != nil {
		t.Errorf("TestAddToList failed @ ipsMgr.Save")
	}

	defer func() {
		if err := ipsMgr.Restore(util.IpsetTestConfigFile); err != nil {
			t.Errorf("TestAddToList failed @ ipsMgr.Restore")
		}
	}()

	testset1 := GetIPSetName()
	testset2 := GetIPSetName()

	spec := append([]string{util.IpsetSetListFlag})
	entry := &ipsEntry{
		operationFlag: util.IpsetCreationFlag,
		set:           util.GetHashedName(testset1),
		spec:          spec,
	}

	if _, err := ipsMgr.Run(entry); err != nil {
		t.Errorf("Expected to not fail when creating ipset: %+v", err)
	}

	entry = &ipsEntry{
		operationFlag: util.IpsetAppendFlag,
		set:           util.GetHashedName(testset1),
		spec:          append([]string{util.GetHashedName(testset2)}),
	}

	if _, err := ipsMgr.Run(entry); err == nil || err.ErrID != SetToBeAddedDeletedTestedDoesNotExist {
		t.Errorf("Expected to fail when adding set to list and the set doesn't exist: %+v", err)
	}
}

*/
func TestMain(m *testing.M) {
	metrics.InitializeAll()

	log.Printf("Uniniting iptables")
	iptm := iptm.NewIptablesManager()
	iptm.UninitNpmChains()
	log.Printf("Uniniting ipsets")
	ipsMgr := NewIpsetManager(exec.New())
	ipsMgr.Destroy()

	exitCode := m.Run()

	os.Exit(exitCode)
}
