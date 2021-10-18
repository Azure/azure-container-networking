package ipsets

import (
	"fmt"
	"sort"
	"strings"
	"testing"

	"github.com/Azure/azure-container-networking/common"
	"github.com/Azure/azure-container-networking/npm/pkg/dataplane/ioutil"
	"github.com/Azure/azure-container-networking/npm/util"
	testutils "github.com/Azure/azure-container-networking/test/utils"
	"github.com/stretchr/testify/require"
)

type testSet struct {
	metadata   *IPSetMetadata
	hashedName string
}

func createTestSet(name string, setType SetType) *testSet {
	set := &testSet{
		metadata: &IPSetMetadata{name, setType},
	}
	set.hashedName = util.GetHashedName(set.metadata.GetPrefixName())
	return set
}

var (
	iMgrApplyAllCfg = &IPSetManagerCfg{
		IPSetMode:   ApplyAllIPSets,
		NetworkName: "",
	}

	ipsetRestoreStringSlice   = []string{util.Ipset, util.IpsetRestoreFlag}
	fakeRestoreSuccessCommand = testutils.TestCmd{
		Cmd:      ipsetRestoreStringSlice,
		Stdout:   "success",
		ExitCode: 0,
	}

	testNSSet           = createTestSet("test-ns-set", Namespace)
	testKeyPodSet       = createTestSet("test-keyPod-set", KeyLabelOfPod)
	testKVPodSet        = createTestSet("test-kvPod-set", KeyValueLabelOfPod)
	testNamedportSet    = createTestSet("test-namedport-set", NamedPorts)
	testCIDRSet         = createTestSet("test-cidr-set", CIDRBlocks)
	testKeyNSList       = createTestSet("test-keyNS-list", KeyLabelOfNamespace)
	testKVNSList        = createTestSet("test-kvNS-list", KeyValueLabelOfNamespace)
	testNestedLabelList = createTestSet("test-nestedlabel-list", NestedLabelOfPod)
)

func TestDestroyNPMIPSets(t *testing.T) {
	calls := []testutils.TestCmd{} // TODO
	iMgr := NewIPSetManager(iMgrApplyAllCfg, common.NewMockIOShim(calls))
	require.NoError(t, iMgr.resetIPSets())
}

func TestConvertAndDeleteCache(t *testing.T) {
	cache := map[string]struct{}{
		"a": {},
		"b": {},
		"c": {},
		"d": {},
	}
	slice := convertAndDeleteCache(cache)
	require.Equal(t, 0, len(cache))
	require.Equal(t, 4, len(slice))
	for _, item := range []string{"a", "b", "c", "d"} {
		success := false
		for _, sliceItem := range slice {
			if item == sliceItem {
				success = true
			}
		}
		if !success {
			require.FailNowf(t, "%s not in the slice", item)
		}
	}
}

