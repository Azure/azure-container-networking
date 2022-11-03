package ipsets

import (
	"fmt"
	"strings"
	"sync"

	"github.com/Azure/azure-container-networking/common"
	"github.com/Azure/azure-container-networking/npm/metrics"
	"github.com/Azure/azure-container-networking/npm/util"
	npmerrors "github.com/Azure/azure-container-networking/npm/util/errors"
	"k8s.io/klog"
)

type IPSetMode string

/*
	IPSet Modes

	- ApplyAllIPSets:
		- all ipsets are added to the kernel
		- ipsets are removed from the kernel when they are deleted from the cache
		- creates empty ipsets
		- adds empty/unreferenced ipsets to the toDelete cache periodically

	- ApplyOnNeed:
		- ipsets are added to the kernel when they are referenced by network policies or lists in the kernel
		- ipsets are removed from the kernel when they no longer have a reference
		- removes empty/unreferenced ipsets from the cache periodically
*/
const (
	ApplyAllIPSets IPSetMode = "all"
	ApplyOnNeed    IPSetMode = "on-need"
)

var (
	emptySetMetadata = &IPSetMetadata{
		Name: "emptyhashset",
		Type: EmptyHashSet,
	}
	emptySetPrefixName = emptySetMetadata.GetPrefixName()
)

type IPSetManager struct {
	iMgrCfg *IPSetManagerCfg
	// emptySet is a direct reference to the empty ipset that should always be in the kernel.
	// This set is used based on the AddEmptySetToLists flag.
	// If emptySet is non-nil, it should be in the kernel or ready to be created in the dirtyCache.
	// Its reference counts are currently unaccounted for and may be incorrect.
	emptySet   *IPSet
	setMap     map[string]*IPSet
	dirtyCache dirtyCacheInterface
	ioShim     *common.IOShim
	sync.RWMutex
}

type IPSetManagerCfg struct {
	IPSetMode IPSetMode
	// NetworkName can be left empty or set to 'azure' (the only supported network)
	NetworkName string
	// AddEmptySetToLists determines whether all lists should have an empty set as a member.
	// This is necessary for HNS (Windows); otherwise, an allow ACL with a list condition
	// allows all IPs if the list has no members.
	AddEmptySetToLists bool
}

func NewIPSetManager(iMgrCfg *IPSetManagerCfg, ioShim *common.IOShim) *IPSetManager {
	return &IPSetManager{
		iMgrCfg:    iMgrCfg,
		emptySet:   nil, // will be set if needed in calls to AddToLists
		setMap:     make(map[string]*IPSet),
		dirtyCache: newDirtyCache(),
		ioShim:     ioShim,
	}
}

/*
	Reconcile removes empty/unreferenced sets from the cache.
	For ApplyAllIPSets mode, those sets are added to the toDeleteCache.
	We can't delete from kernel immediately unless we lock iMgr during policy CRUD.
*/
func (iMgr *IPSetManager) Reconcile() {
	iMgr.Lock()
	defer iMgr.Unlock()
	originalNumSets := len(iMgr.setMap)
	for _, set := range iMgr.setMap {
		iMgr.modifyCacheForCacheDeletion(set, util.SoftDelete)
	}
	numRemovedSets := originalNumSets - len(iMgr.setMap)
	if numRemovedSets > 0 {
		klog.Infof("[IPSetManager] removed %d empty/unreferenced ipsets, updating toDeleteCache to: %+v", numRemovedSets, iMgr.dirtyCache.printDeleteCache())
	}
}

func (iMgr *IPSetManager) ResetIPSets() error {
	iMgr.Lock()
	defer iMgr.Unlock()
	metrics.ResetNumIPSets()
	metrics.ResetIPSetEntries()
	err := iMgr.resetIPSets()
	iMgr.setMap = make(map[string]*IPSet)
	iMgr.emptySet = nil
	iMgr.clearDirtyCache()
	if err != nil {
		metrics.SendErrorLogAndMetric(util.IpsmID, "error: failed to reset ipsetmanager: %s", err.Error())
		return fmt.Errorf("error while resetting ipsetmanager: %w", err)
	}
	return nil
}

