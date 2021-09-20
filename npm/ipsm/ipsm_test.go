// Copyright 2018 Microsoft. All rights reserved.
// MIT License
package ipsm

import (
	"fmt"
	"os"
	"testing"

	"github.com/Azure/azure-container-networking/npm/metrics"
	"github.com/Azure/azure-container-networking/npm/metrics/promutil"
	"github.com/Azure/azure-container-networking/npm/util"
	testutils "github.com/Azure/azure-container-networking/test/utils"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"k8s.io/utils/exec"
)

const (
	testSetName  = "test-set"
	testListName = "test-list"
)

type expectedSetInfo struct {
	val  int
	name string
}

func TestCreateList(t *testing.T) {
	calls := []testutils.TestCmd{
		{Cmd: []string{"ipset", "-N", "-exist", util.GetHashedName(testListName), "setlist"}},
	}

	fexec := testutils.GetFakeExecWithScripts(calls)
	ipsMgr := NewIpsetManager(fexec)
	defer testutils.VerifyCalls(t, fexec, calls)

	execCount := resetPrometheusAndGetExecCount(t)
	defer testPrometheusMetrics(t, 1, execCount+1, 0, expectedSetInfo{0, testListName})

	err := ipsMgr.createList(testListName)
	require.NoError(t, err)
}

func resetPrometheusAndGetExecCount(t *testing.T) int {
	metrics.ResetNumIPSets()
	metrics.ResetIPSetEntries()
	execCount, err := metrics.GetIPSetExecCount()
	promutil.NotifyIfErrors(t, err)
	return execCount
}

func testPrometheusMetrics(t *testing.T, expectedNumSets, expectedExecCount, expectedNumEntries int, expectedSets ...expectedSetInfo) {
	numSets, err := metrics.GetNumIPSets()
	promutil.NotifyIfErrors(t, err)
	if numSets != expectedNumSets {
		require.FailNowf(t, "", "Number of ipsets didn't register correctly in Prometheus. Expected %d. Got %d.", expectedNumSets, numSets)
	}

	execCount, err := metrics.GetIPSetExecCount()
	promutil.NotifyIfErrors(t, err)
	if execCount != expectedExecCount {
		require.FailNowf(t, "", "Count for execution time didn't register correctly in Prometheus. Expected %d. Got %d.", expectedExecCount, execCount)
	}

	numEntries, err := metrics.GetNumIPSetEntries()
	promutil.NotifyIfErrors(t, err)
	if numEntries != expectedNumEntries {
		require.FailNowf(t, "", "Number of ipset entries didn't register correctly in Prometheus. Expected %d. Got %d.", expectedNumEntries, numEntries)
	}

	for _, set := range expectedSets {
		setCount, err := metrics.GetNumEntriesForIPSet(set.name)
		promutil.NotifyIfErrors(t, err)
		if setCount != set.val {
			require.FailNowf(t, "", "Incorrect number of entries in Prometheus for ipset %s. Expected %d. Got %d.", set.name, set.val, setCount)
		}
	}
}

func TestDeleteList(t *testing.T) {
	calls := []testutils.TestCmd{
		{Cmd: []string{"ipset", "-N", "-exist", util.GetHashedName(testListName), "setlist"}},
		{Cmd: []string{"ipset", "-X", "-exist", util.GetHashedName(testListName)}},
	}

	fexec := testutils.GetFakeExecWithScripts(calls)
	ipsMgr := NewIpsetManager(fexec)
	defer testutils.VerifyCalls(t, fexec, calls)

	execCount := resetPrometheusAndGetExecCount(t)
	defer testPrometheusMetrics(t, 0, execCount+1, 0, expectedSetInfo{0, testListName})

	err := ipsMgr.createList(testListName)
	require.NoError(t, err)

	err = ipsMgr.deleteList(testListName)
	require.NoError(t, err)
}