// create all possible SetTypes
func TestApplyCreationsAndAdds(t *testing.T) {
	calls := []testutils.TestCmd{fakeRestoreSuccessCommand}
	iMgr := NewIPSetManager(iMgrApplyAllCfg, common.NewMockIOShim(calls))

	lines := []string{
		fmt.Sprintf("-N %s -exist nethash", testNSSet.hashedName),
		fmt.Sprintf("-N %s -exist nethash", testKeyPodSet.hashedName),
		fmt.Sprintf("-N %s -exist nethash", testKVPodSet.hashedName),
		fmt.Sprintf("-N %s -exist hash:ip,port", testNamedportSet.hashedName),
		fmt.Sprintf("-N %s -exist nethash maxelem 4294967295", testCIDRSet.hashedName),
		fmt.Sprintf("-N %s -exist setlist", testKeyNSList.hashedName),
		fmt.Sprintf("-N %s -exist setlist", testKVNSList.hashedName),
		fmt.Sprintf("-N %s -exist setlist", testNestedLabelList.hashedName),
	}
	lines = append(lines, getSortedLines(testNSSet, "10.0.0.0", "10.0.0.1")...)
	lines = append(lines, getSortedLines(testKeyPodSet, "10.0.0.5")...)
	lines = append(lines, getSortedLines(testKVPodSet)...)
	lines = append(lines, getSortedLines(testNamedportSet)...)
	lines = append(lines, getSortedLines(testCIDRSet)...)
	lines = append(lines, getSortedLines(testKeyNSList, testNSSet.hashedName, testKeyPodSet.hashedName)...)
	lines = append(lines, getSortedLines(testKVNSList, testKVPodSet.hashedName)...)
	lines = append(lines, getSortedLines(testNestedLabelList)...)
	expectedFileString := strings.Join(lines, "\n") + "\n"

	iMgr.CreateIPSet(testNSSet.metadata)
	require.NoError(t, iMgr.AddToSet([]*IPSetMetadata{testNSSet.metadata}, "10.0.0.0", "a"))
	require.NoError(t, iMgr.AddToSet([]*IPSetMetadata{testNSSet.metadata}, "10.0.0.1", "b"))
	iMgr.CreateIPSet(testKeyPodSet.metadata)
	require.NoError(t, iMgr.AddToSet([]*IPSetMetadata{testKeyPodSet.metadata}, "10.0.0.5", "c"))
	iMgr.CreateIPSet(testKVPodSet.metadata)
	iMgr.CreateIPSet(testNamedportSet.metadata)
	iMgr.CreateIPSet(testCIDRSet.metadata)
	iMgr.CreateIPSet(testKeyNSList.metadata)
	require.NoError(t, iMgr.AddToList(testKeyNSList.metadata, []*IPSetMetadata{testNSSet.metadata, testKeyPodSet.metadata}))
	iMgr.CreateIPSet(testKVNSList.metadata)
	require.NoError(t, iMgr.AddToList(testKVNSList.metadata, []*IPSetMetadata{testKVPodSet.metadata}))
	iMgr.CreateIPSet(testNestedLabelList.metadata)
	toAddOrUpdateSetNames := []string{
		testNSSet.metadata.GetPrefixName(),
		testKeyPodSet.metadata.GetPrefixName(),
		testKVPodSet.metadata.GetPrefixName(),
		testNamedportSet.metadata.GetPrefixName(),
		testCIDRSet.metadata.GetPrefixName(),
		testKeyNSList.metadata.GetPrefixName(),
		testKVNSList.metadata.GetPrefixName(),
		testNestedLabelList.metadata.GetPrefixName(),
	}
	assertEqualContentsTestHelper(t, toAddOrUpdateSetNames, iMgr.toAddOrUpdateCache)

	creator := iMgr.getFileCreator(1, nil, toAddOrUpdateSetNames)
	actualFileString := getSortedFileString(creator)

	assertEqualFileStrings(t, expectedFileString, actualFileString)
	wasFileAltered, err := creator.RunCommandOnceWithFile(util.Ipset, util.IpsetRestoreFlag)
	require.NoError(t, err)
	require.False(t, wasFileAltered)
}

