// Copyright 2018 Microsoft. All rights reserved.
// MIT License
package ipsm

import (
	"os"
	"os/exec"
	"strings"
	"syscall"

	"github.com/Azure/azure-container-networking/log"
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
	elements   []string
	referCount int
}

// NewIpset creates a new instance for Ipset object.
func NewIpset(setName string) *Ipset {
	return &Ipset{
		name: setName,
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

	for _, elem := range m[key].elements {
		if elem == val {
			return true
		}
	}

	return false
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
		log.Errorf("Error: failed to create ipset list %s.", listName)
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

		log.Errorf("Error: failed to delete ipset %s %+v", listName, entry)
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
		log.Errorf("Error: failed to create ipset rules. rule: %+v", entry)
		return err
	}

	ipsMgr.listMap[listName].elements = append(ipsMgr.listMap[listName].elements, setName)

	return nil
}

// DeleteFromList removes an ipset to an ipset list.
func (ipsMgr *IpsetManager) DeleteFromList(listName string, setName string) error {
	if _, exists := ipsMgr.listMap[listName]; !exists {
		log.Logf("ipset list with name %s not found", listName)
		return nil
	}

	for i, val := range ipsMgr.listMap[listName].elements {
		if val == setName {
			ipsMgr.listMap[listName].elements = append(ipsMgr.listMap[listName].elements[:i], ipsMgr.listMap[listName].elements[i+1:]...)
		}
	}

	hashedListName, hashedSetName := util.GetHashedName(listName), util.GetHashedName(setName)
	entry := &ipsEntry{
		operationFlag: util.IpsetDeletionFlag,
		set:           hashedListName,
		spec:          []string{hashedSetName},
	}

	if _, err := ipsMgr.Run(entry); err != nil {
		log.Errorf("Error: failed to delete ipset entry. %+v", entry)
		return err
	}

	if len(ipsMgr.listMap[listName].elements) == 0 {
		if err := ipsMgr.DeleteList(listName); err != nil {
			log.Errorf("Error: failed to delete ipset list %s.", listName)
			return err
		}
	}

	return nil
}

// CreateSet creates an ipset.
func (ipsMgr *IpsetManager) CreateSet(setName string, spec []string) error {
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
		log.Errorf("Error: failed to create ipset.")
		return err
	}

	ipsMgr.setMap[setName] = NewIpset(setName)

	return nil
}

// DeleteSet removes a set from ipset.
func (ipsMgr *IpsetManager) DeleteSet(setName string) error {
	if _, exists := ipsMgr.setMap[setName]; !exists {
		log.Logf("ipset with name %s not found", setName)
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

		log.Errorf("Error: failed to delete ipset %s. Entry: %+v", setName, entry)
		return err
	}

	delete(ipsMgr.setMap, setName)

	return nil
}

// AddToSet inserts an ip to an entry in setMap, and creates/updates the corresponding ipset.
func (ipsMgr *IpsetManager) AddToSet(setName, ip, spec string) error {
	if ipsMgr.Exists(setName, ip, spec) {
		return nil
	}

	if err := ipsMgr.CreateSet(setName, append([]string{util.IpsetNetHashFlag})); err != nil {
		return err
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
		log.Logf("Error: failed to create ipset rules. %+v", entry)
		return err
	}

	ipsMgr.setMap[setName].elements = append(ipsMgr.setMap[setName].elements, ip)

	return nil
}

// DeleteFromSet removes an ip from an entry in setMap, and delete/update the corresponding ipset.
func (ipsMgr *IpsetManager) DeleteFromSet(setName, ip string) error {
	if _, exists := ipsMgr.setMap[setName]; !exists {
		log.Logf("ipset with name %s not found", setName)
		return nil
	}

	for i, val := range ipsMgr.setMap[setName].elements {
		if val == ip {
			ipsMgr.setMap[setName].elements = append(ipsMgr.setMap[setName].elements[:i], ipsMgr.setMap[setName].elements[i+1:]...)
		}
	}

	entry := &ipsEntry{
		operationFlag: util.IpsetDeletionFlag,
		set:           util.GetHashedName(setName),
		spec:          append([]string{ip}),
	}

	if errCode, err := ipsMgr.Run(entry); err != nil {
		if errCode == 1 {
			return nil
		}

		log.Errorf("Error: failed to delete ipset entry. Entry: %+v", entry)
		return err
	}

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
			log.Errorf("Error: failed to clean ipset")
			return err
		}
	}

	for listName, list := range ipsMgr.listMap {
		if len(list.elements) > 0 {
			continue
		}

		if err := ipsMgr.DeleteList(listName); err != nil {
			log.Errorf("Error: failed to clean ipset list")
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
		log.Errorf("Error: failed to flush ipset")
		return err
	}

	entry.operationFlag = util.IpsetDestroyFlag
	if _, err := ipsMgr.Run(entry); err != nil {
		log.Errorf("Error: failed to destroy ipset")
		return err
	}

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
			log.Errorf("Error: There was an error running command: [%s %v] Stderr: [%v, %s]", cmdName, strings.Join(cmdArgs, " "), err, strings.TrimSuffix(string(msg.Stderr), "\n"))
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
		log.Errorf("Error: failed to save ipset to file.")
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
		log.Errorf("Error: failed to get file %s stat from ipsm.Restore", configFile)
		return err
	}

	if f.Size() == 0 {
		if err := ipsMgr.Destroy(); err != nil {
			return err
		}
	}

	cmd := exec.Command(util.Ipset, util.IpsetRestoreFlag, util.IpsetFileFlag, configFile)
	if err := cmd.Start(); err != nil {
		log.Errorf("Error: failed to restore ipset from file.")
		return err
	}
	cmd.Wait()

	return nil
}
