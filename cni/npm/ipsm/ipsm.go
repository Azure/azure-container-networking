package ipsm

import (
	"fmt"
	"os"
	"os/exec"
	"strings"

	"github.com/Azure/azure-container-networking/cni/npm/util"
)

type ipsEntry struct {
	operationFlag string
	name          string
	set           string
	spec          string
}

// IpsetManager stores ipset entries.
type IpsetManager struct {
	listMap  map[string][]string //tracks all set lists.
	entryMap map[string]*ipsEntry
	setMap   map[string][]string //label -> []ip
}

// NewIpsetManager creates a new instance for IpsetManager object.
func NewIpsetManager() *IpsetManager {
	ipsMgr := &IpsetManager{
		listMap:  make(map[string][]string),
		entryMap: make(map[string]*ipsEntry),
		setMap:   make(map[string][]string),
	}

	return ipsMgr
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

	for _, elem := range m[key] {
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
	// Ignore system pods.
	if listName == util.KubeSystemFlag {
		return nil
	}

	hashedName := util.AzureNpmPrefix + util.Hash(listName)
	if _, exists := ipsMgr.listMap[listName]; exists {
		return nil
	}

	ipsMgr.entryMap[listName] = &ipsEntry{
		operationFlag: util.IpsetCreationFlag,
		set:           hashedName,
		spec:          util.IpsetSetListFlag,
	}
	if err := ipsMgr.Run(ipsMgr.entryMap[listName]); err != nil {
		fmt.Printf("Error creating ipset list %s.\n", listName)
		return err
	}

	ipsMgr.listMap[listName] = []string{}

	return nil
}

// AddToList inserts an ipset to an ipset list.
func (ipsMgr *IpsetManager) AddToList(listName string, setName string) error {
	if ipsMgr.Exists(listName, setName, util.IpsetSetListFlag) {
		return nil
	}

	if err := ipsMgr.CreateList(listName); err != nil {
		return err
	}

	ipsMgr.entryMap[listName].operationFlag = util.IpsetAppendFlag
	ipsMgr.entryMap[listName].spec = util.AzureNpmPrefix + util.Hash(setName)

	if err := ipsMgr.Run(ipsMgr.entryMap[listName]); err != nil {
		fmt.Printf("Error creating ipset rules.\n")
		fmt.Printf("rule: %+v\n", ipsMgr.entryMap[listName])
		return err
	}

	ipsMgr.listMap[listName] = append(ipsMgr.listMap[listName], setName)

	return nil
}

// DeleteFromList removes an ipset to an ipset list.
func (ipsMgr *IpsetManager) DeleteFromList(listName string, setName string) error {
	if _, exists := ipsMgr.listMap[listName]; !exists {
		return fmt.Errorf("ipset list with name %s not found", listName)
	}

	for i, val := range ipsMgr.listMap[listName] {
		if val == setName {
			ipsMgr.listMap[listName] = append(ipsMgr.listMap[listName][:i], ipsMgr.listMap[listName][i+1:]...)
		}
	}

	hashedListName, hashedSetName := util.AzureNpmPrefix+util.Hash(listName), util.AzureNpmPrefix+util.Hash(setName)
	entry := &ipsEntry{
		operationFlag: util.IpsetDeletionFlag,
		set:           hashedListName,
		spec:          hashedSetName,
	}
	if err := ipsMgr.Run(entry); err != nil {
		fmt.Printf("Error deleting ipset entry.\n")
		fmt.Printf("%+v\n", entry)
		return err
	}

	if len(ipsMgr.listMap[listName]) == 0 {
		if err := ipsMgr.DeleteList(listName); err != nil {
			fmt.Printf("Error deleting ipset list %s.\n", listName)
			return err
		}
	}

	return nil
}

// DeleteList removes an ipset list.
func (ipsMgr *IpsetManager) DeleteList(listName string) error {
	hashedName := util.AzureNpmPrefix + util.Hash(listName)
	entry := &ipsEntry{
		operationFlag: util.IpsetDestroyFlag,
		set:           hashedName,
	}

	if err := ipsMgr.Run(entry); err != nil {
		fmt.Printf("Error deleting ipset %s", listName)
		fmt.Printf("%+v\n", entry)
		return err
	}

	delete(ipsMgr.listMap, listName)

	return nil
}

// CreateSet creates an ipset.
func (ipsMgr *IpsetManager) CreateSet(setName string) error {
	// Use hashed string for set name to avoid string length limit of ipset.
	hashedName := util.AzureNpmPrefix + util.Hash(setName)
	if _, exists := ipsMgr.setMap[setName]; exists {
		return nil
	}

	ipsMgr.entryMap[setName] = &ipsEntry{
		operationFlag: util.IpsetCreationFlag,
		set:           hashedName,
		spec:          util.IpsetNetHashFlag,
	}
	if err := ipsMgr.Run(ipsMgr.entryMap[setName]); err != nil {
		fmt.Printf("Error creating ipset.\n")
		return err
	}

	ipsMgr.setMap[setName] = []string{}

	return nil
}

// AddToSet inserts an ip to an entry in setMap, and creates/updates the corresponding ipset.
func (ipsMgr *IpsetManager) AddToSet(setName string, ip string) error {
	if ipsMgr.Exists(setName, ip, util.IpsetNetHashFlag) {
		return nil
	}

	if err := ipsMgr.CreateSet(setName); err != nil {
		return err
	}

	ipsMgr.entryMap[setName].operationFlag = util.IpsetAppendFlag
	ipsMgr.entryMap[setName].spec = ip

	if err := ipsMgr.Run(ipsMgr.entryMap[setName]); err != nil {
		fmt.Printf("Error creating ipset rules.\n")
		fmt.Printf("rule: %+v\n", ipsMgr.entryMap[setName])
		return err
	}
	ipsMgr.setMap[setName] = append(ipsMgr.setMap[setName], ip)

	return nil
}

// DeleteFromSet removes an ip from an entry in setMap, and delete/update the corresponding ipset.
func (ipsMgr *IpsetManager) DeleteFromSet(setName string, ip string) error {
	if _, exists := ipsMgr.setMap[setName]; !exists {
		return fmt.Errorf("ipset with name %s not found", setName)
	}

	for i, val := range ipsMgr.setMap[setName] {
		if val == ip {
			ipsMgr.setMap[setName] = append(ipsMgr.setMap[setName][:i], ipsMgr.setMap[setName][i+1:]...)
		}
	}

	hashedName := util.AzureNpmPrefix + util.Hash(setName)
	entry := &ipsEntry{
		operationFlag: util.IpsetDeletionFlag,
		set:           hashedName,
		spec:          ip,
	}
	if err := ipsMgr.Run(entry); err != nil {
		fmt.Printf("Error deleting ipset entry.\n")
		fmt.Printf("%+v\n", entry)
		return err
	}

	return nil
}

// DeleteSet removes a set from ipset.
func (ipsMgr *IpsetManager) DeleteSet(setName string) error {
	if _, exists := ipsMgr.setMap[setName]; !exists {
		return fmt.Errorf("ipset with name %s not found", setName)
	}

	if len(ipsMgr.setMap[setName]) > 0 {
		return nil
	}

	hashedName := util.AzureNpmPrefix + util.Hash(setName)
	entry := &ipsEntry{
		operationFlag: util.IpsetDestroyFlag,
		set:           hashedName,
	}
	if err := ipsMgr.Run(entry); err != nil {
		fmt.Printf("Error deleting ipset %s", setName)
		fmt.Printf("%+v\n", entry)
		return err
	}

	delete(ipsMgr.setMap, setName)

	return nil
}

// Clean removes all the empty sets under the namespace.
func (ipsMgr *IpsetManager) Clean() error {
	for setName := range ipsMgr.setMap {
		if isNsSet(setName) {
			continue
		}

		if err := ipsMgr.DeleteSet(setName); err != nil {
			fmt.Printf("Error cleaning ipset\n")
			return err
		}
	}

	return nil
}

// Run execute an ipset command to update ipset.
func (ipsMgr *IpsetManager) Run(entry *ipsEntry) error {
	cmdName := util.Ipset
	cmdArgs := []string{entry.operationFlag}
	if len(entry.set) > 0 {
		cmdArgs = append(cmdArgs, entry.set)
	}
	if len(entry.spec) > 0 {
		cmdArgs = append(cmdArgs, entry.spec)
	}

	var (
		cmdOut []byte
		err    error
	)
	if cmdOut, err = exec.Command(cmdName, cmdArgs...).Output(); err != nil {
		fmt.Println(os.Stderr, "There was an error running command: ", err)
		fmt.Printf("%s %+v\n", string(cmdOut), cmdArgs)
		return err
	}

	fmt.Printf("%s %+v\n", string(cmdOut), cmdArgs)

	return nil
}
