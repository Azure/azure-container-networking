// Package ipsm focus on ip set operation
// Copyright 2018 Microsoft. All rights reserved.
// MIT License
package ipsm

import (
	"os"
	"os/exec"
	"strings"
	"syscall"

	"github.com/Azure/azure-container-networking/log"
	"github.com/Azure/azure-container-networking/npm/metrics"
	"github.com/Azure/azure-container-networking/npm/util"
)

type ipsEntry struct {
	operationFlag string
	name          string
	set           string
	spec          []string
}

// IpsetManager stores ipset states.
type IpsetManager struct {
	listMap map[string]*Ipset //tracks all set lists.
	setMap  map[string]*Ipset //label -> []ip
}

// Ipset represents one ipset entry.
type Ipset struct {
	name       string
	elements   map[string]string // key = ip, value: context associated to the ip like podUid
	referCount int
}

// NewIpset creates a new instance for Ipset object.
func NewIpset(setName string) *Ipset {
	return &Ipset{
		name:     setName,
		elements: make(map[string]string),
	}
}

// NewIpsetManager creates a new instance for IpsetManager object.
func NewIpsetManager() *IpsetManager {
	return &IpsetManager{
		listMap: make(map[string]*Ipset),
		setMap:  make(map[string]*Ipset),
	}
}

// Exists checks if an element exists in setMap/listMap.
func (ipsMgr *IpsetManager) Exists(key string, val string, kind string) bool {
	m := ipsMgr.setMap
	if kind == util.IpsetSetListFlag {
		m = ipsMgr.listMap
	}

	if _, exists := m[key]; !exists {
		return false
	}

	if _, exists := m[key].elements[val]; !exists {
		return false
	}

	return true
}

// SetExists checks whehter an ipset exists.
func (ipsMgr *IpsetManager) SetExists(setName, kind string) bool {
    m := ipsMgr.setMap
    if kind == util.IpsetSetListFlag {
        m = ipsMgr.listMap
    }
    _, exists := m[setName]
    return exists
}

func isNsSet(setName string) bool {
	return !strings.Contains(setName, "-") && !strings.Contains(setName, ":")
}

// CreateList creates an ipset list. npm maintains one setlist per namespace label.
func (ipsMgr *IpsetManager) CreateList(listName string) error {
	if _, exists := ipsMgr.listMap[listName]; exists {
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
		metrics.SendErrorMetric(util.IpsmID, "Error: failed to create ipset list %s.", listName)
		return err
	}

	ipsMgr.listMap[listName] = NewIpset(listName)

	return nil
}

// DeleteList removes an ipset list.
func (ipsMgr *IpsetManager) DeleteList(listName string) error {
	entry := &ipsEntry{
		operationFlag: util.IpsetDestroyFlag,
		set:           util.GetHashedName(listName),
	}

	if errCode, err := ipsMgr.Run(entry); err != nil {
		if errCode == 1 {
			return nil
		}

		metrics.SendErrorMetric(util.IpsmID, "Error: failed to delete ipset %s %+v", listName, entry)
		return err
	}

	delete(ipsMgr.listMap, listName)

	return nil
}

// AddToList inserts an ipset to an ipset list.
func (ipsMgr *IpsetManager) AddToList(listName string, setName string) error {
	if listName == setName {
		return nil
	}

	if ipsMgr.Exists(listName, setName, util.IpsetSetListFlag) {
		return nil
	}

	if err := ipsMgr.CreateList(listName); err != nil {
		return err
	}

	entry := &ipsEntry{
		operationFlag: util.IpsetAppendFlag,
		set:           util.GetHashedName(listName),
		spec:          []string{util.GetHashedName(setName)},
	}

	if errCode, err := ipsMgr.Run(entry); err != nil && errCode != 1 {
		metrics.SendErrorMetric(util.IpsmID, "Error: failed to create ipset rules. rule: %+v", entry)
		return err
	}

	ipsMgr.listMap[listName].elements[setName] = ""

	return nil
}

