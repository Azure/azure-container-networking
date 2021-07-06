// Package ipsm focus on ip set operation
// Copyright 2018 Microsoft. All rights reserved.
// MIT License
package ipsm

import (
	"fmt"
	"os"
	"os/exec"
	"regexp"
	"strings"
	"syscall"

	"github.com/Azure/azure-container-networking/log"
	"github.com/Azure/azure-container-networking/npm/metrics"
	"github.com/Azure/azure-container-networking/npm/util"
	utilexec "k8s.io/utils/exec"
)

// ReferCountOperation is used to indicate whether ipset refer count should be increased or decreased.
type ReferCountOperation bool

const (
	IncrementOp ReferCountOperation = true
	DecrementOp ReferCountOperation = false
)

type ipsEntry struct {
	operationFlag string
	name          string
	set           string
	spec          []string
}

// IpsetManager stores ipset states.
type IpsetManager struct {
	exec    utilexec.Interface
	ListMap map[string]*Ipset //tracks all set lists.
	SetMap  map[string]*Ipset //label -> []ip
}

// Ipset represents one ipset entry.
type Ipset struct {
	name       string
	elements   map[string]string // key = ip, value: context associated to the ip like podKey
	referCount int
}

func (ipset *Ipset) incReferCount() {
	ipset.referCount++
}

func (ipset *Ipset) decReferCount() {
	ipset.referCount--
}

// NewIpset creates a new instance for Ipset object.
func NewIpset(setName string) *Ipset {
	return &Ipset{
		name:       setName,
		elements:   make(map[string]string),
		referCount: 0,
	}
}

// NewIpsetManager creates a new instance for IpsetManager object.
func NewIpsetManager(exec utilexec.Interface) *IpsetManager {
	return &IpsetManager{
		exec:    exec,
		ListMap: make(map[string]*Ipset),
		SetMap:  make(map[string]*Ipset),
	}
}

// Exists checks if an element exists in setMap/listMap.
func (ipsMgr *IpsetManager) Exists(listName string, setName string, kind string) bool {
	m := ipsMgr.SetMap
	if kind == util.IpsetSetListFlag {
		m = ipsMgr.ListMap
	}

	if _, exists := m[listName]; !exists {
		return false
	}

	if _, exists := m[listName].elements[setName]; !exists {
		return false
	}

	return true
}

// IpSetReferIncOrDec checks if an element exists in setMap/listMap and then increases or decreases this referCount.
func (ipsMgr *IpsetManager) IpSetReferIncOrDec(ipsetName string, kind string, countOperation ReferCountOperation) {
	m := ipsMgr.SetMap
	if kind == util.IpsetSetListFlag {
		m = ipsMgr.ListMap
	}

	switch countOperation {
	case IncrementOp:
		m[ipsetName].incReferCount()
	case DecrementOp:
		m[ipsetName].decReferCount()
	}
}

// SetExists checks if an ipset exists, and returns the type
func (ipsMgr *IpsetManager) SetExists(setName string) (bool, string) {
	_, exists := ipsMgr.SetMap[setName]
	if exists {
		return exists, util.IpsetSetGenericFlag
	}

	_, exists = ipsMgr.ListMap[setName]
	if exists {
		return exists, util.IpsetSetListFlag
	}

	return exists, ""
}

func isNsSet(setName string) bool {
	return !strings.Contains(setName, "-") && !strings.Contains(setName, ":")
}

// CreateList creates an ipset list. npm maintains one setlist per namespace label.
func (ipsMgr *IpsetManager) CreateList(listName string) error {
	if _, exists := ipsMgr.ListMap[listName]; exists {
		return nil
	}

	entry := &ipsEntry{
		name:          listName,
		operationFlag: util.IpsetCreationFlag,
		set:           util.GetHashedName(listName),
		spec:          []string{util.IpsetSetListFlag},
	}
	log.Logf("Creating List: %+v", entry)
	if errCode, err := ipsMgr.Run(entry); err != nil && errCode != 1 {
		metrics.SendErrorLogAndMetric(util.IpsmID, "Error: failed to create ipset list %s.", listName)
		return err
	}

	ipsMgr.ListMap[listName] = NewIpset(listName)

	return nil
}

