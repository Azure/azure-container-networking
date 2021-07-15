// Copyright 2018 Microsoft. All rights reserved.
// MIT License
package ipsm

import (
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"testing"

	"github.com/Azure/azure-container-networking/npm/metrics"
	"github.com/Azure/azure-container-networking/npm/metrics/promutil"
	"github.com/Azure/azure-container-networking/npm/util"
	testutils "github.com/Azure/azure-container-networking/test/utils"
	"github.com/stretchr/testify/require"
)

func TestSave(t *testing.T) {
	var calls = []testutils.TestCmd{
		{Cmd: []string{"ipset", "save", "-file", "ipset.conf"}},
	}

	fexec := testutils.GetFakeExecWithScripts(calls)
	ipsMgr := NewIpsetManager(fexec)
	defer testutils.VerifyCalls(t, fexec, calls)
	err := ipsMgr.Save("ipset.conf")
	require.NoError(t, err)
}

func TestRestore(t *testing.T) {
	// create temporary ipset config file to use
	tmpFile, err := ioutil.TempFile(os.TempDir(), filepath.Base(util.IpsetTestConfigFile))
	require.NoError(t, err)
	defer os.Remove(tmpFile.Name())

	var calls = []testutils.TestCmd{
		{Cmd: []string{"ipset", "-F", "-exist"}},
		{Cmd: []string{"ipset", "-X", "-exist"}},
		{Cmd: []string{"ipset", "restore", "-file", tmpFile.Name()}},
	}

	fexec := testutils.GetFakeExecWithScripts(calls)
	ipsMgr := NewIpsetManager(fexec)
	defer testutils.VerifyCalls(t, fexec, calls)

	err = ipsMgr.Restore(tmpFile.Name())
	require.NoError(t, err)
}

func TestCreateList(t *testing.T) {
	var calls = []testutils.TestCmd{
		{Cmd: []string{"ipset", "-N", "-exist", util.GetHashedName("test-list"), "setlist"}},
	}

	fexec := testutils.GetFakeExecWithScripts(calls)
	ipsMgr := NewIpsetManager(fexec)
	defer testutils.VerifyCalls(t, fexec, calls)

	err := ipsMgr.CreateList("test-list")
	require.NoError(t, err)
}

func TestDeleteList(t *testing.T) {
	var calls = []testutils.TestCmd{
		{Cmd: []string{"ipset", "-N", "-exist", util.GetHashedName("test-list"), "setlist"}},
		{Cmd: []string{"ipset", "-X", "-exist", util.GetHashedName("test-list")}},
	}

	fexec := testutils.GetFakeExecWithScripts(calls)
	ipsMgr := NewIpsetManager(fexec)
	defer testutils.VerifyCalls(t, fexec, calls)

	err := ipsMgr.CreateList("test-list")
	require.NoError(t, err)

	err = ipsMgr.DeleteList("test-list")
	require.NoError(t, err)
}

