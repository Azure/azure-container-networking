package ipsets

import (
	"fmt"
	"sort"
	"strings"
	"testing"

	"github.com/Azure/azure-container-networking/common"
	"github.com/Azure/azure-container-networking/npm/pkg/dataplane/ioutil"
	dptestutils "github.com/Azure/azure-container-networking/npm/pkg/dataplane/testutils"
	testutils "github.com/Azure/azure-container-networking/test/utils"
	"github.com/stretchr/testify/require"
)

var (
	ipsetRestoreStringSlice = []string{"ipset", "restore"}
)

func TestDestroyNPMIPSets(t *testing.T) {
	calls := []testutils.TestCmd{} // TODO
	iMgr := NewIPSetManager(iMgrApplyAlwaysCfg, getMockIOShim(calls))
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
	iMgr := NewIPSetManager(iMgrApplyAlwaysCfg, getMockIOShim(calls))

	lines := []string{
		fmt.Sprintf("-N %s -exist nethash", TestNSSet.HashedName),
		fmt.Sprintf("-N %s -exist nethash", TestKeyPodSet.HashedName),
		fmt.Sprintf("-N %s -exist nethash", TestKVPodSet.HashedName),
		fmt.Sprintf("-N %s -exist hash:ip,port", TestNamedportSet.HashedName),
		fmt.Sprintf("-N %s -exist nethash maxelem 4294967295", TestCIDRSet.HashedName),
		fmt.Sprintf("-N %s -exist setlist", TestKeyNSList.HashedName),
		fmt.Sprintf("-N %s -exist setlist", TestKVNSList.HashedName),
		fmt.Sprintf("-N %s -exist setlist", TestNestedLabelList.HashedName),
	}
	lines = append(lines, getSortedLines(TestNSSet, "10.0.0.0", "10.0.0.1")...)
	lines = append(lines, getSortedLines(TestKeyPodSet, "10.0.0.5")...)
	lines = append(lines, getSortedLines(TestKVPodSet)...)
	lines = append(lines, getSortedLines(TestNamedportSet)...)
	lines = append(lines, getSortedLines(TestCIDRSet)...)
	lines = append(lines, getSortedLines(TestKeyNSList, TestNSSet.HashedName, TestKeyPodSet.HashedName)...)
	lines = append(lines, getSortedLines(TestKVNSList, TestKVPodSet.HashedName)...)
	lines = append(lines, getSortedLines(TestNestedLabelList)...)
	expectedFileString := strings.Join(lines, "\n") + "\n"

	iMgr.CreateIPSets([]*IPSetMetadata{TestNSSet.Metadata})
	require.NoError(t, iMgr.AddToSets([]*IPSetMetadata{TestNSSet.Metadata}, "10.0.0.0", "a"))
	require.NoError(t, iMgr.AddToSets([]*IPSetMetadata{TestNSSet.Metadata}, "10.0.0.1", "b"))
	iMgr.CreateIPSets([]*IPSetMetadata{TestKeyPodSet.Metadata})
	require.NoError(t, iMgr.AddToSets([]*IPSetMetadata{TestKeyPodSet.Metadata}, "10.0.0.5", "c"))
	iMgr.CreateIPSets([]*IPSetMetadata{TestKVPodSet.Metadata})
	iMgr.CreateIPSets([]*IPSetMetadata{TestNamedportSet.Metadata})
	iMgr.CreateIPSets([]*IPSetMetadata{TestCIDRSet.Metadata})
	iMgr.CreateIPSets([]*IPSetMetadata{TestKeyNSList.Metadata})
	require.NoError(t, iMgr.AddToLists([]*IPSetMetadata{TestKeyNSList.Metadata}, []*IPSetMetadata{TestNSSet.Metadata, TestKeyPodSet.Metadata}))
	iMgr.CreateIPSets([]*IPSetMetadata{TestKVNSList.Metadata})
	require.NoError(t, iMgr.AddToLists([]*IPSetMetadata{TestKVNSList.Metadata}, []*IPSetMetadata{TestKVPodSet.Metadata}))
	iMgr.CreateIPSets([]*IPSetMetadata{TestNestedLabelList.Metadata})
	toAddOrUpdateSetNames := []string{
		TestNSSet.PrefixName,
		TestKeyPodSet.PrefixName,
		TestKVPodSet.PrefixName,
		TestNamedportSet.PrefixName,
		TestCIDRSet.PrefixName,
		TestKeyNSList.PrefixName,
		TestKVNSList.PrefixName,
		TestNestedLabelList.PrefixName,
	}
	assertEqualContentsTestHelper(t, toAddOrUpdateSetNames, iMgr.toAddOrUpdateCache)

	creator := iMgr.getFileCreator(1, nil, toAddOrUpdateSetNames)
	actualFileString := getSortedFileString(creator)

	dptestutils.AssertEqualMultilineStrings(t, expectedFileString, actualFileString)
	wasFileAltered, err := creator.RunCommandOnceWithFile("ipset", "restore")
	require.NoError(t, err)
	require.False(t, wasFileAltered)
}