// DeleteList removes an ipset list.
func (ipsMgr *IpsetManager) DeleteList(listName string) error {
	entry := &ipsEntry{
		operationFlag: util.IpsetDestroyFlag,
		set:           util.GetHashedName(listName),
	}

	if ipsMgr.ListMap[listName].referCount > 0 {
		ipsMgr.IpSetReferIncOrDec(listName, util.IpsetSetListFlag, DecrementOp)
		return nil
	}

	if errCode, err := ipsMgr.Run(entry); err != nil {
		if errCode == 1 {
			return nil
		}

		metrics.SendErrorLogAndMetric(util.IpsmID, "Error: failed to delete ipset %s %+v", listName, entry)
		return err
	}

	delete(ipsMgr.ListMap, listName)

	return nil
}

// AddToList inserts an ipset to an ipset list.
func (ipsMgr *IpsetManager) AddToList(listName string, setName string) error {
	if listName == setName {
		return nil
	}

	//Check if list being added exists in the listmap, if it exists we don't care about the set type
	exists, _ := ipsMgr.SetExists(setName)

	// if set does not exist, then return because the ipset call will fail due to set not existing
	if !exists {
		return fmt.Errorf("Set [%s] does not exist when attempting to add to list [%s]", setName, listName)
	}

	// Check if the list that is being added to exists
	exists, listtype := ipsMgr.SetExists(listName)

	// Make sure that set returned is of list type, otherwise return because we can't add a set to a non setlist type
	if exists && listtype != util.IpsetSetListFlag {
		return fmt.Errorf("Failed to add set [%s] to list [%s], but list is of type [%s]", setName, listName, listtype)
	} else if !exists {
		// if the list doesn't exist, create it
		if err := ipsMgr.CreateList(listName); err != nil {
			return err
		}
	}

	// check if set already exists in the list
	if ipsMgr.Exists(listName, setName, util.IpsetSetListFlag) {
		return nil
	}

	entry := &ipsEntry{
		operationFlag: util.IpsetAppendFlag,
		set:           util.GetHashedName(listName),
		spec:          []string{util.GetHashedName(setName)},
	}

	// add set to list
	if errCode, err := ipsMgr.Run(entry); err != nil && errCode != 1 {
		return fmt.Errorf("Error: failed to create ipset rules. rule: %+v, error: %v", entry, err)
	}

	ipsMgr.ListMap[listName].elements[setName] = ""

	return nil
}

// DeleteFromList removes an ipset to an ipset list.
func (ipsMgr *IpsetManager) DeleteFromList(listName string, setName string) error {

	//Check if list being added exists in the listmap, if it exists we don't care about the set type
	exists, _ := ipsMgr.SetExists(setName)

	// if set does not exist, then return because the ipset call will fail due to set not existing
	// TODO make sure these are info and not errors, use NPmErr
	if !exists {
		metrics.SendErrorLogAndMetric(util.IpsmID, "Set [%s] does not exist when attempting to delete from list [%s]", setName, listName)
		return nil
	}

	//Check if list being added exists in the listmap, if it exists we don't care about the set type
	exists, listtype := ipsMgr.SetExists(listName)

	// if set does not exist, then return because the ipset call will fail due to set not existing
	if !exists {
		metrics.SendErrorLogAndMetric(util.IpsmID, "Set [%s] does not exist when attempting to add to list [%s]", setName, listName)
		return nil
	}

	if listtype != util.IpsetSetListFlag {
		metrics.SendErrorLogAndMetric(util.IpsmID, "Set [%s] is of the wrong type when attempting to delete list [%s], actual type [%s]", setName, listName, listtype)
		return nil
	}

	if _, exists := ipsMgr.ListMap[listName]; !exists {
		metrics.SendErrorLogAndMetric(util.IpsmID, "ipset list with name %s not found", listName)
		return nil
	}

	hashedListName, hashedSetName := util.GetHashedName(listName), util.GetHashedName(setName)
	entry := &ipsEntry{
		operationFlag: util.IpsetDeletionFlag,
		set:           hashedListName,
		spec:          []string{hashedSetName},
	}

	if _, err := ipsMgr.Run(entry); err != nil {
		metrics.SendErrorLogAndMetric(util.IpsmID, "Error: failed to delete ipset entry. %+v", entry)
		return err
	}

	// Now cleanup the cache
	if _, exists := ipsMgr.ListMap[listName].elements[setName]; exists {
		delete(ipsMgr.ListMap[listName].elements, setName)
	}

	if len(ipsMgr.ListMap[listName].elements) == 0 {
		if err := ipsMgr.DeleteList(listName); err != nil {
			metrics.SendErrorLogAndMetric(util.IpsmID, "Error: failed to delete ipset list %s.", listName)
			return err
		}
	}

	return nil
}