func TestAddToList(t *testing.T) {
	calls := []testutils.TestCmd{
		{Cmd: []string{"ipset", "-N", "-exist", util.GetHashedName(testSetName), "nethash"}},
		{Cmd: []string{"ipset", "-N", "-exist", util.GetHashedName(testListName), "setlist"}},
		{Cmd: []string{"ipset", "-A", "-exist", util.GetHashedName(testListName), util.GetHashedName(testSetName)}},
	}

	fexec := testutils.GetFakeExecWithScripts(calls)
	ipsMgr := NewIpsetManager(fexec)
	defer testutils.VerifyCalls(t, fexec, calls)

	execCount := resetPrometheusAndGetExecCount(t)
	defer testPrometheusMetrics(t, 2, execCount+2, 1, expectedSetInfo{1, testListName})

	err := ipsMgr.createSet(testSetName, []string{util.IpsetNetHashFlag})
	require.NoError(t, err)

	err = ipsMgr.AddToList(testListName, testSetName)
	require.NoError(t, err)
}

func TestDeleteFromList(t *testing.T) {
	calls := []testutils.TestCmd{
		{Cmd: []string{"ipset", "-N", "-exist", util.GetHashedName(testSetName), "nethash"}},
		{Cmd: []string{"ipset", "list", "-exist", util.GetHashedName(testSetName)}},
		{Cmd: []string{"ipset", "-N", "-exist", util.GetHashedName(testListName), "setlist"}},
		{Cmd: []string{"ipset", "-A", "-exist", util.GetHashedName(testListName), util.GetHashedName(testSetName)}},
		{Cmd: []string{"ipset", "test", "-exist", util.GetHashedName(testListName), util.GetHashedName(testSetName)}},
		{Cmd: []string{"ipset", "-D", "-exist", util.GetHashedName(testListName), util.GetHashedName(testSetName)}},
		{Cmd: []string{"ipset", "-X", "-exist", util.GetHashedName(testListName)}},
		{Cmd: []string{"ipset", "test", "-exist", util.GetHashedName(testListName), util.GetHashedName(testSetName)}, Stdout: "ipset still exists", ExitCode: 2},
		{Cmd: []string{"ipset", "list", "-exist", util.GetHashedName(testListName)}, Stdout: "ipset still exists", ExitCode: 2},
		{Cmd: []string{"ipset", "-X", "-exist", util.GetHashedName(testSetName)}},
		{Cmd: []string{"ipset", "list", "-exist", util.GetHashedName(testSetName)}, Stdout: "ipset still exists", ExitCode: 2},
	}

	fexec := testutils.GetFakeExecWithScripts(calls)
	defer func() { require.Equal(t, fexec.CommandCalls, len(calls)) }()

	ipsMgr := NewIpsetManager(fexec)
	defer testutils.VerifyCalls(t, fexec, calls)

	execCount := resetPrometheusAndGetExecCount(t)
	expectedSets := []expectedSetInfo{{0, testSetName}, {0, testListName}}
	defer testPrometheusMetrics(t, 0, execCount+2, 0, expectedSets...)

	// Create set and validate set is created.
	err := ipsMgr.createSet(testSetName, []string{util.IpsetNetHashFlag})
	require.NoError(t, err)

	entry := &ipsEntry{
		operationFlag: util.IPsetCheckListFlag,
		set:           util.GetHashedName(testSetName),
	}

	_, err = ipsMgr.run(entry)
	require.NoError(t, err)

	// Create list, add set to list and validate set is in the list.
	err = ipsMgr.AddToList(testListName, testSetName)
	require.NoError(t, err)

	entry = &ipsEntry{
		operationFlag: util.IpsetTestFlag,
		set:           util.GetHashedName(testListName),
		spec:          []string{util.GetHashedName(testSetName)},
	}

	_, err = ipsMgr.run(entry)
	require.NoError(t, err)

	// Delete set from list and validate set is not in list anymore.
	err = ipsMgr.DeleteFromList(testListName, testSetName)
	require.NoError(t, err)

	// Delete set from list and validate set is not in list anymore.
	err = ipsMgr.DeleteFromList(testListName, "nonexistentsetname")
	require.NoError(t, err)

	// Delete set from list, but list isn't of list type
	err = ipsMgr.DeleteFromList(testSetName, testSetName)
	require.NoError(t, err)

	entry = &ipsEntry{
		operationFlag: util.IpsetTestFlag,
		set:           util.GetHashedName(testListName),
		spec:          []string{util.GetHashedName(testSetName)},
	}

	_, err = ipsMgr.run(entry)
	require.Error(t, err)

	// Delete List and validate list is not exist.
	err = ipsMgr.deleteSet(testListName)
	require.NoError(t, err)

	entry = &ipsEntry{
		operationFlag: util.IPsetCheckListFlag,
		set:           util.GetHashedName(testListName),
	}

	_, err = ipsMgr.run(entry)
	require.Error(t, err)

	// Delete set and validate set is not exist.
	err = ipsMgr.deleteSet(testSetName)
	require.NoError(t, err)

	entry = &ipsEntry{
		operationFlag: util.IPsetCheckListFlag,
		set:           util.GetHashedName(testSetName),
	}

	_, err = ipsMgr.run(entry)
	require.Error(t, err)
}

