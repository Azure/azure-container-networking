package ipsets

import (
	"fmt"
	"sync"

	"github.com/Azure/azure-container-networking/common"
	"github.com/Azure/azure-container-networking/npm/metrics"
	npmerrors "github.com/Azure/azure-container-networking/npm/util/errors"
	"k8s.io/klog"
)

type IPSetMode string

const (
	// ApplyAllIPSets will change dataplane behavior to apply all ipsets
	ApplyAllIPSets IPSetMode = "all"
	// ApplyOnNeed will change dataplane behavior to apply
	// only ipsets that are referenced by network policies
	ApplyOnNeed IPSetMode = "on-need"
)

type IPSetManager struct {
	iMgrCfg *IPSetManagerCfg
	setMap  map[string]*IPSet
	// Map with Key as IPSet name to to emulate set
	// and value as struct{} for minimal memory consumption.
	toAddOrUpdateCache map[string]struct{}
	// IPSets referred to in this cache may be in the setMap, but must be deleted from the kernel
	toDeleteCache map[string]struct{}
	ioShim        *common.IOShim
	sync.Mutex
}

type IPSetManagerCfg struct {
	IPSetMode   IPSetMode
	NetworkName string
}

func NewIPSetManager(iMgrCfg *IPSetManagerCfg, ioShim *common.IOShim) *IPSetManager {
	return &IPSetManager{
		iMgrCfg:            iMgrCfg,
		setMap:             make(map[string]*IPSet),
		toAddOrUpdateCache: make(map[string]struct{}),
		toDeleteCache:      make(map[string]struct{}),
		ioShim:             ioShim,
	}
}

func (iMgr *IPSetManager) ResetIPSets() error {
	iMgr.Lock()
	defer iMgr.Unlock()
	err := iMgr.resetIPSets()
	if err != nil {
		return fmt.Errorf("error while resetting ipsetmanager: %w", err)
	}
	// TODO update prometheus metrics here instead of in OS-specific functions (done in Linux right now)
	// metrics.ResetNumIPSets() and metrics.ResetIPSetEntries()
	return nil
}

func (iMgr *IPSetManager) CreateIPSets(setMetadatas []*IPSetMetadata) {
	iMgr.Lock()
	defer iMgr.Unlock()

	for _, set := range setMetadatas {
		iMgr.createIPSet(set)
	}
}

func (iMgr *IPSetManager) createIPSet(setMetadata *IPSetMetadata) {
	// TODO (vamsi) check for os specific restrictions on ipsets
	prefixedName := setMetadata.GetPrefixName()
	if iMgr.exists(prefixedName) {
		return
	}
	iMgr.setMap[prefixedName] = NewIPSet(setMetadata)
	metrics.IncNumIPSets()
	if iMgr.iMgrCfg.IPSetMode == ApplyAllIPSets {
		iMgr.modifyCacheForKernelCreation(prefixedName)
	}
}

// DeleteIPSet expects the prefixed ipset name
func (iMgr *IPSetManager) DeleteIPSet(name string) {
	iMgr.Lock()
	defer iMgr.Unlock()
	if !iMgr.exists(name) {
		return
	}

	set := iMgr.setMap[name]
	if !set.canBeDeleted() {
		return
	}

	delete(iMgr.setMap, name)
	metrics.DecNumIPSets()
	if iMgr.iMgrCfg.IPSetMode == ApplyAllIPSets {
		// NOTE in ApplyAllIPSets mode, if this ipset has never been created in the kernel, it would be added to the deleteCache, and then the OS would fail to delete it
		iMgr.modifyCacheForKernelRemoval(name)
	}
	// if mode is ApplyOnNeed, the set will not be in the kernel (or will be in the delete cache already) since there are no references
}

// GetIPSet needs the prefixed ipset name
func (iMgr *IPSetManager) GetIPSet(name string) *IPSet {
	iMgr.Lock()
	defer iMgr.Unlock()
	if !iMgr.exists(name) {
		return nil
	}
	return iMgr.setMap[name]
}