// CreateSet creates an ipset.
func (ipsMgr *IpsetManager) CreateSet(setName string, spec []string) error {
	timer := metrics.StartNewTimer()

	if _, exists := ipsMgr.SetMap[setName]; exists {
		return nil
	}

	entry := &ipsEntry{
		name:          setName,
		operationFlag: util.IpsetCreationFlag,
		// Use hashed string for set name to avoid string length limit of ipset.
		set:  util.GetHashedName(setName),
		spec: spec,
	}
	log.Logf("Creating Set: %+v", entry)
	// (TODO): need to differentiate errCode handler
	// since errCode can be one in case of "set with the same name already exists" and "maximal number of sets reached, cannot create more."
	// It may have more situations with errCode==1.
	if errCode, err := ipsMgr.Run(entry); err != nil && errCode != 1 {
		metrics.SendErrorLogAndMetric(util.IpsmID, "Error: failed to create ipset.")
		return err
	}

	ipsMgr.SetMap[setName] = NewIpset(setName)

	metrics.NumIPSets.Inc()
	timer.StopAndRecord(metrics.AddIPSetExecTime)
	metrics.SetIPSetInventory(setName, 0)

	return nil
}

// DeleteSet removes a set from ipset.
func (ipsMgr *IpsetManager) DeleteSet(setName string) error {
	if _, exists := ipsMgr.SetMap[setName]; !exists {
		metrics.SendErrorLogAndMetric(util.IpsmID, "ipset with name %s not found", setName)
		return nil
	}

	entry := &ipsEntry{
		operationFlag: util.IpsetDestroyFlag,
		set:           util.GetHashedName(setName),
	}

	if errCode, err := ipsMgr.Run(entry); err != nil {
		if errCode == 1 {
			return nil
		}

		metrics.SendErrorLogAndMetric(util.IpsmID, "Error: failed to delete ipset %s. Entry: %+v", setName, entry)
		return err
	}

	delete(ipsMgr.SetMap, setName)

	metrics.NumIPSets.Dec()
	metrics.NumIPSetEntries.Add(float64(-metrics.GetIPSetInventory(setName)))
	metrics.SetIPSetInventory(setName, 0)

	return nil
}

// AddToSet inserts an ip to an entry in setMap, and creates/updates the corresponding ipset.
func (ipsMgr *IpsetManager) AddToSet(setName, ip, spec, podKey string) error {
	if ipsMgr.Exists(setName, ip, spec) {

		// make sure we have updated the podKey in case it gets changed
		cachedPodKey := ipsMgr.SetMap[setName].elements[ip]
		if cachedPodKey != podKey {
			log.Logf("AddToSet: PodOwner has changed for Ip: %s, setName:%s, Old podKey: %s, new podKey: %s. Replace context with new PodOwner.",
				ip, setName, cachedPodKey, podKey)

			ipsMgr.SetMap[setName].elements[ip] = podKey
		}

		return nil
	}

	// possible formats
	//192.168.0.1
	//192.168.0.1,tcp:25227
	// todo: handle ip and port with protocol, plus just ip
	// always guaranteed to have ip, not guaranteed to have port + protocol
	ipDetails := strings.Split(ip, ",")
	if len(ipDetails) > 0 && ipDetails[0] == "" {
		return fmt.Errorf("Failed to add IP to set [%s], the ip to be added was empty, spec: %+v", setName, spec)
	}

	// check if the set exists, ignore the type of the set being added if it exists since the only purpose is to see if it's created or not
	exists, _ := ipsMgr.SetExists(setName)

	if !exists {
		if err := ipsMgr.CreateSet(setName, []string{spec}); err != nil {
			return err
		}
	}

	var resultSpec []string
	if strings.Contains(ip, util.IpsetNomatch) {
		ip = strings.Trim(ip, util.IpsetNomatch)
		resultSpec = []string{ip, util.IpsetNomatch}
	} else {
		resultSpec = []string{ip}
	}

	entry := &ipsEntry{
		operationFlag: util.IpsetAppendFlag,
		set:           util.GetHashedName(setName),
		spec:          resultSpec,
	}

	// todo: check err handling besides error code, corrupt state possible here
	if errCode, err := ipsMgr.Run(entry); err != nil && errCode != 1 {
		metrics.SendErrorLogAndMetric(util.IpsmID, "Error: failed to create ipset rules. %+v", entry)
		return err
	}

	// Stores the podKey as the context for this ip.
	ipsMgr.SetMap[setName].elements[ip] = podKey

	metrics.NumIPSetEntries.Inc()
	metrics.IncIPSetInventory(setName)

	return nil
}