// DeleteFromList removes an ipset to an ipset list.
func (ipsMgr *IpsetManager) DeleteFromList(listName string, setName string) error {
	if _, exists := ipsMgr.listMap[listName]; !exists {
		metrics.SendErrorMetric(util.IpsmID, "ipset list with name %s not found", listName)
		return nil
	}

	hashedListName, hashedSetName := util.GetHashedName(listName), util.GetHashedName(setName)
	entry := &ipsEntry{
		operationFlag: util.IpsetDeletionFlag,
		set:           hashedListName,
		spec:          []string{hashedSetName},
	}

	if _, err := ipsMgr.Run(entry); err != nil {
		metrics.SendErrorMetric(util.IpsmID, "Error: failed to delete ipset entry. %+v", entry)
		return err
	}

	// Now cleanup the cache
	if _, exists := ipsMgr.listMap[listName].elements[setName]; exists {
		delete(ipsMgr.listMap[listName].elements, setName)
	}

	if len(ipsMgr.listMap[listName].elements) == 0 {
		if err := ipsMgr.DeleteList(listName); err != nil {
			metrics.SendErrorMetric(util.IpsmID, "Error: failed to delete ipset list %s.", listName)
			return err
		}
	}

	return nil
}

// CreateSet creates an ipset.
func (ipsMgr *IpsetManager) CreateSet(setName string, spec []string) error {
	timer := metrics.StartNewTimer()

	if _, exists := ipsMgr.setMap[setName]; exists {
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
	if errCode, err := ipsMgr.Run(entry); err != nil && errCode != 1 {
		metrics.SendErrorMetric(util.IpsmID, "Error: failed to create ipset.")
		return err
	}

	ipsMgr.setMap[setName] = NewIpset(setName)

	metrics.NumIPSets.Inc()
	timer.StopAndRecord(metrics.AddIPSetExecTime)
	metrics.SetIPSetInventory(setName, 0)

	return nil
}

// DeleteSet removes a set from ipset.
func (ipsMgr *IpsetManager) DeleteSet(setName string) error {
	if _, exists := ipsMgr.setMap[setName]; !exists {
		metrics.SendErrorMetric(util.IpsmID, "ipset with name %s not found", setName)
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

		metrics.SendErrorMetric(util.IpsmID, "Error: failed to delete ipset %s. Entry: %+v", setName, entry)
		return err
	}

	delete(ipsMgr.setMap, setName)

	metrics.NumIPSets.Dec()
	metrics.NumIPSetEntries.Add(float64(-metrics.GetIPSetInventory(setName)))
	metrics.SetIPSetInventory(setName, 0)

	return nil
}

// AddToSet inserts an ip to an entry in setMap, and creates/updates the corresponding ipset.
func (ipsMgr *IpsetManager) AddToSet(setName, ip, spec, podUid string) error {
	if ipsMgr.Exists(setName, ip, spec) {

		// make sure we have updated the podUid in case it gets changed
		cachedPodUid := ipsMgr.setMap[setName].elements[ip]
		if cachedPodUid != podUid {
			log.Logf("AddToSet: PodOwner has changed for Ip: %s, setName:%s, Old podUid: %s, new PodUid: %s. Replace context with new PodOwner.",
				ip, setName, cachedPodUid, podUid)

			ipsMgr.setMap[setName].elements[ip] = podUid
		}

		return nil
	}

	if !ipsMgr.SetExists(setName, spec) {
		if err := ipsMgr.CreateSet(setName, append([]string{spec})); err != nil {
			return err
		}
	}
	var resultSpec []string
	if strings.Contains(ip, util.IpsetNomatch) {
		ip = strings.Trim(ip, util.IpsetNomatch)
		resultSpec = append([]string{ip, util.IpsetNomatch})
	} else {
		resultSpec = append([]string{ip})
	}

	entry := &ipsEntry{
		operationFlag: util.IpsetAppendFlag,
		set:           util.GetHashedName(setName),
		spec:          resultSpec,
	}

	if errCode, err := ipsMgr.Run(entry); err != nil && errCode != 1 {
		metrics.SendErrorMetric(util.IpsmID, "Error: failed to create ipset rules. %+v", entry)
		return err
	}

	// Stores the podUid as the context for this ip.
	ipsMgr.setMap[setName].elements[ip] = podUid

	metrics.NumIPSetEntries.Inc()
	metrics.IncIPSetInventory(setName)

	return nil
}

// DeleteFromSet removes an ip from an entry in setMap, and delete/update the corresponding ipset.
func (ipsMgr *IpsetManager) DeleteFromSet(setName, ip, podUid string) error {
	ipSet, exists := ipsMgr.setMap[setName]
	if !exists {
		log.Logf("ipset with name %s not found", setName)
		return nil
	}

	if _, exists := ipsMgr.setMap[setName].elements[ip]; exists {
		// in case the IP belongs to a new Pod, then ignore this Delete call as this might be stale
		cachedPodUid := ipSet.elements[ip]
		if cachedPodUid != podUid {
			log.Logf("DeleteFromSet: PodOwner has changed for Ip: %s, setName:%s, Old podUid: %s, new PodUid: %s. Ignore the delete as this is stale update",
				ip, setName, cachedPodUid, podUid)

			return nil
		}
	}

	// TODO optimize to not run this command in case cache has already been updated.
	entry := &ipsEntry{
		operationFlag: util.IpsetDeletionFlag,
		set:           util.GetHashedName(setName),
		spec:          append([]string{ip}),
	}

	if errCode, err := ipsMgr.Run(entry); err != nil {
		if errCode == 1 {
			return nil
		}

		metrics.SendErrorMetric(util.IpsmID, "Error: failed to delete ipset entry. Entry: %+v", entry)
		return err
	}

	// Now cleanup the cache
	delete(ipsMgr.setMap[setName].elements, ip)

	metrics.NumIPSetEntries.Dec()
	metrics.DecIPSetInventory(setName)

	if len(ipsMgr.setMap[setName].elements) == 0 {
		ipsMgr.DeleteSet(setName)
	}

	return nil
}

// Clean removes all the empty sets & lists under the namespace.
func (ipsMgr *IpsetManager) Clean() error {
	for setName, set := range ipsMgr.setMap {
		if len(set.elements) > 0 {
			continue
		}

		if err := ipsMgr.DeleteSet(setName); err != nil {
			metrics.SendErrorMetric(util.IpsmID, "Error: failed to clean ipset")
			return err
		}
	}

	for listName, list := range ipsMgr.listMap {
		if len(list.elements) > 0 {
			continue
		}

		if err := ipsMgr.DeleteList(listName); err != nil {
			metrics.SendErrorMetric(util.IpsmID, "Error: failed to clean ipset list")
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
		metrics.SendErrorMetric(util.IpsmID, "Error: failed to flush ipset")
		return err
	}

	entry.operationFlag = util.IpsetDestroyFlag
	if _, err := ipsMgr.Run(entry); err != nil {
		metrics.SendErrorMetric(util.IpsmID, "Error: failed to destroy ipset")
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
	_, err := exec.Command(cmdName, cmdArgs...).Output()
	if msg, failed := err.(*exec.ExitError); failed {
		errCode := msg.Sys().(syscall.WaitStatus).ExitStatus()
		if errCode > 0 {
			metrics.SendErrorMetric(util.IpsmID, "Error: There was an error running command: [%s %v] Stderr: [%v, %s]", cmdName, strings.Join(cmdArgs, " "), err, strings.TrimSuffix(string(msg.Stderr), "\n"))
		}

		return errCode, err
	}

	return 0, nil
}

// Save saves ipset to file.
func (ipsMgr *IpsetManager) Save(configFile string) error {
	if len(configFile) == 0 {
		configFile = util.IpsetConfigFile
	}

	cmd := exec.Command(util.Ipset, util.IpsetSaveFlag, util.IpsetFileFlag, configFile)
	if err := cmd.Start(); err != nil {
		metrics.SendErrorMetric(util.IpsmID, "Error: failed to save ipset to file.")
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
		metrics.SendErrorMetric(util.IpsmID, "Error: failed to get file %s stat from ipsm.Restore", configFile)
		return err
	}

	if f.Size() == 0 {
		if err := ipsMgr.Destroy(); err != nil {
			return err
		}
	}

	cmd := exec.Command(util.Ipset, util.IpsetRestoreFlag, util.IpsetFileFlag, configFile)
	if err := cmd.Start(); err != nil {
		metrics.SendErrorMetric(util.IpsmID, "Error: failed to to restore ipset from file.")
		return err
	}
	cmd.Wait()

	//TODO based on the set name and number of entries in the config file, update IPSetInventory

	return nil
}