func (iMgr *IPSetManager) CreateIPSets(setMetadatas []*IPSetMetadata) {
	iMgr.Lock()
	defer iMgr.Unlock()

	for _, set := range setMetadatas {
		_ = iMgr.createAndGetIPSet(set)
	}
}

func (iMgr *IPSetManager) createAndGetIPSet(setMetadata *IPSetMetadata) *IPSet {
	prefixedName := setMetadata.GetPrefixName()
	set, exists := iMgr.setMap[prefixedName]
	if exists {
		return set
	}

	set = NewIPSet(setMetadata)
	iMgr.setMap[prefixedName] = set
	metrics.IncNumIPSets()
	if iMgr.iMgrCfg.IPSetMode == ApplyAllIPSets {
		iMgr.modifyCacheForKernelCreation(set)
	}

	// if configured, add the empty set to lists of type KeyLabelOfNamespace and KeyValueLabelOfNamespace.
	// The NestedLabelOfPod list ipset type is assumed to always have a member (it is created specifically for network policy pod selectors).
	if iMgr.iMgrCfg.AddEmptySetToLists && (set.Type == KeyLabelOfNamespace || set.Type == KeyValueLabelOfNamespace) {
		if iMgr.emptySet == nil {
			// duplicate of code chunk above
			iMgr.emptySet = NewIPSet(emptySetMetadata)
			iMgr.setMap[emptySetPrefixName] = iMgr.emptySet
			metrics.IncNumIPSets()
			iMgr.modifyCacheForKernelCreation(iMgr.emptySet)
		}

		iMgr.addMemberToList(set, iMgr.emptySet)
	}

	return set
}