func TestApplyDeletions(t *testing.T) {
	calls := []testutils.TestCmd{fakeRestoreSuccessCommand}
	iMgr := NewIPSetManager(iMgrApplyAllCfg, common.NewMockIOShim(calls))

	// Remove members and delete others
	iMgr.CreateIPSet(testNSSet.metadata)
	require.NoError(t, iMgr.AddToSet([]*IPSetMetadata{testNSSet.metadata}, "10.0.0.0", "a"))
	require.NoError(t, iMgr.AddToSet([]*IPSetMetadata{testNSSet.metadata}, "10.0.0.1", "b"))
	iMgr.CreateIPSet(testKeyPodSet.metadata)
	iMgr.CreateIPSet(testKeyNSList.metadata)
	require.NoError(t, iMgr.AddToList(testKeyNSList.metadata, []*IPSetMetadata{testNSSet.metadata, testKeyPodSet.metadata}))
	require.NoError(t, iMgr.RemoveFromSet([]*IPSetMetadata{testNSSet.metadata}, "10.0.0.1", "b"))
	require.NoError(t, iMgr.RemoveFromList(testKeyNSList.metadata, []*IPSetMetadata{testKeyPodSet.metadata}))
	iMgr.CreateIPSet(testCIDRSet.metadata)
	iMgr.DeleteIPSet(testCIDRSet.metadata.GetPrefixName())
	iMgr.CreateIPSet(testNestedLabelList.metadata)
	iMgr.DeleteIPSet(testNestedLabelList.metadata.GetPrefixName())

	toDeleteSetNames := []string{testCIDRSet.metadata.GetPrefixName(), testNestedLabelList.metadata.GetPrefixName()}
	assertEqualContentsTestHelper(t, toDeleteSetNames, iMgr.toDeleteCache)
	toAddOrUpdateSetNames := []string{testNSSet.metadata.GetPrefixName(), testKeyPodSet.metadata.GetPrefixName(), testKeyNSList.metadata.GetPrefixName()}
	assertEqualContentsTestHelper(t, toAddOrUpdateSetNames, iMgr.toAddOrUpdateCache)
	creator := iMgr.getFileCreator(1, toDeleteSetNames, toAddOrUpdateSetNames)
	actualFileString := getSortedFileString(creator)

	lines := []string{
		fmt.Sprintf("-F %s", testCIDRSet.hashedName),
		fmt.Sprintf("-F %s", testNestedLabelList.hashedName),
		fmt.Sprintf("-X %s", testCIDRSet.hashedName),
		fmt.Sprintf("-X %s", testNestedLabelList.hashedName),
		fmt.Sprintf("-N %s -exist nethash", testNSSet.hashedName),
		fmt.Sprintf("-N %s -exist nethash", testKeyPodSet.hashedName),
		fmt.Sprintf("-N %s -exist setlist", testKeyNSList.hashedName),
	}
	lines = append(lines, getSortedLines(testNSSet, "10.0.0.0")...)
	lines = append(lines, getSortedLines(testKeyPodSet)...)
	lines = append(lines, getSortedLines(testKeyNSList, testNSSet.hashedName)...)
	expectedFileString := strings.Join(lines, "\n") + "\n"

	assertEqualFileStrings(t, expectedFileString, actualFileString)
	wasFileAltered, err := creator.RunCommandOnceWithFile(util.Ipset, util.IpsetRestoreFlag)
	require.NoError(t, err)
	require.False(t, wasFileAltered)
}

// TODO test that a reconcile list is updated
func TestFailureOnCreation(t *testing.T) {
	setAlreadyExistsCommand := testutils.TestCmd{
		Cmd:      ipsetRestoreStringSlice,
		Stdout:   "Error in line 3: Set cannot be created: set with the same name already exists",
		ExitCode: 1,
	}
	calls := []testutils.TestCmd{setAlreadyExistsCommand, fakeRestoreSuccessCommand}
	iMgr := NewIPSetManager(iMgrApplyAllCfg, common.NewMockIOShim(calls))

	iMgr.CreateIPSet(testNSSet.metadata)
	require.NoError(t, iMgr.AddToSet([]*IPSetMetadata{testNSSet.metadata}, "10.0.0.0", "a"))
	require.NoError(t, iMgr.AddToSet([]*IPSetMetadata{testNSSet.metadata}, "10.0.0.1", "b"))
	iMgr.CreateIPSet(testKeyPodSet.metadata)
	require.NoError(t, iMgr.AddToSet([]*IPSetMetadata{testKeyPodSet.metadata}, "10.0.0.5", "c"))
	iMgr.CreateIPSet(testCIDRSet.metadata)
	iMgr.DeleteIPSet(testCIDRSet.metadata.GetPrefixName())

	toAddOrUpdateSetNames := []string{testNSSet.metadata.GetPrefixName(), testKeyPodSet.metadata.GetPrefixName()}
	assertEqualContentsTestHelper(t, toAddOrUpdateSetNames, iMgr.toAddOrUpdateCache)
	toDeleteSetNames := []string{testCIDRSet.metadata.GetPrefixName()}
	assertEqualContentsTestHelper(t, toDeleteSetNames, iMgr.toDeleteCache)
	creator := iMgr.getFileCreator(2, toDeleteSetNames, toAddOrUpdateSetNames)
	wasFileAltered, err := creator.RunCommandOnceWithFile(util.Ipset, util.IpsetRestoreFlag)
	require.Error(t, err)
	require.True(t, wasFileAltered)

	lines := []string{
		fmt.Sprintf("-F %s", testCIDRSet.hashedName),
		fmt.Sprintf("-X %s", testCIDRSet.hashedName),
		fmt.Sprintf("-N %s -exist nethash", testKeyPodSet.hashedName),
	}
	lines = append(lines, getSortedLines(testKeyPodSet, "10.0.0.5")...)
	expectedFileString := strings.Join(lines, "\n") + "\n"

	actualFileString := getSortedFileString(creator)
	assertEqualFileStrings(t, expectedFileString, actualFileString)
	wasFileAltered, err = creator.RunCommandOnceWithFile(util.Ipset, util.IpsetRestoreFlag)
	require.NoError(t, err)
	require.False(t, wasFileAltered)
}