// AddReference takes in the prefixed setname and adds relevant reference
func (iMgr *IPSetManager) AddReference(setName, referenceName string, referenceType ReferenceType) error {
	iMgr.Lock()
	defer iMgr.Unlock()
	if !iMgr.exists(setName) {
		npmErrorString := npmerrors.AddSelectorReference
		if referenceType == NetPolType {
			npmErrorString = npmerrors.AddNetPolReference
		}
		return npmerrors.Errorf(npmErrorString, false, fmt.Sprintf("ipset %s does not exist", setName))
	}

	set := iMgr.setMap[setName]
	if referenceType == SelectorType && !set.canSetBeSelectorIPSet() {
		return npmerrors.Errorf(npmerrors.AddSelectorReference, false, fmt.Sprintf("ipset %s is not a selector ipset it is of type %s", setName, set.Type.String()))
	}
	wasInKernel := iMgr.shouldBeInKernel(set)
	set.addReference(referenceName, referenceType)
	if !wasInKernel {
		// the set should be in the kernel, so add it to the kernel if it wasn't beforehand
		iMgr.modifyCacheForKernelCreation(setName)

		// if set.Kind == HashSet, then this for loop will do nothing
		for _, member := range set.MemberIPSets {
			iMgr.incKernelReferCountAndModifyCache(member)
		}
	}
	return nil
}

// DeleteReference takes in the prefixed setname and removes relevant reference
func (iMgr *IPSetManager) DeleteReference(setName, referenceName string, referenceType ReferenceType) error {
	iMgr.Lock()
	defer iMgr.Unlock()
	if !iMgr.exists(setName) {
		npmErrorString := npmerrors.DeleteSelectorReference
		if referenceType == NetPolType {
			npmErrorString = npmerrors.DeleteNetPolReference
		}
		return npmerrors.Errorf(npmErrorString, false, fmt.Sprintf("ipset %s does not exist", setName))
	}

	set := iMgr.setMap[setName]
	wasInKernel := iMgr.shouldBeInKernel(set) // required because the set may not be in the kernel if this reference doesn't exist
	set.deleteReference(referenceName, referenceType)
	if wasInKernel && !iMgr.shouldBeInKernel(set) {
		// remove from kernel if it was in the kernel before and shouldn't be now
		iMgr.modifyCacheForKernelRemoval(set.Name)

		// if set.Kind == HashSet, then this for loop will do nothing
		for _, member := range set.MemberIPSets {
			iMgr.decKernelReferCountAndModifyCache(member)
		}
	}
	return nil
}

func (iMgr *IPSetManager) AddToSets(addToSets []*IPSetMetadata, ip, podKey string) error {
	if len(addToSets) == 0 {
		return nil
	}
	// TODO check if the IP is IPV4 family in controller
	iMgr.Lock()
	defer iMgr.Unlock()

	for _, metadata := range addToSets {
		// 1. check for errors and create a missing set
		prefixedName := metadata.GetPrefixName()
		set, exists := iMgr.setMap[prefixedName]
		if !exists {
			// NOTE: any newly created IPSet will still be in the cache if an error is returned later
			iMgr.createIPSet(metadata)
			set = iMgr.setMap[prefixedName]
		}
		if set.Kind != HashSet {
			return npmerrors.Errorf(npmerrors.AppendIPSet, false, fmt.Sprintf("ipset %s is not a hash set", prefixedName))
		}

		// 2. add ip to the set, and update the pod key
		_, ok := set.IPPodKey[ip]
		set.IPPodKey[ip] = podKey
		if ok {
			continue
		}

		iMgr.modifyCacheForKernelMemberUpdate(set)
		metrics.AddEntryToIPSet(prefixedName)
	}
	return nil
}

func (iMgr *IPSetManager) RemoveFromSets(removeFromSets []*IPSetMetadata, ip, podKey string) error {
	if len(removeFromSets) == 0 {
		return nil
	}
	iMgr.Lock()
	defer iMgr.Unlock()

	// 1. check for errors (ignore missing sets)
	for _, metadata := range removeFromSets {
		prefixedName := metadata.GetPrefixName()
		set, exists := iMgr.setMap[prefixedName]
		if !exists {
			continue
		}
		if set.Kind != HashSet {
			return npmerrors.Errorf(npmerrors.DeleteIPSet, false, fmt.Sprintf("ipset %s is not a hash set", prefixedName))
		}

		// 2. remove ip from the set
		cachedPodKey, exists := set.IPPodKey[ip]
		if !exists {
			continue
		}
		// in case the IP belongs to a new Pod, then ignore this Delete call as this might be stale
		if cachedPodKey != podKey {
			klog.Infof(
				"DeleteFromSet: PodOwner has changed for Ip: %s, setName:%s, Old podKey: %s, new podKey: %s. Ignore the delete as this is stale update",
				ip, prefixedName, cachedPodKey, podKey,
			)
			continue
		}

		// update the IP ownership with podkey
		delete(set.IPPodKey, ip)
		iMgr.modifyCacheForKernelMemberUpdate(set)
		metrics.RemoveEntryFromIPSet(prefixedName)
	}
	return nil
}

