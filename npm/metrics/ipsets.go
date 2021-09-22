package metrics

import (
	"github.com/Azure/azure-container-networking/npm/util"
	"github.com/prometheus/client_golang/prometheus"
)

var ipsetInventoryMap = make(map[string]int)

// IncNumIPSets increments the number of IPSets.
func IncNumIPSets() {
	numIPSets.Inc()
}

// DecNumIPSets decrements the number of IPSets.
func DecNumIPSets() {
	numIPSets.Dec()
}

// SetNumIPSets sets the number of IPsets to val.
func SetNumIPSets(val int) {
	numIPSets.Set(float64(val))
}

// ResetNumIPSets sets the number of IPSets to 0.
func ResetNumIPSets() {
	numIPSets.Set(0)
}

// NumIPSetsIsPositive is true when the number of IPSets is positive.
// This function is slow.
// TODO might be more efficient to keep track of the count
func NumIPSetsIsPositive() bool {
	val, err := GetNumIPSets()
	return val > 0 && err == nil
}

// RecordIPSetExecTime adds an observation of execution time for adding an IPSet.
// The execution time is from the timer's start until now.
func RecordIPSetExecTime(timer *Timer) {
	timer.stopAndRecord(addIPSetExecTime)
}

// AddEntryToIPSet increments the number of entries for IPSet setName.
// It doesn't ever update the number of IPSets.
func AddEntryToIPSet(setName string) {
	numIPSetEntries.Inc()
	ipsetInventoryMap[setName]++ // adds the setName with value 1 if it doesn't exist
	updateIPSetInventory(setName)
}

// RemoveEntryFromIPSet decrements the number of entries for IPSet setName.
func RemoveEntryFromIPSet(setName string) {
	_, exists := ipsetInventoryMap[setName]
	if exists {
		numIPSetEntries.Dec()
		ipsetInventoryMap[setName]--
		if ipsetInventoryMap[setName] == 0 {
			removeFromIPSetInventory(setName)
		} else {
			updateIPSetInventory(setName)
		}
	}
}

// RemoveAllEntriesFromIPSet sets the number of entries for ipset setName to 0.
// It doesn't ever update the number of IPSets.
func RemoveAllEntriesFromIPSet(setName string) {
	numIPSetEntries.Add(-getEntryCountForIPSet(setName))
	delete(ipsetInventoryMap, setName)
	removeFromIPSetInventory(setName)
}

// DeleteIPSet decrements the number of IPSets and resets the number of entries for ipset setName to 0.
func DeleteIPSet(setName string) {
	RemoveAllEntriesFromIPSet(setName)
	DecNumIPSets()
}

// ResetIPSetEntries sets the number of entries to 0 for all IPSets.
// It doesn't ever update the number of IPSets.
func ResetIPSetEntries() {
	numIPSetEntries.Set(0)
	ipsetInventoryMap = make(map[string]int)
}

// GetNumIPSets returns the number of IPSets.
// This function is slow.
func GetNumIPSets() (int, error) {
	return getValue(numIPSets)
}

// GetNumIPSetEntries returns the total number of IPSet entries.
// This function is slow.
func GetNumIPSetEntries() (int, error) {
	return getValue(numIPSetEntries)
}

// GetNumEntriesForIPSet returns the number entries for IPSet setName.
// This function is slow.
// TODO could use the map if this function needs to be faster.
// If updated, replace GetNumEntriesForIPSet() with getVecValue() in assertEqualMapAndMetricElements() in ipsets_test.go
func GetNumEntriesForIPSet(setName string) (int, error) {
	labels := getIPSetInventoryLabels(setName)
	return getVecValue(ipsetInventory, labels)
}

// GetIPSetExecCount returns the number of observations for execution time of adding IPSets.
// This function is slow.
func GetIPSetExecCount() (int, error) {
	return getCountValue(addIPSetExecTime)
}

func updateIPSetInventory(setName string) {
	labels := getIPSetInventoryLabels(setName)
	val := getEntryCountForIPSet(setName)
	ipsetInventory.With(labels).Set(val)
}

func removeFromIPSetInventory(setName string) {
	labels := getIPSetInventoryLabels(setName)
	ipsetInventory.Delete(labels)
}

func getEntryCountForIPSet(setName string) float64 {
	return float64(ipsetInventoryMap[setName]) // returns 0 if setName doesn't exist
}

func getIPSetInventoryLabels(setName string) prometheus.Labels {
	return prometheus.Labels{setNameLabel: setName, setHashLabel: util.GetHashedName(setName)}
}