// TODO test that a reconcile list is updated
func TestFailureOnAddToList(t *testing.T) {
	// This exact scenario wouldn't occur. This error happens when the cache is out of date with the kernel.
	setAlreadyExistsCommand := testutils.TestCmd{
		Cmd:      ipsetRestoreStringSlice,
		Stdout:   "Error in line 12: Set to be added/deleted/tested as element does not exist",
		ExitCode: 1,
	}
	calls := []testutils.TestCmd{setAlreadyExistsCommand, fakeRestoreSuccessCommand}
	iMgr := NewIPSetManager(iMgrApplyAllCfg, common.NewMockIOShim(calls))

	iMgr.CreateIPSet(testNSSet.metadata)
	require.NoError(t, iMgr.AddToSet([]*IPSetMetadata{testNSSet.metadata}, "10.0.0.0", "a"))
	iMgr.CreateIPSet(testKeyPodSet.metadata)
	iMgr.CreateIPSet(testKeyNSList.metadata)
	require.NoError(t, iMgr.AddToList(testKeyNSList.metadata, []*IPSetMetadata{testNSSet.metadata, testKeyPodSet.metadata}))
	iMgr.CreateIPSet(testKVNSList.metadata)
	require.NoError(t, iMgr.AddToList(testKVNSList.metadata, []*IPSetMetadata{testNSSet.metadata}))
	iMgr.CreateIPSet(testCIDRSet.metadata)
	iMgr.DeleteIPSet(testCIDRSet.metadata.GetPrefixName())

	toAddOrUpdateSetNames := []string{
		testNSSet.metadata.GetPrefixName(),
		testKeyPodSet.metadata.GetPrefixName(),
		testKeyNSList.metadata.GetPrefixName(),
		testKVNSList.metadata.GetPrefixName(),
	}
	assertEqualContentsTestHelper(t, toAddOrUpdateSetNames, iMgr.toAddOrUpdateCache)
	toDeleteSetNames := []string{testCIDRSet.metadata.GetPrefixName()}
	assertEqualContentsTestHelper(t, toDeleteSetNames, iMgr.toDeleteCache)
	creator := iMgr.getFileCreator(2, toDeleteSetNames, toAddOrUpdateSetNames)
	originalFileString := creator.ToString()
	wasFileAltered, err := creator.RunCommandOnceWithFile(util.Ipset, util.IpsetRestoreFlag)
	require.Error(t, err)
	require.True(t, wasFileAltered)

	lines := []string{
		fmt.Sprintf("-F %s", testCIDRSet.hashedName),
		fmt.Sprintf("-X %s", testCIDRSet.hashedName),
		fmt.Sprintf("-N %s -exist nethash", testNSSet.hashedName),
		fmt.Sprintf("-N %s -exist nethash", testKeyPodSet.hashedName),
		fmt.Sprintf("-N %s -exist setlist", testKeyNSList.hashedName),
		fmt.Sprintf("-N %s -exist setlist", testKVNSList.hashedName),
	}
	lines = append(lines, getSortedLines(testNSSet, "10.0.0.0")...)
	lines = append(lines, getSortedLines(testKeyPodSet)...)                                                 // line 9
	lines = append(lines, getSortedLines(testKeyNSList, testNSSet.hashedName, testKeyPodSet.hashedName)...) // lines 10, 11, 12
	lines = append(lines, getSortedLines(testKVNSList, testNSSet.hashedName)...)
	expectedFileString := strings.Join(lines, "\n") + "\n"

	// need this because adds are nondeterminstic
	badLine := strings.Split(originalFileString, "\n")[12-1]
	if badLine != fmt.Sprintf("-A %s %s", testKeyNSList.hashedName, testNSSet.hashedName) && badLine != fmt.Sprintf("-A %s %s", testKeyNSList.hashedName, testKeyPodSet.hashedName) {
		require.FailNow(t, "incorrect failed line")
	}
	expectedFileString = strings.ReplaceAll(expectedFileString, badLine+"\n", "")

	actualFileString := getSortedFileString(creator)
	assertEqualFileStrings(t, expectedFileString, actualFileString)
	wasFileAltered, err = creator.RunCommandOnceWithFile(util.Ipset, util.IpsetRestoreFlag)
	require.NoError(t, err)
	require.False(t, wasFileAltered)
}