func TestApplyDeletions(t *testing.T) {
	calls := []testutils.TestCmd{fakeRestoreSuccessCommand}
	iMgr := NewIPSetManager(iMgrApplyAlwaysCfg, getMockIOShim(calls))

	// Remove members and delete others
	iMgr.CreateIPSets([]*IPSetMetadata{TestNSSet.Metadata})
	require.NoError(t, iMgr.AddToSets([]*IPSetMetadata{TestNSSet.Metadata}, "10.0.0.0", "a"))
	require.NoError(t, iMgr.AddToSets([]*IPSetMetadata{TestNSSet.Metadata}, "10.0.0.1", "b"))
	iMgr.CreateIPSets([]*IPSetMetadata{TestKeyPodSet.Metadata})
	iMgr.CreateIPSets([]*IPSetMetadata{TestKeyNSList.Metadata})
	require.NoError(t, iMgr.AddToLists([]*IPSetMetadata{TestKeyNSList.Metadata}, []*IPSetMetadata{TestNSSet.Metadata, TestKeyPodSet.Metadata}))
	require.NoError(t, iMgr.RemoveFromSets([]*IPSetMetadata{TestNSSet.Metadata}, "10.0.0.1", "b"))
	require.NoError(t, iMgr.RemoveFromList(TestKeyNSList.Metadata, []*IPSetMetadata{TestKeyPodSet.Metadata}))
	iMgr.CreateIPSets([]*IPSetMetadata{TestCIDRSet.Metadata})
	iMgr.DeleteIPSet(TestCIDRSet.PrefixName)
	iMgr.CreateIPSets([]*IPSetMetadata{TestNestedLabelList.Metadata})
	iMgr.DeleteIPSet(TestNestedLabelList.PrefixName)

	toDeleteSetNames := []string{TestCIDRSet.PrefixName, TestNestedLabelList.PrefixName}
	assertEqualContentsTestHelper(t, toDeleteSetNames, iMgr.toDeleteCache)
	toAddOrUpdateSetNames := []string{TestNSSet.PrefixName, TestKeyPodSet.PrefixName, TestKeyNSList.PrefixName}
	assertEqualContentsTestHelper(t, toAddOrUpdateSetNames, iMgr.toAddOrUpdateCache)
	creator := iMgr.getFileCreator(1, toDeleteSetNames, toAddOrUpdateSetNames)
	actualFileString := getSortedFileString(creator)

	lines := []string{
		fmt.Sprintf("-F %s", TestCIDRSet.HashedName),
		fmt.Sprintf("-F %s", TestNestedLabelList.HashedName),
		fmt.Sprintf("-X %s", TestCIDRSet.HashedName),
		fmt.Sprintf("-X %s", TestNestedLabelList.HashedName),
		fmt.Sprintf("-N %s -exist nethash", TestNSSet.HashedName),
		fmt.Sprintf("-N %s -exist nethash", TestKeyPodSet.HashedName),
		fmt.Sprintf("-N %s -exist setlist", TestKeyNSList.HashedName),
	}
	lines = append(lines, getSortedLines(TestNSSet, "10.0.0.0")...)
	lines = append(lines, getSortedLines(TestKeyPodSet)...)
	lines = append(lines, getSortedLines(TestKeyNSList, TestNSSet.HashedName)...)
	expectedFileString := strings.Join(lines, "\n") + "\n"

	dptestutils.AssertEqualMultilineStrings(t, expectedFileString, actualFileString)
	wasFileAltered, err := creator.RunCommandOnceWithFile("ipset", "restore")
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
	iMgr := NewIPSetManager(iMgrApplyAlwaysCfg, getMockIOShim(calls))

	iMgr.CreateIPSets([]*IPSetMetadata{TestNSSet.Metadata})
	require.NoError(t, iMgr.AddToSets([]*IPSetMetadata{TestNSSet.Metadata}, "10.0.0.0", "a"))
	require.NoError(t, iMgr.AddToSets([]*IPSetMetadata{TestNSSet.Metadata}, "10.0.0.1", "b"))
	iMgr.CreateIPSets([]*IPSetMetadata{TestKeyPodSet.Metadata})
	require.NoError(t, iMgr.AddToSets([]*IPSetMetadata{TestKeyPodSet.Metadata}, "10.0.0.5", "c"))
	iMgr.CreateIPSets([]*IPSetMetadata{TestCIDRSet.Metadata})
	iMgr.DeleteIPSet(TestCIDRSet.PrefixName)

	toAddOrUpdateSetNames := []string{TestNSSet.PrefixName, TestKeyPodSet.PrefixName}
	assertEqualContentsTestHelper(t, toAddOrUpdateSetNames, iMgr.toAddOrUpdateCache)
	toDeleteSetNames := []string{TestCIDRSet.PrefixName}
	assertEqualContentsTestHelper(t, toDeleteSetNames, iMgr.toDeleteCache)
	creator := iMgr.getFileCreator(2, toDeleteSetNames, toAddOrUpdateSetNames)
	wasFileAltered, err := creator.RunCommandOnceWithFile("ipset", "restore")
	require.Error(t, err)
	require.True(t, wasFileAltered)

	lines := []string{
		fmt.Sprintf("-F %s", TestCIDRSet.HashedName),
		fmt.Sprintf("-X %s", TestCIDRSet.HashedName),
		fmt.Sprintf("-N %s -exist nethash", TestKeyPodSet.HashedName),
	}
	lines = append(lines, getSortedLines(TestKeyPodSet, "10.0.0.5")...)
	expectedFileString := strings.Join(lines, "\n") + "\n"

	actualFileString := getSortedFileString(creator)
	dptestutils.AssertEqualMultilineStrings(t, expectedFileString, actualFileString)
	wasFileAltered, err = creator.RunCommandOnceWithFile("ipset", "restore")
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
	iMgr := NewIPSetManager(iMgrApplyAlwaysCfg, getMockIOShim(calls))

	iMgr.CreateIPSets([]*IPSetMetadata{TestNSSet.Metadata})
	require.NoError(t, iMgr.AddToSets([]*IPSetMetadata{TestNSSet.Metadata}, "10.0.0.0", "a"))
	iMgr.CreateIPSets([]*IPSetMetadata{TestKeyPodSet.Metadata})
	iMgr.CreateIPSets([]*IPSetMetadata{TestKeyNSList.Metadata})
	require.NoError(t, iMgr.AddToLists([]*IPSetMetadata{TestKeyNSList.Metadata}, []*IPSetMetadata{TestNSSet.Metadata, TestKeyPodSet.Metadata}))
	iMgr.CreateIPSets([]*IPSetMetadata{TestKVNSList.Metadata})
	require.NoError(t, iMgr.AddToLists([]*IPSetMetadata{TestKVNSList.Metadata}, []*IPSetMetadata{TestNSSet.Metadata}))
	iMgr.CreateIPSets([]*IPSetMetadata{TestCIDRSet.Metadata})
	iMgr.DeleteIPSet(TestCIDRSet.PrefixName)

	toAddOrUpdateSetNames := []string{
		TestNSSet.PrefixName,
		TestKeyPodSet.PrefixName,
		TestKeyNSList.PrefixName,
		TestKVNSList.PrefixName,
	}
	assertEqualContentsTestHelper(t, toAddOrUpdateSetNames, iMgr.toAddOrUpdateCache)
	toDeleteSetNames := []string{TestCIDRSet.PrefixName}
	assertEqualContentsTestHelper(t, toDeleteSetNames, iMgr.toDeleteCache)
	creator := iMgr.getFileCreator(2, toDeleteSetNames, toAddOrUpdateSetNames)
	originalFileString := creator.ToString()
	wasFileAltered, err := creator.RunCommandOnceWithFile("ipset", "restore")
	require.Error(t, err)
	require.True(t, wasFileAltered)

	lines := []string{
		fmt.Sprintf("-F %s", TestCIDRSet.HashedName),
		fmt.Sprintf("-X %s", TestCIDRSet.HashedName),
		fmt.Sprintf("-N %s -exist nethash", TestNSSet.HashedName),
		fmt.Sprintf("-N %s -exist nethash", TestKeyPodSet.HashedName),
		fmt.Sprintf("-N %s -exist setlist", TestKeyNSList.HashedName),
		fmt.Sprintf("-N %s -exist setlist", TestKVNSList.HashedName),
	}
	lines = append(lines, getSortedLines(TestNSSet, "10.0.0.0")...)
	lines = append(lines, getSortedLines(TestKeyPodSet)...)                                                 // line 9
	lines = append(lines, getSortedLines(TestKeyNSList, TestNSSet.HashedName, TestKeyPodSet.HashedName)...) // lines 10, 11, 12
	lines = append(lines, getSortedLines(TestKVNSList, TestNSSet.HashedName)...)
	expectedFileString := strings.Join(lines, "\n") + "\n"

	// need this because adds are nondeterminstic
	badLine := strings.Split(originalFileString, "\n")[12-1]
	if badLine != fmt.Sprintf("-A %s %s", TestKeyNSList.HashedName, TestNSSet.HashedName) && badLine != fmt.Sprintf("-A %s %s", TestKeyNSList.HashedName, TestKeyPodSet.HashedName) {
		require.FailNow(t, "incorrect failed line")
	}
	expectedFileString = strings.ReplaceAll(expectedFileString, badLine+"\n", "")

	actualFileString := getSortedFileString(creator)
	dptestutils.AssertEqualMultilineStrings(t, expectedFileString, actualFileString)
	wasFileAltered, err = creator.RunCommandOnceWithFile("ipset", "restore")
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
	iMgr := NewIPSetManager(iMgrApplyAlwaysCfg, getMockIOShim(calls))

	iMgr.CreateIPSets([]*IPSetMetadata{TestNSSet.Metadata})
	require.NoError(t, iMgr.AddToSets([]*IPSetMetadata{TestNSSet.Metadata}, "10.0.0.0", "a"))
	iMgr.CreateIPSets([]*IPSetMetadata{TestKVPodSet.Metadata})
	iMgr.DeleteIPSet(TestKVPodSet.PrefixName)
	iMgr.CreateIPSets([]*IPSetMetadata{TestCIDRSet.Metadata})
	iMgr.DeleteIPSet(TestCIDRSet.PrefixName)

	toAddOrUpdateSetNames := []string{TestNSSet.PrefixName}
	assertEqualContentsTestHelper(t, toAddOrUpdateSetNames, iMgr.toAddOrUpdateCache)
	toDeleteSetNames := []string{TestKVPodSet.PrefixName, TestCIDRSet.PrefixName}
	assertEqualContentsTestHelper(t, toDeleteSetNames, iMgr.toDeleteCache)
	creator := iMgr.getFileCreator(2, toDeleteSetNames, toAddOrUpdateSetNames)
	wasFileAltered, err := creator.RunCommandOnceWithFile("ipset", "restore")
	require.Error(t, err)
	require.True(t, wasFileAltered)

	lines := []string{
		fmt.Sprintf("-F %s", TestCIDRSet.HashedName),
		fmt.Sprintf("-X %s", TestCIDRSet.HashedName),
		fmt.Sprintf("-N %s -exist nethash", TestNSSet.HashedName),
	}
	lines = append(lines, getSortedLines(TestNSSet, "10.0.0.0")...)
	expectedFileString := strings.Join(lines, "\n") + "\n"

	actualFileString := getSortedFileString(creator)
	dptestutils.AssertEqualMultilineStrings(t, expectedFileString, actualFileString)
	wasFileAltered, err = creator.RunCommandOnceWithFile("ipset", "restore")
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
	iMgr := NewIPSetManager(iMgrApplyAlwaysCfg, getMockIOShim(calls))

	iMgr.CreateIPSets([]*IPSetMetadata{TestNSSet.Metadata})
	require.NoError(t, iMgr.AddToSets([]*IPSetMetadata{TestNSSet.Metadata}, "10.0.0.0", "a"))
	iMgr.CreateIPSets([]*IPSetMetadata{TestKVPodSet.Metadata})
	iMgr.DeleteIPSet(TestKVPodSet.PrefixName)
	iMgr.CreateIPSets([]*IPSetMetadata{TestCIDRSet.Metadata})
	iMgr.DeleteIPSet(TestCIDRSet.PrefixName)

	toAddOrUpdateSetNames := []string{TestNSSet.PrefixName}
	assertEqualContentsTestHelper(t, toAddOrUpdateSetNames, iMgr.toAddOrUpdateCache)
	toDeleteSetNames := []string{TestKVPodSet.PrefixName, TestCIDRSet.PrefixName}
	assertEqualContentsTestHelper(t, toDeleteSetNames, iMgr.toDeleteCache)
	creator := iMgr.getFileCreator(2, toDeleteSetNames, toAddOrUpdateSetNames)
	wasFileAltered, err := creator.RunCommandOnceWithFile("ipset", "restore")
	require.Error(t, err)
	require.True(t, wasFileAltered)

	lines := []string{
		fmt.Sprintf("-F %s", TestKVPodSet.HashedName),
		fmt.Sprintf("-F %s", TestCIDRSet.HashedName),
		fmt.Sprintf("-X %s", TestCIDRSet.HashedName),
		fmt.Sprintf("-N %s -exist nethash", TestNSSet.HashedName),
	}
	lines = append(lines, getSortedLines(TestNSSet, "10.0.0.0")...)
	expectedFileString := strings.Join(lines, "\n") + "\n"

	actualFileString := getSortedFileString(creator)
	dptestutils.AssertEqualMultilineStrings(t, expectedFileString, actualFileString)
	wasFileAltered, err = creator.RunCommandOnceWithFile("ipset", "restore")
	require.NoError(t, err)
	require.False(t, wasFileAltered)
}

// TODO if we add file-level error handlers, add tests for them
// the order of adds is nondeterministic, so we're sorting them
func getSortedLines(set *TestSet, members ...string) []string {
	result := []string{fmt.Sprintf("-F %s", set.HashedName)}
	adds := make([]string, len(members))
	for k, member := range members {
		adds[k] = fmt.Sprintf("-A %s %s", set.HashedName, member)
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