// DeleteIPSet expects the prefixed ipset name
func (iMgr *IPSetManager) DeleteIPSet(name string, deleteOption util.DeleteOption) {
	iMgr.Lock()
	defer iMgr.Unlock()
	set, exists := iMgr.setMap[name]
	if !exists {
		return
	}
	iMgr.modifyCacheForCacheDeletion(set, deleteOption)
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

// AddReference creates the set if necessary and adds relevant reference
// it throws an error if the set and reference type are an invalid combination
func (iMgr *IPSetManager) AddReference(setMetadata *IPSetMetadata, referenceName string, referenceType ReferenceType) error {
	iMgr.Lock()
	defer iMgr.Unlock()
	// NOTE: any newly created IPSet will still be in the cache if an error is returned later
	set := iMgr.createAndGetIPSet(setMetadata)
	if referenceType == SelectorType && !set.canSetBeSelectorIPSet() {
		msg := fmt.Sprintf("ipset %s is not a selector ipset it is of type %s", set.Name, set.Type.String())
		metrics.SendErrorLogAndMetric(util.IpsmID, "error: failed to add reference: %s", msg)
		return npmerrors.Errorf(npmerrors.AddSelectorReference, false, msg)
	}
	wasInKernel := iMgr.shouldBeInKernel(set)
	set.addReference(referenceName, referenceType)
	if !wasInKernel {
		// the set should be in the kernel, so add it to the kernel if it wasn't beforehand
		// this branch can only be taken for ApplyOnNeed mode
		iMgr.modifyCacheForKernelCreation(set)

		// for ApplyAllIPSets mode, the set either:
		// a) existed already and doesn't need to be added to toAddOrUpdateCache
		// b) was created in createAndGetIPSet, where it was added to toAddOrUpdateCache

		// if set.Kind == HashSet, then this for loop will do nothing
		for _, member := range set.MemberIPSets {
			iMgr.incKernelReferCountAndModifyCache(member)
		}
	}
	return nil
}

// DeleteReference removes relevant reference
// it throws an error if the set doesn't exist (since a set should exist in the cache & kernel if it has a reference)
func (iMgr *IPSetManager) DeleteReference(setName, referenceName string, referenceType ReferenceType) error {
	iMgr.Lock()
	defer iMgr.Unlock()
	if !iMgr.exists(setName) {
		npmErrorString := npmerrors.DeleteSelectorReference
		if referenceType == NetPolType {
			npmErrorString = npmerrors.DeleteNetPolReference
		}
		msg := fmt.Sprintf("ipset %s does not exist", setName)
		metrics.SendErrorLogAndMetric(util.IpsmID, "error: failed to delete reference: %s", msg)
		return npmerrors.Errorf(npmErrorString, false, msg)
	}

	set := iMgr.setMap[setName]
	wasInKernel := iMgr.shouldBeInKernel(set) // required because the set may not be in the kernel if this reference doesn't exist
	set.deleteReference(referenceName, referenceType)
	if wasInKernel && !iMgr.shouldBeInKernel(set) {
		// remove from kernel if it was in the kernel before and shouldn't be now
		// this branch can only be taken for ApplyOnNeed mode
		iMgr.modifyCacheForKernelRemoval(set)

		// for ApplyAllIPSets mode, we don't want to make the set dirty

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

	if !validateIPSetMemberIP(ip) {
		msg := fmt.Sprintf("error: failed to add to sets: invalid ip %s", ip)
		metrics.SendErrorLogAndMetric(util.IpsmID, msg)
		return npmerrors.Errorf(npmerrors.AppendIPSet, true, msg)
	}

	iMgr.Lock()
	defer iMgr.Unlock()

	for _, metadata := range addToSets {
		// 1. check for errors and create a missing set
		prefixedName := metadata.GetPrefixName()
		// NOTE: any newly created IPSet will still be in the cache if an error is returned later
		set := iMgr.createAndGetIPSet(metadata)
		if set.Kind != HashSet {
			msg := fmt.Sprintf("ipset %s is not a hash set", prefixedName)
			metrics.SendErrorLogAndMetric(util.IpsmID, "error: failed to add to sets: %s", msg)
			return npmerrors.Errorf(npmerrors.AppendIPSet, false, msg)
		}

		// 2. add ip to the set, and update the pod key
		_, ok := set.IPPodKey[ip]
		if !ok {
			iMgr.modifyCacheForKernelMemberAdd(set, ip)
			metrics.AddEntryToIPSet(prefixedName)
		}
		set.IPPodKey[ip] = podKey
	}
	return nil
}

func (iMgr *IPSetManager) RemoveFromSets(removeFromSets []*IPSetMetadata, ip, podKey string) error {
	if len(removeFromSets) == 0 {
		return nil
	}

	if !validateIPSetMemberIP(ip) {
		msg := fmt.Sprintf("error: failed to add to sets: invalid ip %s", ip)
		metrics.SendErrorLogAndMetric(util.IpsmID, msg)
		return npmerrors.Errorf(npmerrors.AppendIPSet, true, msg)
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
			msg := fmt.Sprintf("ipset %s is not a hash set", prefixedName)
			metrics.SendErrorLogAndMetric(util.IpsmID, "error: failed to remove from sets: %s", msg)
			return npmerrors.Errorf(npmerrors.DeleteIPSet, false, msg)
		}

		// 2. remove ip from the set
		cachedPodKey, exists := set.IPPodKey[ip]
		if !exists {
			continue
		}
		// in case the IP belongs to a new Pod, then ignore this Delete call as this might be stale
		if cachedPodKey != podKey {
			klog.Infof(
				"[IPSetManager] DeleteFromSet: PodOwner has changed for Ip: %s, setName:%s, Old podKey: %s, new podKey: %s. Ignore the delete as this is stale update",
				ip, prefixedName, cachedPodKey, podKey,
			)
			continue
		}

		// update the IP ownership with podkey
		iMgr.modifyCacheForKernelMemberDelete(set, ip)
		delete(set.IPPodKey, ip)
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
		// NOTE: any newly created IPSet will still be in the cache if an error is returned later
		set := iMgr.createAndGetIPSet(setMetadata)

		// Nested IPSets are only supported for windows
		// Check if we want to actually use that support
		if set.Kind != HashSet {
			msg := fmt.Sprintf("ipset %s is not a hash set and nested list sets are not supported", set.Name)
			metrics.SendErrorLogAndMetric(util.IpsmID, "error: failed to add to lists: %s", msg)
			return npmerrors.Errorf(npmerrors.AppendIPSet, false, msg)
		}
	}

	for _, listMetadata := range listMetadatas {
		// 2. create the list if it's missing and check for list errors
		// NOTE: any newly created IPSet will still be in the cache if an error is returned later
		list := iMgr.createAndGetIPSet(listMetadata)

		if list.Kind != ListSet {
			msg := fmt.Sprintf("ipset %s is not a list set", list.Name)
			metrics.SendErrorLogAndMetric(util.IpsmID, "error: failed to add to lists: %s", msg)
			return npmerrors.Errorf(npmerrors.AppendIPSet, false, msg)
		}

		// 3. add all members to the list
		for _, memberMetadata := range setMetadatas {
			memberName := memberMetadata.GetPrefixName()
			if memberName == "" {
				metrics.SendErrorLogAndMetric(util.IpsmID, "[AddToLists] warning: adding empty member name to list %s", list.Name)
				continue
			}
			// the member shouldn't be the list itself, but this is satisfied since we already asserted that the member is a HashSet
			if list.hasMember(memberName) {
				continue
			}
			member := iMgr.setMap[memberName]

			iMgr.addMemberToList(list, member)
			listIsInKernel := iMgr.shouldBeInKernel(list)
			if listIsInKernel {
				iMgr.incKernelReferCountAndModifyCache(member)
			}
		}
	}

	return nil
}

func (iMgr *IPSetManager) addMemberToList(list, member *IPSet) {
	iMgr.modifyCacheForKernelMemberAdd(list, member.HashedName)
	list.MemberIPSets[member.Name] = member
	member.incIPSetReferCount()
	metrics.AddEntryToIPSet(list.Name)
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
		msg := fmt.Sprintf("ipset %s is not a list set", listName)
		metrics.SendErrorLogAndMetric(util.IpsmID, "error: failed to remove from list: %s", msg)
		return npmerrors.Errorf(npmerrors.DeleteIPSet, false, msg)
	}

	for _, setMetadata := range setMetadatas {
		memberName := setMetadata.GetPrefixName()
		if memberName == "" {
			metrics.SendErrorLogAndMetric(util.IpsmID, "[RemoveFromList] warning: tried to remove empty member name from list %s", list.Name)
			continue
		}

		if iMgr.iMgrCfg.AddEmptySetToLists && memberName == emptySetPrefixName {
			metrics.SendErrorLogAndMetric(util.IpsmID, "[RemoveFromList] warning: tried to remove empty set from list %s", list.Name)
			continue
		}

		member, exists := iMgr.setMap[memberName]
		if !exists {
			continue
		}

		// Nested IPSets are only supported for windows
		// Check if we want to actually use that support
		if member.Kind != HashSet {
			msg := fmt.Sprintf("ipset %s is not a hash set and nested list sets are not supported", memberName)
			metrics.SendErrorLogAndMetric(util.IpsmID, "error: failed to remove from list: %s", msg)
			return npmerrors.Errorf(npmerrors.DeleteIPSet, false, msg)
		}

		// 2. remove member from the list
		if !list.hasMember(memberName) {
			continue
		}

		iMgr.modifyCacheForKernelMemberDelete(list, member.HashedName)
		delete(list.MemberIPSets, memberName)
		member.decIPSetReferCount()
		metrics.RemoveEntryFromIPSet(list.Name)
		listIsInKernel := iMgr.shouldBeInKernel(list)
		if listIsInKernel {
			iMgr.decKernelReferCountAndModifyCache(member)
		}
	}
	return nil
}