func TestAddToList(t *testing.T) {
	var calls = []testutils.TestCmd{
		{Cmd: []string{"ipset", "-N", "-exist", util.GetHashedName("test-set"), "nethash"}},
		{Cmd: []string{"ipset", "-N", "-exist", util.GetHashedName("test-list"), "setlist"}},
		{Cmd: []string{"ipset", "-A", "-exist", util.GetHashedName("test-list"), util.GetHashedName("test-set")}},
	}

	fexec := testutils.GetFakeExecWithScripts(calls)
	ipsMgr := NewIpsetManager(fexec)
	defer testutils.VerifyCalls(t, fexec, calls)

	err := ipsMgr.CreateSet("test-set", []string{util.IpsetNetHashFlag})
	require.NoError(t, err)

	err = ipsMgr.AddToList("test-list", "test-set")
	require.NoError(t, err)

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

	fexec := testutils.GetFakeExecWithScripts(calls)
	defer func() { require.Equal(t, fexec.CommandCalls, len(calls)) }()

	ipsMgr := NewIpsetManager(fexec)
	defer testutils.VerifyCalls(t, fexec, calls)

	// Create set and validate set is created.
	setName := "test-set"
	err := ipsMgr.CreateSet(setName, []string{util.IpsetNetHashFlag})
	require.NoError(t, err)

	entry := &ipsEntry{
		operationFlag: util.IPsetCheckListFlag,
		set:           util.GetHashedName(setName),
	}

	_, err = ipsMgr.Run(entry)
	require.NoError(t, err)

	// Create list, add set to list and validate set is in the list.
	listName := "test-list"
	err = ipsMgr.AddToList(listName, setName)
	require.NoError(t, err)

	entry = &ipsEntry{
		operationFlag: util.IpsetTestFlag,
		set:           util.GetHashedName(listName),
		spec:          []string{util.GetHashedName(setName)},
	}

	_, err = ipsMgr.Run(entry)
	require.NoError(t, err)

	// Delete set from list and validate set is not in list anymore.
	err = ipsMgr.DeleteFromList(listName, setName)
	require.NoError(t, err)

	// Delete set from list and validate set is not in list anymore.
	err = ipsMgr.DeleteFromList(listName, "nonexistentsetname")
	require.NoError(t, err)

	// Delete set from list, but list isn't of list type
	err = ipsMgr.DeleteFromList(setName, setName)
	require.NoError(t, err)

	entry = &ipsEntry{
		operationFlag: util.IpsetTestFlag,
		set:           util.GetHashedName(listName),
		spec:          []string{util.GetHashedName(setName)},
	}

	_, err = ipsMgr.Run(entry)
	require.Error(t, err)

	// Delete List and validate list is not exist.

	err = ipsMgr.DeleteSet(listName)
	require.NoError(t, err)

	entry = &ipsEntry{
		operationFlag: util.IPsetCheckListFlag,
		set:           util.GetHashedName(listName),
	}

	_, err = ipsMgr.Run(entry)
	require.Error(t, err)

	// Delete set and validate set is not exist.
	err = ipsMgr.DeleteSet(setName)
	require.NoError(t, err)

	entry = &ipsEntry{
		operationFlag: util.IPsetCheckListFlag,
		set:           util.GetHashedName(setName),
	}

	_, err = ipsMgr.Run(entry)
	require.Error(t, err)
}

func TestCreateSet(t *testing.T) {
	metrics.NumIPSetEntries.Set(0)

	var (
		testSet1Name = "test-set"
		testSet2Name = "test-set-with-maxelem"
		testSet3Name = "test-set-with-port"
	)

	var calls = []testutils.TestCmd{
		{Cmd: []string{"ipset", "-N", "-exist", util.GetHashedName(testSet1Name), "nethash"}},
		{Cmd: []string{"ipset", "-N", "-exist", util.GetHashedName(testSet2Name), "nethash", "maxelem", "4294967295"}},
		{Cmd: []string{"ipset", "-N", "-exist", util.GetHashedName(testSet3Name), "hash:ip,port"}},
		{Cmd: []string{"ipset", "-A", "-exist", util.GetHashedName(testSet3Name), "1.1.1.1,tcp8080"}, Stdout: "Bad formatting", ExitCode: 2},
		{Cmd: []string{"ipset", "-A", "-exist", util.GetHashedName(testSet3Name), "1.1.1.1,tcp,8080"}}, // todo: verify this is proper formatting
	}

	fexec := testutils.GetFakeExecWithScripts(calls)
	ipsMgr := NewIpsetManager(fexec)
	defer testutils.VerifyCalls(t, fexec, calls)

	gaugeVal, err1 := promutil.GetValue(metrics.NumIPSets)
	countVal, err2 := promutil.GetCountValue(metrics.AddIPSetExecTime)

	err := ipsMgr.CreateSet(testSet1Name, []string{util.IpsetNetHashFlag})
	require.NoError(t, err)

	spec := []string{util.IpsetNetHashFlag, util.IpsetMaxelemName, util.IpsetMaxelemNum}
	if err := ipsMgr.CreateSet(testSet2Name, spec); err != nil {
		t.Errorf("TestCreateSet failed @ ipsMgr.CreateSet when set maxelem")
	}

	spec = []string{util.IpsetIPPortHashFlag}
	if err := ipsMgr.CreateSet(testSet3Name, spec); err != nil {
		t.Errorf("TestCreateSet failed @ ipsMgr.CreateSet when creating port set")
	}
	
	err = ipsMgr.AddToSet(testSet3Name, fmt.Sprintf("%s,%s%d", "1.1.1.1", "tcp", 8080), util.IpsetIPPortHashFlag, "0")
	require.Error(t, err)

	if err := ipsMgr.AddToSet(testSet3Name, fmt.Sprintf("%s,%s,%d", "1.1.1.1", "tcp", 8080), util.IpsetIPPortHashFlag, "0"); err != nil {
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
		require.FailNowf(t, "", "Change in ipset number didn't register in Prometheus")
	}
	if newCountVal != countVal+3 {
		require.FailNowf(t, "", "Execution time didn't register in Prometheus")
	}
	if testSet1Count != 0 || testSet2Count != 0 || testSet3Count != 1 || entryCount != 1 {
		require.FailNowf(t, "", "Prometheus IPSet count has incorrect number of entries")
	}
}

