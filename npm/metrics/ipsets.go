package metrics

import (
	"github.com/Azure/azure-container-networking/npm/util"
	"github.com/prometheus/client_golang/prometheus"
)

var ipsetInventoryMap map[string]int

// inventorySeries holds the set names that ipset_counts currently reports, so the number of
// series stays bounded and a set that is already reported keeps reporting.
var inventorySeries map[string]struct{}

// maxIPSetInventorySeries bounds how many individual ipsets the ipset_counts metric reports.
// That series is labelled by set name, and NPM creates a set per distinct pod label, so its
// cardinality follows workload labels rather than anything an operator controls: one pod
// carrying tens of thousands of labels otherwise adds that many series on every node, which
// both retains them in the agent and inflates the response built for each scrape of an
// endpoint served on the host network. The aggregate counters stay exact past the bound and
// nothing NPM does reads the breakdown, so only the per-set detail stops growing; an operator
// can tell it is incomplete by comparing the reported series against num_ipsets. The bound is
// far above the number of sets a cluster's namespaces, policies and workloads produce.
const maxIPSetInventorySeries = 20000

// AddPod increments the number of Pod IPs.
func AddPod() {
	podsWatched.Inc()
}

// RemovePod decrements the number of Pod IPs.
func RemovePod() {
	podsWatched.Dec()
}

// PodCount gets the current number of Pod IPs.
// This function is slow.
func PodCount() (int, error) {
	return getValue(podsWatched)
}

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
	for setName := range ipsetInventoryMap {
		removeFromIPSetInventory(setName)
	}
	ipsetInventoryMap = make(map[string]int)
	inventorySeries = make(map[string]struct{})
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
	if !canReportIPSetInventory(setName) {
		return
	}
	labels := getIPSetInventoryLabels(setName)
	val := getEntryCountForIPSet(setName)
	ipsetInventory.With(labels).Set(val)
}

// canReportIPSetInventory reports whether setName may hold a per-set series, claiming a slot
// for it the first time. A set that already has a series keeps it, so an ipset's reported
// count does not flap once established.
func canReportIPSetInventory(setName string) bool {
	if _, reported := inventorySeries[setName]; reported {
		return true
	}
	if len(inventorySeries) >= maxIPSetInventorySeries {
		return false
	}
	inventorySeries[setName] = struct{}{}
	return true
}

func removeFromIPSetInventory(setName string) {
	delete(inventorySeries, setName)
	labels := getIPSetInventoryLabels(setName)
	ipsetInventory.Delete(labels)
}

func getEntryCountForIPSet(setName string) float64 {
	return float64(ipsetInventoryMap[setName]) // returns 0 if setName doesn't exist
}

func getIPSetInventoryLabels(setName string) prometheus.Labels {
	return prometheus.Labels{setNameLabel: setName, setHashLabel: util.GetHashedName(setName)}
}
