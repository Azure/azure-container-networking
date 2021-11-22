package metrics

import (
	"testing"

	"github.com/Azure/azure-container-networking/npm/metrics/promutil"
	"github.com/stretchr/testify/require"
)

const (
	testName1 = "test-set1"
	testName2 = "test-set2"
)

var (
	numSetsMetric = &basicMetric{ResetNumIPSets, IncNumIPSets, DecNumIPSets, GetNumIPSets}
	setExecMetric = &recordingMetric{RecordIPSetExecTime, GetIPSetExecCount}
)

type testSet struct {
	name       string
	entryCount int
}

func TestRecordIPSetExecTime(t *testing.T) {
	testStopAndRecord(t, setExecMetric)
}

func TestIncNumIPSets(t *testing.T) {
	testIncMetric(t, numSetsMetric)
}

func TestDecNumIPSets(t *testing.T) {
	testDecMetric(t, numSetsMetric)
}

func TestResetNumIPSets(t *testing.T) {
	testResetMetric(t, numSetsMetric)
}

func TestSetNumIPSets(t *testing.T) {
	ResetIPSetEntries()
	SetNumIPSets(10)
	assertMetricVal(t, numSetsMetric, 10)
	SetNumIPSets(0)
	assertMetricVal(t, numSetsMetric, 0)
}

func TestNumIPSetsIsPositive(t *testing.T) {
	ResetIPSetEntries()
	assertNotPositive(t)

	IncNumIPSets()
	DecNumIPSets()
	assertNotPositive(t)

	IncNumIPSets()
	assertPositive(t)

	SetNumIPSets(5)
	assertPositive(t)

	SetNumIPSets(0)
	assertNotPositive(t)

	DecNumIPSets()
	assertNotPositive(t)
}

func assertNotPositive(t *testing.T) {
	if NumIPSetsIsPositive() {
		numSets, _ := GetNumIPSets()
		require.FailNowf(t, "", "expected num IPSets not to be positive. Current number: %d", numSets)
	}
}

func assertPositive(t *testing.T) {
	if !NumIPSetsIsPositive() {
		numSets, _ := GetNumIPSets()
		require.FailNowf(t, "", "expected num IPSets to be positive. Current number: %d", numSets)
	}
}

func TestAddEntryToIPSet(t *testing.T) {
	ResetIPSetEntries()
	AddEntryToIPSet(testName1)
	AddEntryToIPSet(testName1)
	AddEntryToIPSet(testName2)
	assertNumEntriesAndCounts(t, &testSet{testName1, 2}, &testSet{testName2, 1})
	assertMapIsGood(t)
}

func assertNumEntriesAndCounts(t *testing.T, sets ...*testSet) {
	expectedTotal := 0
	for _, set := range sets {
		val, exists := ipsetInventoryMap[set.name]
		if exists && set.entryCount == 0 {
			require.FailNowf(t, "", "expected set %s to not exist since we expect an entry count of 0", set.name)
		}
		if !exists && set.entryCount > 0 {
			require.FailNowf(t, "", "expected set %s to exist in map for ipset entries", set.name)
		}
		if set.entryCount != val {
			require.FailNowf(t, "", "set %s has incorrect number of entries in the map. Expected %d, got %d", set.name, set.entryCount, val)
		}
		prometheusVal, err := GetNumEntriesForIPSet(set.name)
		require.NoError(t, err, "unexpectedly got an error when getting prometheus num entries for set %s", set.name)
		if set.entryCount != prometheusVal {
			require.FailNow(t, "", "set %s has incorrect number of entries in the Prometheus metric. Expected %d, got %d", set.name, set.entryCount, prometheusVal)
		}
		expectedTotal += val
	}

	numEntries, err := GetNumIPSetEntries()
	promutil.NotifyIfErrors(t, err)
	if numEntries != expectedTotal {
		require.FailNowf(t, "", "incorrect numver of entries. Expected %d, got %d", expectedTotal, numEntries)
	}
}

func assertMapIsGood(t *testing.T) {
	assertEqualMapAndMetricElements(t)
	assertNoZeroEntriesInMap(t)
}

// can't get all of a GaugeVec's labels, so need to trust code to delete labels in GaugeVec when done
func assertEqualMapAndMetricElements(t *testing.T) {
	for setName, mapVal := range ipsetInventoryMap {
		val, err := GetNumEntriesForIPSet(setName)
		promutil.NotifyIfErrors(t, err)
		promutil.NotifyIfErrors(t, err)
		if mapVal != val {
			require.FailNowf(t, "", "set %s has incorrect number of entries. Metric has %d, map has %d", setName, val, mapVal)
		}
	}
}

func assertNoZeroEntriesInMap(t *testing.T) {
	for setname, mapVal := range ipsetInventoryMap {
		if mapVal <= 0 {
			require.FailNowf(t, "", "expected all ipset entry counts to be positive, but got %d for set %s", mapVal, setname)
		}
	}
}

func TestRemoveEntryFromIPSet(t *testing.T) {
	ResetIPSetEntries()
	AddEntryToIPSet(testName1)
	AddEntryToIPSet(testName1)
	AddEntryToIPSet(testName2)
	RemoveEntryFromIPSet(testName1)
	assertNumEntriesAndCounts(t, &testSet{testName1, 1}, &testSet{testName2, 1})
	assertMapIsGood(t)
}

func TestRemoveAllEntriesFromIPSet(t *testing.T) {
	ResetIPSetEntries()
	AddEntryToIPSet(testName1)
	AddEntryToIPSet(testName1)
	AddEntryToIPSet(testName2)
	RemoveAllEntriesFromIPSet(testName1)
	assertNumEntriesAndCounts(t, &testSet{testName1, 0}, &testSet{testName2, 1})
	assertMapIsGood(t)
}

func TestDeleteIPSet(t *testing.T) {
	ResetNumIPSets()
	ResetIPSetEntries()
	IncNumIPSets()
	AddEntryToIPSet(testName1)
	AddEntryToIPSet(testName1)
	IncNumIPSets()
	AddEntryToIPSet(testName2)
	DeleteIPSet(testName1)
	assertNumSets(t, 1)
	assertNumEntriesAndCounts(t, &testSet{testName1, 0}, &testSet{testName2, 1})
	assertMapIsGood(t)
}

func assertNumSets(t *testing.T, exectedVal int) {
	numSets, err := GetNumIPSets()
	promutil.NotifyIfErrors(t, err)
	if numSets != exectedVal {
		require.FailNowf(t, "", "incorrect number of ipsets. Expected %d, got %d", exectedVal, numSets)
	}
}

func TestResetIPSetEntries(t *testing.T) {
	AddEntryToIPSet(testName1)
	AddEntryToIPSet(testName1)
	AddEntryToIPSet(testName2)
	ResetIPSetEntries()
	assertNumEntriesAndCounts(t, &testSet{testName1, 0}, &testSet{testName2, 0})
	assertMapIsGood(t)
}