func TestDeleteSet(t *testing.T) {
	metrics.NumIPSetEntries.Set(0)
	testSetName := "test-delete-set"
	var calls = []testutils.TestCmd{
		{Cmd: []string{"ipset", "-N", "-exist", util.GetHashedName(testSetName), "nethash"}},
		{Cmd: []string{"ipset", "-X", "-exist", util.GetHashedName(testSetName)}},
	}

	fexec := testutils.GetFakeExecWithScripts(calls)
	ipsMgr := NewIpsetManager(fexec)
	defer testutils.VerifyCalls(t, fexec, calls)

	err := ipsMgr.CreateSet(testSetName, []string{util.IpsetNetHashFlag})
	require.NoError(t, err)

	gaugeVal, err1 := promutil.GetValue(metrics.NumIPSets)

	err = ipsMgr.DeleteSet(testSetName)
	require.NoError(t, err)

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

	testSetName := "test-set"
	var calls = []testutils.TestCmd{
		{Cmd: []string{"ipset", "-N", "-exist", util.GetHashedName(testSetName), "nethash"}},
		{Cmd: []string{"ipset", "-A", "-exist", util.GetHashedName(testSetName), "1.2.3.4"}},
		{Cmd: []string{"ipset", "-A", "-exist", util.GetHashedName(testSetName), "1.2.3.4/", "nomatch"}},
		{Cmd: []string{"ipset", "-A", "-exist", util.GetHashedName(testSetName), "1.1.1.1,tcp:8080"}},
		{Cmd: []string{"ipset", "-A", "-exist", util.GetHashedName(testSetName), "1.1.1.1,:"}}, // todo: verify this is proper formatting
		{Cmd: []string{"ipset", "-A", "-exist", util.GetHashedName(testSetName), "1.1.1.1"}},
	}

	fexec := testutils.GetFakeExecWithScripts(calls)
	ipsMgr := NewIpsetManager(fexec)
	defer testutils.VerifyCalls(t, fexec, calls)

	err := ipsMgr.AddToSet(testSetName, "1.2.3.4", util.IpsetNetHashFlag, "")
	require.NoError(t, err)

	err = ipsMgr.AddToSet(testSetName, "1.2.3.4/nomatch", util.IpsetNetHashFlag, "")
	require.NoError(t, err)

	if err := ipsMgr.AddToSet(testSetName, fmt.Sprintf("%s,%s:%d", "1.1.1.1", "tcp", 8080), util.IpsetIPPortHashFlag, "0"); err != nil {
		t.Errorf("AddToSet failed @ ipsMgr.AddToSet when set port: %v", err)
	}

	err = ipsMgr.AddToSet(testSetName, fmt.Sprintf("%s,:", "1.1.1.1"), util.IpsetIPPortHashFlag, "0")
	require.NoError(t, err)

	err = ipsMgr.AddToSet(testSetName, fmt.Sprintf("%s,%s:%d", "", "tcp", 8080), util.IpsetIPPortHashFlag, "0")
	require.Errorf(t, err, "Expect failure when port is specified but ip is empty")

	err = ipsMgr.AddToSet(testSetName, "1.1.1.1", util.IpsetIPPortHashFlag, "0")
	require.NoError(t, err)

	err = ipsMgr.AddToSet(testSetName, "", util.IpsetIPPortHashFlag, "0")
	require.Error(t, err)

	testSetCount, err1 := promutil.GetVecValue(metrics.IPSetInventory, metrics.GetIPSetInventoryLabels(testSetName))
	entryCount, err2 := promutil.GetValue(metrics.NumIPSetEntries)
	promutil.NotifyIfErrors(t, err1, err2)
	if testSetCount != 5 || entryCount != 5 {
		t.Fatalf("Prometheus IPSet count has incorrect number of entries, testSetCount %d, entryCount %d", testSetCount, entryCount)
	}
}