func (iMgr *IPSetManager) AddToLists(listMetadatas, setMetadatas []*IPSetMetadata) error {
	if len(listMetadatas) == 0 || len(setMetadatas) == 0 {
		return nil
	}
	iMgr.Lock()
	defer iMgr.Unlock()

	// 1. check for errors in members and create any missing sets
	for _, setMetadata := range setMetadatas {
		setName := setMetadata.GetPrefixName()
		set, exists := iMgr.setMap[setName]
		if !exists {
			// NOTE: any newly created IPSet will still be in the cache if an error is returned later
			iMgr.createIPSet(setMetadata)
			set = iMgr.setMap[setName]
		}

		// Nested IPSets are only supported for windows
		// Check if we want to actually use that support
		if set.Kind != HashSet {
			return npmerrors.Errorf(npmerrors.AppendIPSet, false, fmt.Sprintf("ipset %s is not a hash set and nested list sets are not supported", setName))
		}
	}

	for _, listMetadata := range listMetadatas {
		// 2. create the list if it's missing and check for list errors
		listName := listMetadata.GetPrefixName()
		list, exists := iMgr.setMap[listName]
		if !exists {
			// NOTE: any newly created IPSet will still be in the cache if an error is returned later
			iMgr.createIPSet(listMetadata)
			list = iMgr.setMap[listName]
		}

		if list.Kind != ListSet {
			return npmerrors.Errorf(npmerrors.AppendIPSet, false, fmt.Sprintf("ipset %s is not a list set", listName))
		}

		modified := false
		// 3. add all members to the list
		for _, memberMetadata := range setMetadatas {
			memberName := memberMetadata.GetPrefixName()
			// the member shouldn't be the list itself, but this is satisfied since we already asserted that the member is a HashSet
			if list.hasMember(memberName) {
				continue
			}
			member := iMgr.setMap[memberName]

			list.MemberIPSets[memberName] = member
			member.incIPSetReferCount()
			metrics.AddEntryToIPSet(listName)
			listIsInKernel := iMgr.shouldBeInKernel(list)
			if listIsInKernel {
				iMgr.incKernelReferCountAndModifyCache(member)
			}
			modified = true
		}
		if modified {
			iMgr.modifyCacheForKernelMemberUpdate(list)
		}
	}
	return nil
}

func (iMgr *IPSetManager) RemoveFromList(listMetadata *IPSetMetadata, setMetadatas []*IPSetMetadata) error {
	if len(setMetadatas) == 0 {
		return nil
	}
	iMgr.Lock()
	defer iMgr.Unlock()

	// 1. check for errors (ignore missing sets)
	listName := listMetadata.GetPrefixName()
	list, exists := iMgr.setMap[listName]
	if !exists {
		return nil
	}

	if list.Kind != ListSet {
		return npmerrors.Errorf(npmerrors.DeleteIPSet, false, fmt.Sprintf("ipset %s is not a list set", listName))
	}

	modified := false
	for _, setMetadata := range setMetadatas {
		memberName := setMetadata.GetPrefixName()
		member, exists := iMgr.setMap[memberName]
		if !exists {
			continue
		}

		// Nested IPSets are only supported for windows
		// Check if we want to actually use that support
		if member.Kind != HashSet {
			if modified {
				iMgr.modifyCacheForKernelMemberUpdate(list)
			}
			return npmerrors.Errorf(npmerrors.DeleteIPSet, false, fmt.Sprintf("ipset %s is not a hash set and nested list sets are not supported", memberName))
		}

		// 2. remove member from the list
		if !list.hasMember(memberName) {
			continue
		}

		delete(list.MemberIPSets, memberName)
		member.decIPSetReferCount()
		metrics.RemoveEntryFromIPSet(list.Name)
		listIsInKernel := iMgr.shouldBeInKernel(list)
		if listIsInKernel {
			iMgr.decKernelReferCountAndModifyCache(member)
		}
		modified = true
	}
	if modified {
		iMgr.modifyCacheForKernelMemberUpdate(list)
	}
	return nil
}

func (iMgr *IPSetManager) ApplyIPSets() error {
	prometheusTimer := metrics.StartNewTimer()

	iMgr.Lock()
	defer iMgr.Unlock()

	if len(iMgr.toAddOrUpdateCache) == 0 && len(iMgr.toDeleteCache) == 0 {
		klog.Info("[IPSetManager] No IPSets to apply")
		return nil
	}
	defer metrics.RecordIPSetExecTime(prometheusTimer) // record execution time regardless of failure

	klog.Infof("[IPSetManager] toAddUpdateCache %+v \n ", iMgr.toAddOrUpdateCache)
	klog.Infof("[IPSetManager] toDeleteCache %+v \n ", iMgr.toDeleteCache)
	iMgr.sanitizeDirtyCache()

	// Call the appropriate apply ipsets
	err := iMgr.applyIPSets()
	if err != nil {
		return err
	}

	iMgr.clearDirtyCache()
	// TODO could also set the number of ipsets in NPM (not necessarily in kernel) here using len(iMgr.setMap)
	return nil
}