func TestCreateSet(t *testing.T) {
	testSet1Name := "test-set"
	testSet2Name := "test-set-with-maxelem"
	testSet3Name := "test-set-with-port"

	calls := []testutils.TestCmd{
		{Cmd: []string{"ipset", "-N", "-exist", util.GetHashedName(testSet1Name), "nethash"}},
		{Cmd: []string{"ipset", "-N", "-exist", util.GetHashedName(testSet2Name), "nethash", "maxelem", "4294967295"}},
		{Cmd: []string{"ipset", "-N", "-exist", util.GetHashedName(testSet3Name), "hash:ip,port"}},
		{Cmd: []string{"ipset", "-A", "-exist", util.GetHashedName(testSet3Name), "1.1.1.1,tcp8080"}, Stdout: "Bad formatting", ExitCode: 2},
		{Cmd: []string{"ipset", "-A", "-exist", util.GetHashedName(testSet3Name), "1.1.1.1,tcp,8080"}}, // todo: verify this is proper formatting
	}

	fexec := testutils.GetFakeExecWithScripts(calls)
	ipsMgr := NewIpsetManager(fexec)
	defer testutils.VerifyCalls(t, fexec, calls)

	execCount := resetPrometheusAndGetExecCount(t)
	expectedSets := []expectedSetInfo{{0, testSet1Name}, {0, testSet2Name}, {1, testSet3Name}}
	defer testPrometheusMetrics(t, 3, execCount+3, 1, expectedSets...)

	err := ipsMgr.createSet(testSet1Name, []string{util.IpsetNetHashFlag})
	require.NoError(t, err)

	spec := []string{util.IpsetNetHashFlag, util.IpsetMaxelemName, util.IpsetMaxelemNum}
	if err := ipsMgr.createSet(testSet2Name, spec); err != nil {
		t.Errorf("TestCreateSet failed @ ipsMgr.CreateSet when set maxelem")
	}

	spec = []string{util.IpsetIPPortHashFlag}
	if err := ipsMgr.createSet(testSet3Name, spec); err != nil {
		t.Errorf("TestCreateSet failed @ ipsMgr.CreateSet when creating port set")
	}

	err = ipsMgr.AddToSet(testSet3Name, fmt.Sprintf("%s,%s%d", "1.1.1.1", "tcp", 8080), util.IpsetIPPortHashFlag, "0")
	require.Error(t, err)

	if err := ipsMgr.AddToSet(testSet3Name, fmt.Sprintf("%s,%s,%d", "1.1.1.1", "tcp", 8080), util.IpsetIPPortHashFlag, "0"); err != nil {
		t.Errorf("AddToSet failed @ ipsMgr.CreateSet when set port")
	}
}