func (iMgr *IPSetManager) ApplyIPSets() error {
	iMgr.Lock()
	defer iMgr.Unlock()

	if iMgr.dirtyCache.numSetsToAddOrUpdate() == 0 && iMgr.dirtyCache.numSetsToDelete() == 0 {
		klog.Info("[IPSetManager] No IPSets to apply")
		return nil
	}

	klog.Infof(
		"[IPSetManager] dirty caches. toAddUpdateCache: %s, toDeleteCache: %s",
		iMgr.dirtyCache.printAddOrUpdateCache(), iMgr.dirtyCache.printDeleteCache(),
	)
	iMgr.sanitizeDirtyCache()

	// Call the appropriate apply ipsets
	prometheusTimer := metrics.StartNewTimer()
	defer metrics.RecordIPSetExecTime(prometheusTimer) // record execution time regardless of failure
	err := iMgr.applyIPSets()
	if err != nil {
		metrics.SendErrorLogAndMetric(util.IpsmID, "error: failed to apply ipsets: %s", err.Error())
		return err
	}

	iMgr.clearDirtyCache()
	// TODO could also set the number of ipsets in NPM (not necessarily in kernel) here using len(iMgr.setMap)
	return nil
}

func (iMgr *IPSetManager) GetAllIPSets() map[string]string {
	iMgr.RLock()
	defer iMgr.RUnlock()
	setMap := make(map[string]string, len(iMgr.setMap))
	for _, metadata := range iMgr.setMap {
		setMap[metadata.HashedName] = metadata.Name
	}
	return setMap
}