// DeleteFromSet removes an ip from an entry in setMap, and delete/update the corresponding ipset.
func (ipsMgr *IpsetManager) DeleteFromSet(setName, ip, podKey string) error {
	ipSet, exists := ipsMgr.SetMap[setName]
	if !exists {
		log.Logf("ipset with name %s not found", setName)
		return nil
	}

	// possible formats
	//192.168.0.1
	//192.168.0.1,tcp:25227
	// todo: handle ip and port with protocol, plus just ip
	// always guaranteed to have ip, not guaranteed to have port + protocol
	ipDetails := strings.Split(ip, ",")
	if len(ipDetails) > 0 && ipDetails[0] == "" {
		return fmt.Errorf("Failed to add IP to set [%s], the ip to be added was empty", setName)
	}

	if _, exists := ipsMgr.SetMap[setName].elements[ip]; exists {
		// in case the IP belongs to a new Pod, then ignore this Delete call as this might be stale
		cachedPodKey := ipSet.elements[ip]
		if cachedPodKey != podKey {
			log.Logf("DeleteFromSet: PodOwner has changed for Ip: %s, setName:%s, Old podKey: %s, new podKey: %s. Ignore the delete as this is stale update",
				ip, setName, cachedPodKey, podKey)

			return nil
		}
	}

	// TODO optimize to not run this command in case cache has already been updated.
	entry := &ipsEntry{
		operationFlag: util.IpsetDeletionFlag,
		set:           util.GetHashedName(setName),
		spec:          []string{ip},
	}

	if errCode, err := ipsMgr.Run(entry); err != nil {
		if errCode == 1 {
			return nil
		}

		metrics.SendErrorLogAndMetric(util.IpsmID, "Error: failed to delete ipset entry: [%+v] err: [%v]", entry, err)
		return err
	}

	// Now cleanup the cache
	delete(ipsMgr.SetMap[setName].elements, ip)

	metrics.NumIPSetEntries.Dec()
	metrics.DecIPSetInventory(setName)

	if len(ipsMgr.SetMap[setName].elements) == 0 {
		ipsMgr.DeleteSet(setName)
	}

	return nil
}

// TODO this below function is to be extended while improving ipset refer count
// support, if not used, please remove this stale function.
// Clean removes all the empty sets & lists under the namespace.
func (ipsMgr *IpsetManager) Clean() error {
	for setName, set := range ipsMgr.SetMap {
		if len(set.elements) > 0 {
			continue
		}

		if err := ipsMgr.DeleteSet(setName); err != nil {
			metrics.SendErrorLogAndMetric(util.IpsmID, "Error: failed to clean ipset")
			return err
		}
	}

	for listName, list := range ipsMgr.ListMap {
		if len(list.elements) > 0 {
			continue
		}

		if err := ipsMgr.DeleteList(listName); err != nil {
			metrics.SendErrorLogAndMetric(util.IpsmID, "Error: failed to clean ipset list")
			return err
		}
	}

	return nil
}

// Destroy completely cleans ipset.
func (ipsMgr *IpsetManager) Destroy() error {
	entry := &ipsEntry{
		operationFlag: util.IpsetFlushFlag,
	}
	if _, err := ipsMgr.Run(entry); err != nil {
		metrics.SendErrorLogAndMetric(util.IpsmID, "Error: failed to flush ipset")
		return err
	}

	entry.operationFlag = util.IpsetDestroyFlag
	if _, err := ipsMgr.Run(entry); err != nil {
		metrics.SendErrorLogAndMetric(util.IpsmID, "Error: failed to destroy ipset")
		return err
	}

	//TODO set IPSetInventory to 0 for all set names

	return nil
}

// Run execute an ipset command to update ipset.
func (ipsMgr *IpsetManager) Run(entry *ipsEntry) (int, error) {
	cmdName := util.Ipset
	cmdArgs := append([]string{entry.operationFlag, util.IpsetExistFlag, entry.set}, entry.spec...)
	cmdArgs = util.DropEmptyFields(cmdArgs)

	log.Logf("Executing ipset command %s %v", cmdName, cmdArgs)

	cmd := ipsMgr.exec.Command(cmdName, cmdArgs...)
	output, err := cmd.CombinedOutput()

	if result, isExitError := err.(utilexec.ExitError); isExitError {
		exitCode := result.ExitStatus()
		errfmt := fmt.Errorf("Error: There was an error running command: [%s %v] Stderr: [%v, %s]", cmdName, strings.Join(cmdArgs, " "), err, strings.TrimSuffix(string(output), "\n"))
		if exitCode > 0 {
			metrics.SendErrorLogAndMetric(util.IpsmID, errfmt.Error())
		}

		return exitCode, errfmt
	}

	return 0, nil
}