func TestDeleteSet(t *testing.T) {
	testSetName := "test-delete-set"
	calls := []testutils.TestCmd{
		{Cmd: []string{"ipset", "-N", "-exist", util.GetHashedName(testSetName), "nethash"}},
		{Cmd: []string{"ipset", "-X", "-exist", util.GetHashedName(testSetName)}},
	}

	fexec := testutils.GetFakeExecWithScripts(calls)
	ipsMgr := NewIpsetManager(fexec)
	defer testutils.VerifyCalls(t, fexec, calls)

	execCount := resetPrometheusAndGetExecCount(t)
	defer testPrometheusMetrics(t, 0, execCount+1, 0, expectedSetInfo{0, testSetName})

	err := ipsMgr.createSet(testSetName, []string{util.IpsetNetHashFlag})
	require.NoError(t, err)

	err = ipsMgr.deleteSet(testSetName)
	require.NoError(t, err)
}

func TestAddToSet(t *testing.T) {
	calls := []testutils.TestCmd{
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

	execCount := resetPrometheusAndGetExecCount(t)
	defer testPrometheusMetrics(t, 1, execCount+1, 5, expectedSetInfo{5, testSetName})

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
}

func TestAddToSetWithCachePodInfo(t *testing.T) {
	pod1 := "pod1"
	setname := "test-podcache_new"
	ip := "10.0.2.7"
	pod2 := "pod2"

	calls := []testutils.TestCmd{
		{Cmd: []string{"ipset", "-N", "-exist", util.GetHashedName(setname), "nethash"}},
		{Cmd: []string{"ipset", "-A", "-exist", util.GetHashedName(setname), ip}},
		{Cmd: []string{"ipset", "-D", "-exist", util.GetHashedName(setname), ip}},
		{Cmd: []string{"ipset", "-X", "-exist", util.GetHashedName(setname)}},
	}

	fexec := testutils.GetFakeExecWithScripts(calls)
	ipsMgr := NewIpsetManager(fexec)
	defer testutils.VerifyCalls(t, fexec, calls)

	execCount := resetPrometheusAndGetExecCount(t)
	defer testPrometheusMetrics(t, 0, execCount+1, 0, expectedSetInfo{0, setname})

	err := ipsMgr.AddToSet(setname, ip, util.IpsetNetHashFlag, pod1)
	require.NoError(t, err)

	// validate if Pod1 exists
	cachedPodKey := ipsMgr.setMap[setname].elements[ip]
	if cachedPodKey != pod1 {
		t.Errorf("setname: %s, hashedname: %s is added with wrong cachedPodKey: %s, expected: %s",
			setname, util.GetHashedName(setname), cachedPodKey, pod1)
	}

	// now add pod2 with the same ip. This is possible if DeletePod1 is handled after AddPod2 event callback.

	if err := ipsMgr.AddToSet(setname, ip, util.IpsetNetHashFlag, pod2); err != nil {
		t.Errorf("TestAddToSetWithCachePodInfo with pod2 failed @ ipsMgr.AddToSet")
	}

	cachedPodKey = ipsMgr.setMap[setname].elements[ip]
	if cachedPodKey != pod2 {
		t.Errorf("setname: %s, hashedname: %s is added with wrong cachedPodKey: %s, expected: %s",
			setname, util.GetHashedName(setname), cachedPodKey, pod2)
	}

	// Delete from set, it will delete the set if this is the last member
	err = ipsMgr.DeleteFromSet(setname, ip, pod2)
	require.NoError(t, err)
}

func TestDeleteFromSet(t *testing.T) {
	testSetName := "test-delete-from-set"
	calls := []testutils.TestCmd{
		{Cmd: []string{"ipset", "-N", "-exist", util.GetHashedName(testSetName), "nethash"}},
		{Cmd: []string{"ipset", "-A", "-exist", util.GetHashedName(testSetName), "1.2.3.4"}},
		{Cmd: []string{"ipset", "-D", "-exist", util.GetHashedName(testSetName), "1.2.3.4"}},
		{Cmd: []string{"ipset", "-X", "-exist", util.GetHashedName(testSetName)}},
	}

	fexec := testutils.GetFakeExecWithScripts(calls)
	ipsMgr := NewIpsetManager(fexec)
	defer testutils.VerifyCalls(t, fexec, calls)

	execCount := resetPrometheusAndGetExecCount(t)
	defer testPrometheusMetrics(t, 0, execCount+1, 0, expectedSetInfo{0, testSetName}) // set is deleted when it has no members

	err := ipsMgr.AddToSet(testSetName, "1.2.3.4", util.IpsetNetHashFlag, "")
	require.NoError(t, err)

	if len(ipsMgr.setMap[testSetName].elements) != 1 {
		require.FailNow(t, "TestDeleteFromSet failed @ ipsMgr.AddToSet")
	}

	err = ipsMgr.DeleteFromSet(testSetName, "1.2.3.4", "")
	require.NoError(t, err)

	// After deleting the only entry, "1.2.3.4" from testSetName, testSetName ipset won't exist
	if _, exists := ipsMgr.setMap[testSetName]; exists {
		t.Errorf("TestDeleteFromSet failed @ ipsMgr.DeleteFromSet")
	}
}

func TestDeleteFromSetWithPodCache(t *testing.T) {
	setname := "test-deleteset-withcache"
	ip := "10.0.2.8"
	pod1 := "pod1"
	pod2 := "pod2"

	calls := []testutils.TestCmd{
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

	execCount := resetPrometheusAndGetExecCount(t)
	defer testPrometheusMetrics(t, 0, execCount+2, 0, expectedSetInfo{0, setname}) // set must be created again after deletion from having 0 members

	if err := ipsMgr.AddToSet(setname, ip, util.IpsetNetHashFlag, pod1); err != nil {
		t.Errorf("TestDeleteFromSetWithPodCache failed for pod1 @ ipsMgr.AddToSet with err %+v", err)
	}

	if len(ipsMgr.setMap[setname].elements) != 1 {
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
	cachedPodKey := ipsMgr.setMap[setname].elements[ip]
	if cachedPodKey != pod2 {
		t.Errorf("setname: %s, hashedname: %s is added with wrong cachedPodKey: %s, expected: %s",
			setname, util.GetHashedName(setname), cachedPodKey, pod2)
	}

	// Now cleanup and delete pod2
	if err := ipsMgr.DeleteFromSet(setname, ip, pod2); err != nil {
		t.Errorf("TestDeleteFromSetWithPodCache for pod2 failed @ ipsMgr.DeleteFromSet with err %+v", err)
	}

	if _, exists := ipsMgr.setMap[setname]; exists {
		t.Errorf("TestDeleteFromSetWithPodCache failed @ ipsMgr.DeleteFromSet")
	}
}

func TestRun(t *testing.T) {
	calls := []testutils.TestCmd{
		{Cmd: []string{"ipset", "-N", "-exist", util.GetHashedName(testSetName), "nethash"}},
	}

	fexec := testutils.GetFakeExecWithScripts(calls)
	ipsMgr := NewIpsetManager(fexec)
	defer testutils.VerifyCalls(t, fexec, calls)

	entry := &ipsEntry{
		operationFlag: util.IpsetCreationFlag,
		set:           util.GetHashedName(testSetName),
		spec:          []string{util.IpsetNetHashFlag},
	}
	if _, err := ipsMgr.run(entry); err != nil {
		t.Errorf("TestRun failed @ ipsMgr.Run with err %+v", err)
	}
}

func TestRunErrorWithNonZeroExitCode(t *testing.T) {
	calls := []testutils.TestCmd{
		{Cmd: []string{"ipset", "-A", "-exist", util.GetHashedName(testSetName), "nethash"}, Stdout: "test failure", ExitCode: 2},
	}

	fexec := testutils.GetFakeExecWithScripts(calls)
	ipsMgr := NewIpsetManager(fexec)
	defer testutils.VerifyCalls(t, fexec, calls)

	entry := &ipsEntry{
		operationFlag: util.IpsetAppendFlag,
		set:           util.GetHashedName(testSetName),
		spec:          []string{util.IpsetNetHashFlag},
	}
	_, err := ipsMgr.run(entry)
	require.Error(t, err)
}

func TestDestroyNpmIpsets(t *testing.T) {
	testSet1Name := util.AzureNpmPrefix + "123456"
	testSet2Name := util.AzureNpmPrefix + "56543"

	ipsMgr := NewIpsetManager(exec.New())

	execCount := resetPrometheusAndGetExecCount(t)
	expectedSets := []expectedSetInfo{{0, testSet1Name}, {0, testSet1Name}}
	defer testPrometheusMetrics(t, 0, execCount+2, 0, expectedSets...)

	err := ipsMgr.createSet(testSet1Name, []string{"nethash"})
	if err != nil {
		t.Errorf("TestDestroyNpmIpsets failed @ ipsMgr.createSet")
		t.Errorf(err.Error())
	}

	err = ipsMgr.createSet(testSet2Name, []string{"nethash"})
	if err != nil {
		t.Errorf("TestDestroyNpmIpsets failed @ ipsMgr.createSet")
		t.Errorf(err.Error())
	}

	err = ipsMgr.DestroyNpmIpsets()
	if err != nil {
		t.Errorf("TestDestroyNpmIpsets failed @ ipsMgr.DestroyNpmIpsets")
		t.Errorf(err.Error())
	}
}

func TestMarshalListMapJSON(t *testing.T) {
	testListSet := "test-list"
	calls := []testutils.TestCmd{
		{Cmd: []string{"ipset", "-N", "-exist", util.GetHashedName(testListSet), "setlist"}},
	}

	fexec := testutils.GetFakeExecWithScripts(calls)
	ipsMgr := NewIpsetManager(fexec)
	defer testutils.VerifyCalls(t, fexec, calls)

	err := ipsMgr.createList(testListSet)
	require.NoError(t, err)

	listMapRaw, err := ipsMgr.MarshalListMapJSON()
	require.NoError(t, err)
	fmt.Println(string(listMapRaw))

	expect := []byte(`{"test-list":{}}`)

	fmt.Printf("%v\n", ipsMgr.listMap)
	assert.ElementsMatch(t, expect, listMapRaw)
}

func TestMarshalSetMapJSON(t *testing.T) {
	testSet := "test-set"
	calls := []testutils.TestCmd{
		{Cmd: []string{"ipset", "-N", "-exist", util.GetHashedName(testSet), "nethash"}},
	}

	fexec := testutils.GetFakeExecWithScripts(calls)
	ipsMgr := NewIpsetManager(fexec)
	defer testutils.VerifyCalls(t, fexec, calls)

	err := ipsMgr.createSet(testSet, []string{util.IpsetNetHashFlag})
	require.NoError(t, err)

	setMapRaw, err := ipsMgr.MarshalSetMapJSON()
	require.NoError(t, err)
	fmt.Println(string(setMapRaw))

	expect := []byte(`{"test-set":{}}`)
	for key, val := range ipsMgr.setMap {
		fmt.Printf("key: %s value: %+v\n", key, val)
	}

	assert.ElementsMatch(t, expect, setMapRaw)
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

	if err := ipsMgr.createSet(testset1, append([]string{util.IpsetNetHashFlag})); err != nil {
		t.Errorf("Failed to create set with err %v", err)
	}

	if err := ipsMgr.AddToSet(testset1, fmt.Sprintf("%s", "1.1.1.1"), util.IpsetIPPortHashFlag, "0"); err != nil {
		t.Errorf("Failed to add to set with err %v", err)
	}

	if err := ipsMgr.AddToList(testlist1, testset1); err != nil {
		t.Errorf("Failed to add to list with err %v", err)
	}

	// Delete set and validate set is not exist.
	if err := ipsMgr.deleteSet(testset1); err != nil {
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

	if err := ipsMgr.createSet(testset1, append([]string{util.IpsetNetHashFlag})); err != nil {
		t.Errorf("TestAddToList failed @ ipsMgr.createSet")
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
	if err := ipsMgr.createSet(testset1, spec); err != nil {
		t.Errorf("TestcreateSet failed @ ipsMgr.createSet when creating port set")
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
	if err := ipsMgr.createSet(testset1, spec); err != nil {
		t.Errorf("TestcreateSet failed @ ipsMgr.createSet when creating port set")
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