func (iMgr *IPSetManager) exists(name string) bool {
	_, ok := iMgr.setMap[name]
	return ok
}

// the metric for number of ipsets in the kernel will be lower than in reality until the next applyIPSet call
func (iMgr *IPSetManager) modifyCacheForCacheDeletion(set *IPSet, deleteOption util.DeleteOption) {
	if set == iMgr.emptySet {
		return
	}

	if deleteOption == util.ForceDelete {
		// If force delete, then check if Set is used by other set or network policy
		// else delete the set even if it has members
		if !set.canBeForceDeleted() {
			return
		}
	} else if !set.canBeDeleted(iMgr.emptySet) {
		return
	}

	delete(iMgr.setMap, set.Name)
	metrics.DeleteIPSet(set.Name)
	if iMgr.iMgrCfg.IPSetMode == ApplyAllIPSets {
		iMgr.modifyCacheForKernelRemoval(set)
	}
	// if mode is ApplyOnNeed, the set will not be in the kernel (or will be in the delete cache already) since there are no references
}

func (iMgr *IPSetManager) modifyCacheForKernelCreation(set *IPSet) {
	iMgr.dirtyCache.create(set)
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
		iMgr.modifyCacheForKernelCreation(member)
	}
}

func (iMgr *IPSetManager) shouldBeInKernel(set *IPSet) bool {
	return set.shouldBeInKernel() || iMgr.iMgrCfg.IPSetMode == ApplyAllIPSets || set == iMgr.emptySet
}

func (iMgr *IPSetManager) modifyCacheForKernelRemoval(set *IPSet) {
	iMgr.dirtyCache.destroy(set)
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
		iMgr.modifyCacheForKernelRemoval(member)
	}
}

func (iMgr *IPSetManager) modifyCacheForKernelMemberAdd(set *IPSet, member string) {
	if iMgr.shouldBeInKernel(set) {
		iMgr.dirtyCache.addMember(set, member)
	}
}

func (iMgr *IPSetManager) modifyCacheForKernelMemberDelete(set *IPSet, member string) {
	if iMgr.shouldBeInKernel(set) {
		iMgr.dirtyCache.deleteMember(set, member)
	}
}

// sanitizeDirtyCache will check if any set marked as delete is in toAddUpdate
// if so will not delete it
func (iMgr *IPSetManager) sanitizeDirtyCache() {
	anyProblems := false
	for setName := range iMgr.dirtyCache.setsToDelete() {
		if iMgr.dirtyCache.isSetToAddOrUpdate(setName) {
			klog.Errorf("[IPSetManager] Unexpected state in dirty cache %s set is part of both update and delete caches", setName)
			anyProblems = true
		}
	}
	if anyProblems {
		metrics.SendErrorLogAndMetric(util.IpsmID, "error: some dirty cache sets are part of both update and delete caches")
	}
}

func (iMgr *IPSetManager) clearDirtyCache() {
	iMgr.dirtyCache.reset()
}

// validateIPSetMemberIP helps valid if a member added to an HashSet has valid IP or CIDR
func validateIPSetMemberIP(ip string) bool {
	// possible formats
	// 192.168.0.1
	// 192.168.0.1,tcp:25227
	// 192.168.0.1 nomatch
	// 192.168.0.0/24
	// 192.168.0.0/24,tcp:25227
	// 192.168.0.0/24 nomatch
	// always guaranteed to have ip, not guaranteed to have port + protocol
	ipDetails := strings.Split(ip, ",")
	ipField := strings.Split(ipDetails[0], " ")

	return util.IsIPV4(ipField[0])
}