func TestAddToSetWithCachePodInfo(t *testing.T) {
	var pod1 = "pod1"
	var setname = "test-podcache_new"
	var ip = "10.0.2.7"
	var pod2 = "pod2"

	var calls = []testutils.TestCmd{
		{Cmd: []string{"ipset", "-N", "-exist", util.GetHashedName(setname), "nethash"}},
		{Cmd: []string{"ipset", "-A", "-exist", util.GetHashedName(setname), ip}},
		{Cmd: []string{"ipset", "-D", "-exist", util.GetHashedName(setname), ip}},
		{Cmd: []string{"ipset", "-X", "-exist", util.GetHashedName(setname)}},
	}

	fexec := testutils.GetFakeExecWithScripts(calls)
	ipsMgr := NewIpsetManager(fexec)
	defer testutils.VerifyCalls(t, fexec, calls)

	err := ipsMgr.AddToSet(setname, ip, util.IpsetNetHashFlag, pod1)
	require.NoError(t, err)

	// validate if Pod1 exists
	cachedPodUid := ipsMgr.SetMap[setname].elements[ip]
	if cachedPodUid != pod1 {
		t.Errorf("setname: %s, hashedname: %s is added with wrong podUid: %s, expected: %s", setname, util.GetHashedName(setname), cachedPodUid, pod1)
	}

	// now add pod2 with the same ip. This is possible if DeletePod1 is handled after AddPod2 event callback.

	if err := ipsMgr.AddToSet(setname, ip, util.IpsetNetHashFlag, pod2); err != nil {
		t.Errorf("TestAddToSetWithCachePodInfo with pod2 failed @ ipsMgr.AddToSet")
	}

	cachedPodUid = ipsMgr.SetMap[setname].elements[ip]
	if cachedPodUid != pod2 {
		t.Errorf("setname: %s, hashedname: %s is added with wrong podUid: %s, expected: %s", setname, util.GetHashedName(setname), cachedPodUid, pod2)
	}

	// Delete from set, it will delete the set if this is the last member
	err = ipsMgr.DeleteFromSet(setname, ip, pod2)
	require.NoError(t, err)
}

func TestDeleteFromSet(t *testing.T) {
	metrics.NumIPSetEntries.Set(0)

	testSetName := "test-delete-from-set"
	var calls = []testutils.TestCmd{
		{Cmd: []string{"ipset", "-N", "-exist", util.GetHashedName(testSetName), "nethash"}},
		{Cmd: []string{"ipset", "-A", "-exist", util.GetHashedName(testSetName), "1.2.3.4"}},
		{Cmd: []string{"ipset", "-D", "-exist", util.GetHashedName(testSetName), "1.2.3.4"}},
		{Cmd: []string{"ipset", "-X", "-exist", util.GetHashedName(testSetName)}},
	}

	fexec := testutils.GetFakeExecWithScripts(calls)
	ipsMgr := NewIpsetManager(fexec)
	defer testutils.VerifyCalls(t, fexec, calls)

	err := ipsMgr.AddToSet(testSetName, "1.2.3.4", util.IpsetNetHashFlag, "")
	require.NoError(t, err)

	if len(ipsMgr.SetMap[testSetName].elements) != 1 {
		require.FailNow(t, "TestDeleteFromSet failed @ ipsMgr.AddToSet")
	}

	err = ipsMgr.DeleteFromSet(testSetName, "1.2.3.4", "")
	require.NoError(t, err)

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
	var setname = "test-deleteset-withcache"
	var ip = "10.0.2.8"
	var pod1 = "pod1"
	var pod2 = "pod2"

	var calls = []testutils.TestCmd{
		{Cmd: []string{"ipset", "-N", "-exist", util.GetHashedName(setname), "nethash"}},
		{Cmd: []string{"ipset", "-A", "-exist", util.GetHashedName(setname), ip}},
		{Cmd: []string{"ipset", "-D", "-exist", util.GetHashedName(setname), ip}},
		{Cmd: []string{"ipset", "-X", "-exist", util.GetHashedName(setname)}},
		{Cmd: []string{"ipset", "-N", "-exist", util.GetHashedName(setname), "nethash"}},
		{Cmd: []string{"ipset", "-A", "-exist", util.GetHashedName(setname), ip}},
		{Cmd: []string{"ipset", "-D", "-exist", util.GetHashedName(setname), ip}},
		{Cmd: []string{"ipset", "-X", "-exist", util.GetHashedName(setname)}},
	}

	fexec := testutils.GetFakeExecWithScripts(calls)
	ipsMgr := NewIpsetManager(fexec)
	defer testutils.VerifyCalls(t, fexec, calls)

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
	var calls = []testutils.TestCmd{
		{Cmd: []string{"ipset", "save", "-file", "/var/log/ipset-test.conf"}},
	}

	fexec := testutils.GetFakeExecWithScripts(calls)
	ipsMgr := NewIpsetManager(fexec)
	defer testutils.VerifyCalls(t, fexec, calls)

	if err := ipsMgr.Save(util.IpsetTestConfigFile); err != nil {
		t.Errorf("TestClean failed @ ipsMgr.Save")
	}

	if err := ipsMgr.Clean(); err != nil {
		t.Errorf("TestClean failed @ ipsMgr.Clean")
	}
}