// GetIPsFromSelectorIPSets will take in a map of prefixedSetNames and return an intersection of IPs
func (iMgr *IPSetManager) GetIPsFromSelectorIPSets(setList map[string]struct{}) (map[string]struct{}, error) {
	if len(setList) == 0 {
		return map[string]struct{}{}, nil
	}
	iMgr.Lock()
	defer iMgr.Unlock()

	setintersections := make(map[string]struct{})
	var err error
	firstLoop := true
	for setName := range setList {
		if !iMgr.exists(setName) {
			return nil, npmerrors.Errorf(
				npmerrors.GetSelectorReference,
				false,
				fmt.Sprintf("[ipset manager] selector ipset %s does not exist", setName))
		}
		set := iMgr.setMap[setName]
		if firstLoop {
			intialSetIPs := set.IPPodKey
			for k := range intialSetIPs {
				setintersections[k] = struct{}{}
			}
			firstLoop = false
		}
		setintersections, err = set.getSetIntersection(setintersections)
		if err != nil {
			return nil, err
		}
	}
	return setintersections, err
}

func (iMgr *IPSetManager) GetSelectorReferencesBySet(setName string) (map[string]struct{}, error) {
	iMgr.Lock()
	defer iMgr.Unlock()
	if !iMgr.exists(setName) {
		return nil, npmerrors.Errorf(
			npmerrors.GetSelectorReference,
			false,
			fmt.Sprintf("[ipset manager] selector ipset %s does not exist", setName))
	}
	set := iMgr.setMap[setName]
	return set.SelectorReference, nil
}

func (iMgr *IPSetManager) exists(name string) bool {
	_, ok := iMgr.setMap[name]
	return ok
}

func (iMgr *IPSetManager) modifyCacheForKernelCreation(setName string) {
	iMgr.toAddOrUpdateCache[setName] = struct{}{}
	delete(iMgr.toDeleteCache, setName)
	/*
		TODO kernel-based prometheus metrics

		metrics.IncNumKernelIPSets()
		numEntries := len(set.MemberIPsets) OR len(set.IPPodKey)
		metrics.SetNumEntriesForKernelIPSet(setName, numEntries)
	*/
}

func (iMgr *IPSetManager) incKernelReferCountAndModifyCache(member *IPSet) {
	wasInKernel := iMgr.shouldBeInKernel(member)
	member.incKernelReferCount()
	if !wasInKernel {
		iMgr.modifyCacheForKernelCreation(member.Name)
	}
}

func (iMgr *IPSetManager) shouldBeInKernel(set *IPSet) bool {
	return set.shouldBeInKernel() || iMgr.iMgrCfg.IPSetMode == ApplyAllIPSets
}

func (iMgr *IPSetManager) modifyCacheForKernelRemoval(setName string) {
	iMgr.toDeleteCache[setName] = struct{}{}
	delete(iMgr.toAddOrUpdateCache, setName)
	/*
		TODO kernel-based prometheus metrics

		metrics.DecNumKernelIPSets()
		numEntries := len(set.MemberIPsets) OR len(set.IPPodKey)
		metrics.RemoveAllEntriesFromKernelIPSet(setName)
	*/
}

func (iMgr *IPSetManager) decKernelReferCountAndModifyCache(member *IPSet) {
	member.decKernelReferCount()
	if !iMgr.shouldBeInKernel(member) {
		iMgr.modifyCacheForKernelRemoval(member.Name)
	}
}

func (iMgr *IPSetManager) modifyCacheForKernelMemberUpdate(set *IPSet) {
	if iMgr.shouldBeInKernel(set) {
		iMgr.toAddOrUpdateCache[set.Name] = struct{}{}
	}
}

// sanitizeDirtyCache will check if any set marked as delete is in toAddUpdate
// if so will not delete it
func (iMgr *IPSetManager) sanitizeDirtyCache() {
	for setName := range iMgr.toDeleteCache {
		_, ok := iMgr.toAddOrUpdateCache[setName]
		if ok {
			klog.Errorf("[IPSetManager] Unexpected state in dirty cache %s set is part of both update and delete caches \n ", setName)
		}
	}
}

func (iMgr *IPSetManager) clearDirtyCache() {
	iMgr.toAddOrUpdateCache = make(map[string]struct{})
	iMgr.toDeleteCache = make(map[string]struct{})
}