// Save saves ipset to file.
func (ipsMgr *IpsetManager) Save(configFile string) error {
	if len(configFile) == 0 {
		configFile = util.IpsetConfigFile
	}

	cmd := ipsMgr.exec.Command(util.Ipset, util.IpsetSaveFlag, util.IpsetFileFlag, configFile)
	output, err := cmd.CombinedOutput()
	if err != nil {
		metrics.SendErrorLogAndMetric(util.IpsmID, "Error: failed to save ipset: [%s] Stderr: [%v, %s]", cmd, err, strings.TrimSuffix(string(output), "\n"))
		return err
	}
	cmd.Wait()

	return nil
}

// Restore restores ipset from file.
func (ipsMgr *IpsetManager) Restore(configFile string) error {
	if len(configFile) == 0 {
		configFile = util.IpsetConfigFile
	}

	f, err := os.Stat(configFile)
	if err != nil {
		metrics.SendErrorLogAndMetric(util.IpsmID, "Error: failed to get file %s stat from ipsm.Restore", configFile)
		return err
	}

	if f.Size() == 0 {
		if err := ipsMgr.Destroy(); err != nil {
			return err
		}
	}

	cmd := ipsMgr.exec.Command(util.Ipset, util.IpsetRestoreFlag, util.IpsetFileFlag, configFile)
	output, err := cmd.CombinedOutput()
	if err != nil {
		metrics.SendErrorLogAndMetric(util.IpsmID, "Error: failed to to restore ipset from file: [%s] Stderr: [%v, %s]", cmd, err, strings.TrimSuffix(string(output), "\n"))
		return err
	}

	//TODO based on the set name and number of entries in the config file, update IPSetInventory

	return nil
}

// DestroyNpmIpsets destroys only ipsets created by NPM
func (ipsMgr *IpsetManager) DestroyNpmIpsets() error {
	cmdName := util.Ipset
	cmdArgs := util.IPsetCheckListFlag

	reply, err := ipsMgr.exec.Command(cmdName, cmdArgs).CombinedOutput()
	if msg, failed := err.(*exec.ExitError); failed {
		errCode := msg.Sys().(syscall.WaitStatus).ExitStatus()
		if errCode > 0 {
			metrics.SendErrorLogAndMetric(util.IpsmID, "{DestroyNpmIpsets} Error: There was an error running command: [%s] Stderr: [%v, %s]", cmdName, err, strings.TrimSuffix(string(msg.Stderr), "\n"))
		}

		return err
	}
	if reply == nil {
		metrics.SendErrorLogAndMetric(util.IpsmID, "{DestroyNpmIpsets} Received empty string from ipset list while destroying azure-npm ipsets")
		return nil
	}

	re := regexp.MustCompile("Name: (" + util.AzureNpmPrefix + "\\d+)")
	ipsetRegexSlice := re.FindAllSubmatch(reply, -1)

	if len(ipsetRegexSlice) == 0 {
		log.Logf("No Azure-NPM IPsets are found in the Node.")
		return nil
	}

	ipsetLists := make([]string, 0)
	for _, matchedItem := range ipsetRegexSlice {
		if len(matchedItem) == 2 {
			itemString := string(matchedItem[1])
			if strings.Contains(itemString, util.AzureNpmFlag) {
				ipsetLists = append(ipsetLists, itemString)
			}
		}
	}

	if len(ipsetLists) == 0 {
		return nil
	}

	entry := &ipsEntry{
		operationFlag: util.IpsetFlushFlag,
	}

	for _, ipsetName := range ipsetLists {
		entry := &ipsEntry{
			operationFlag: util.IpsetFlushFlag,
			set:           ipsetName,
		}

		if _, err := ipsMgr.Run(entry); err != nil {
			metrics.SendErrorLogAndMetric(util.IpsmID, "{DestroyNpmIpsets} Error: failed to flush ipset %s", ipsetName)
		}
	}

	for _, ipsetName := range ipsetLists {
		entry.operationFlag = util.IpsetDestroyFlag
		entry.set = ipsetName
		if _, err := ipsMgr.Run(entry); err != nil {
			metrics.SendErrorLogAndMetric(util.IpsmID, "{DestroyNpmIpsets} Error: failed to destroy ipset %s", ipsetName)
		}
	}

	return nil
}