func TestDestroy(t *testing.T) {
	setName := "test-destroy"
	testIP := "1.2.3.4"

	var calls = []testutils.TestCmd{
		{Cmd: []string{"ipset", "-N", "-exist", util.GetHashedName(setName), "nethash"}},
		{Cmd: []string{"ipset", "-A", "-exist", util.GetHashedName(setName), testIP}},
		{Cmd: []string{"ipset", "-F", "-exist"}},
		{Cmd: []string{"ipset", "-X", "-exist"}},
		{Cmd: []string{"ipset", "list", "-exist", util.GetHashedName(setName)}, ExitCode: 2},
	}

	fexec := testutils.GetFakeExecWithScripts(calls)
	ipsMgr := NewIpsetManager(fexec)
	defer testutils.VerifyCalls(t, fexec, calls)

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
			spec:          []string{testIP},
		}

		if _, err := ipsMgr.Run(entry); err == nil {
			t.Errorf("TestDestroy failed @ ipsMgr.Destroy since %s still exist in ipset with err %+v", testIP, err)
		}
	}
}

func TestRun(t *testing.T) {
	var calls = []testutils.TestCmd{
		{Cmd: []string{"ipset", "-N", "-exist", util.GetHashedName("test-set"), "nethash"}},
	}

	fexec := testutils.GetFakeExecWithScripts(calls)
	ipsMgr := NewIpsetManager(fexec)
	defer testutils.VerifyCalls(t, fexec, calls)

	entry := &ipsEntry{
		operationFlag: util.IpsetCreationFlag,
		set:           util.GetHashedName("test-set"),
		spec:          []string{util.IpsetNetHashFlag},
	}
	if _, err := ipsMgr.Run(entry); err != nil {
		t.Errorf("TestRun failed @ ipsMgr.Run with err %+v", err)
	}
}

func TestRunErrorWithNonZeroExitCode(t *testing.T) {
	var calls = []testutils.TestCmd{
		{Cmd: []string{"ipset", "-A", "-exist", util.GetHashedName("test-set"), "nethash"}, Stdout: "test failure", ExitCode: 2},
	}

	fexec := testutils.GetFakeExecWithScripts(calls)
	ipsMgr := NewIpsetManager(fexec)
	defer testutils.VerifyCalls(t, fexec, calls)

	entry := &ipsEntry{
		operationFlag: util.IpsetAppendFlag,
		set:           util.GetHashedName("test-set"),
		spec:          []string{util.IpsetNetHashFlag},
	}
	_, err := ipsMgr.Run(entry)
	require.Error(t, err)
}

func TestDestroyNpmIpsets(t *testing.T) {
	var calls = []testutils.TestCmd{
		{Cmd: []string{"ipset", "-N", "-exist", util.GetHashedName("azure-npm-123456"), "nethash"}},
		{Cmd: []string{"ipset", "-N", "-exist", util.GetHashedName("azure-npm-56543"), "nethash"}},
		{Cmd: []string{"ipset", "list"}},
	}

	fexec := testutils.GetFakeExecWithScripts(calls)
	ipsMgr := NewIpsetManager(fexec)
	defer testutils.VerifyCalls(t, fexec, calls)

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

	exitCode := m.Run()

	os.Exit(exitCode)
}