// TODO test that a reconcile list is updated
func TestFailureOnFlush(t *testing.T) {
	// This exact scenario wouldn't occur. This error happens when the cache is out of date with the kernel.
	setAlreadyExistsCommand := testutils.TestCmd{
		Cmd:      ipsetRestoreStringSlice,
		Stdout:   "Error in line 1: The set with the given name does not exist",
		ExitCode: 1,
	}
	calls := []testutils.TestCmd{setAlreadyExistsCommand, fakeRestoreSuccessCommand}
	iMgr := NewIPSetManager(iMgrApplyAllCfg, common.NewMockIOShim(calls))

	iMgr.CreateIPSet(testNSSet.metadata)
	require.NoError(t, iMgr.AddToSet([]*IPSetMetadata{testNSSet.metadata}, "10.0.0.0", "a"))
	iMgr.CreateIPSet(testKVPodSet.metadata)
	iMgr.DeleteIPSet(testKVPodSet.metadata.GetPrefixName())
	iMgr.CreateIPSet(testCIDRSet.metadata)
	iMgr.DeleteIPSet(testCIDRSet.metadata.GetPrefixName())

	toAddOrUpdateSetNames := []string{testNSSet.metadata.GetPrefixName()}
	assertEqualContentsTestHelper(t, toAddOrUpdateSetNames, iMgr.toAddOrUpdateCache)
	toDeleteSetNames := []string{testKVPodSet.metadata.GetPrefixName(), testCIDRSet.metadata.GetPrefixName()}
	assertEqualContentsTestHelper(t, toDeleteSetNames, iMgr.toDeleteCache)
	creator := iMgr.getFileCreator(2, toDeleteSetNames, toAddOrUpdateSetNames)
	wasFileAltered, err := creator.RunCommandOnceWithFile(util.Ipset, util.IpsetRestoreFlag)
	require.Error(t, err)
	require.True(t, wasFileAltered)

	lines := []string{
		fmt.Sprintf("-F %s", testCIDRSet.hashedName),
		fmt.Sprintf("-X %s", testCIDRSet.hashedName),
		fmt.Sprintf("-N %s -exist nethash", testNSSet.hashedName),
	}
	lines = append(lines, getSortedLines(testNSSet, "10.0.0.0")...)
	expectedFileString := strings.Join(lines, "\n") + "\n"

	actualFileString := getSortedFileString(creator)
	assertEqualFileStrings(t, expectedFileString, actualFileString)
	wasFileAltered, err = creator.RunCommandOnceWithFile(util.Ipset, util.IpsetRestoreFlag)
	require.NoError(t, err)
	require.False(t, wasFileAltered)
}

// TODO test that a reconcile list is updated
func TestFailureOnDeletion(t *testing.T) {
	setAlreadyExistsCommand := testutils.TestCmd{
		Cmd:      ipsetRestoreStringSlice,
		Stdout:   "Error in line 3: Set cannot be destroyed: it is in use by a kernel component",
		ExitCode: 1,
	}
	calls := []testutils.TestCmd{setAlreadyExistsCommand, fakeRestoreSuccessCommand}
	iMgr := NewIPSetManager(iMgrApplyAllCfg, common.NewMockIOShim(calls))

	iMgr.CreateIPSet(testNSSet.metadata)
	require.NoError(t, iMgr.AddToSet([]*IPSetMetadata{testNSSet.metadata}, "10.0.0.0", "a"))
	iMgr.CreateIPSet(testKVPodSet.metadata)
	iMgr.DeleteIPSet(testKVPodSet.metadata.GetPrefixName())
	iMgr.CreateIPSet(testCIDRSet.metadata)
	iMgr.DeleteIPSet(testCIDRSet.metadata.GetPrefixName())

	toAddOrUpdateSetNames := []string{testNSSet.metadata.GetPrefixName()}
	assertEqualContentsTestHelper(t, toAddOrUpdateSetNames, iMgr.toAddOrUpdateCache)
	toDeleteSetNames := []string{testKVPodSet.metadata.GetPrefixName(), testCIDRSet.metadata.GetPrefixName()}
	assertEqualContentsTestHelper(t, toDeleteSetNames, iMgr.toDeleteCache)
	creator := iMgr.getFileCreator(2, toDeleteSetNames, toAddOrUpdateSetNames)
	wasFileAltered, err := creator.RunCommandOnceWithFile(util.Ipset, util.IpsetRestoreFlag)
	require.Error(t, err)
	require.True(t, wasFileAltered)

	lines := []string{
		fmt.Sprintf("-F %s", testKVPodSet.hashedName),
		fmt.Sprintf("-F %s", testCIDRSet.hashedName),
		fmt.Sprintf("-X %s", testCIDRSet.hashedName),
		fmt.Sprintf("-N %s -exist nethash", testNSSet.hashedName),
	}
	lines = append(lines, getSortedLines(testNSSet, "10.0.0.0")...)
	expectedFileString := strings.Join(lines, "\n") + "\n"

	actualFileString := getSortedFileString(creator)
	assertEqualFileStrings(t, expectedFileString, actualFileString)
	wasFileAltered, err = creator.RunCommandOnceWithFile(util.Ipset, util.IpsetRestoreFlag)
	require.NoError(t, err)
	require.False(t, wasFileAltered)
}

// TODO if we add file-level error handlers, add tests for them

func assertEqualContentsTestHelper(t *testing.T, setNames []string, cache map[string]struct{}) {
	require.Equal(t, len(setNames), len(cache), "cache is different than list of set names")
	for _, setName := range setNames {
		_, exists := cache[setName]
		require.True(t, exists, "cache is different than list of set names")
	}
}

// the order of adds is nondeterministic, so we're sorting them
func getSortedLines(set *testSet, members ...string) []string {
	result := []string{fmt.Sprintf("-F %s", set.hashedName)}
	adds := make([]string, len(members))
	for k, member := range members {
		adds[k] = fmt.Sprintf("-A %s %s", set.hashedName, member)
	}
	sort.Strings(adds)
	return append(result, adds...)
}

// the order of adds is nondeterministic, so we're sorting all neighboring adds
func getSortedFileString(creator *ioutil.FileCreator) string {
	lines := strings.Split(creator.ToString(), "\n")

	sortedLines := make([]string, 0)
	k := 0
	for k < len(lines) {
		line := lines[k]
		if !isAddLine(line) {
			sortedLines = append(sortedLines, line)
			k++
			continue
		}
		addLines := make([]string, 0)
		for k < len(lines) {
			line := lines[k]
			if !isAddLine(line) {
				break
			}
			addLines = append(addLines, line)
			k++
		}
		sort.Strings(addLines)
		sortedLines = append(sortedLines, addLines...)
	}
	return strings.Join(sortedLines, "\n")
}

func isAddLine(line string) bool {
	return len(line) >= 2 && line[:2] == "-A"
}

func assertEqualFileStrings(t *testing.T, expectedFileString, actualFileString string) {
	if expectedFileString == actualFileString {
		return
	}
	fmt.Println("EXPECTED FILE STRING:")
	for _, line := range strings.Split(expectedFileString, "\n") {
		fmt.Println(line)
	}
	fmt.Println("ACTUAL FILE STRING")
	for _, line := range strings.Split(actualFileString, "\n") {
		fmt.Println(line)
	}
	require.FailNow(t, "got unexpected file string (see print contents above)")
}
