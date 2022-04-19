package ipsets

import (
	"fmt"
	"strings"

	"github.com/Azure/azure-container-networking/npm/metrics"
	"github.com/Azure/azure-container-networking/npm/util"
	"k8s.io/klog"
)

/*
	dirtyCacheInterface will maintain the dirty cache.
	It may maintain membersToAdd and membersToDelete.
	Members are either IPs, CIDRs, IP-Port pairs, or prefixed set names if the parent is a list.

	Assumptions:
	- if the set becomes dirty via update or destroy, then the set WAS in the kernel before
	- if the set becomes dirty via create, then the set was NOT in the kernel before

	Usage:
	- create, addMember, deleteMember, and destroy are idempotent
	- create should not be called if the set becomes dirty via add/delete or the set is removed from the deleteCache via add/update
	- deleteMember should not be called if the set is in the deleteCache
	- deleteMember is safe to call on members in the kernel and members added via addMember
	- deleteMember is also safe to call on members not in the kernel if the set isn't in the kernel yet (became dirty via create)

	Examples of Expected Behavior:
	- if a set is created and then destroyed, that set will not be in the dirty cache anymore
	- if a set is updated and then destroyed, that set will be in the delete cache
	- if the only operations on a set are adding and removing the same member, the set may still be in the dirty cache, but the member will be untracked
*/
type dirtyCacheInterface interface {
	// reset empties dirty cache
	reset()
	// resetAddOrUpdateCache empties the dirty cache of sets to be created or updated
	resetAddOrUpdateCache()
	// create will mark the new set to be created.
	create(set *IPSet)
	// addMember will mark the set to be updated and track the member to be added (if implemented).
	addMember(set *IPSet, member string)
	// deleteMember will mark the set to be updated and track the member to be deleted (if implemented).
	deleteMember(set *IPSet, member string)
	// delete will mark the set to be deleted in the cache
	destroy(set *IPSet)
	// setsToAddOrUpdate returns the set names to be added or updated
	setsToAddOrUpdate() map[string]struct{}
	// setsToDelete returns the set names to be deleted
	setsToDelete() map[string]struct{}
	// numSetsToAddOrUpdate returns the number of sets to be added or updated
	numSetsToAddOrUpdate() int
	// numSetsToDelete returns the number of sets to be deleted
	numSetsToDelete() int
	// isSetToAddOrUpdate returns true if the set is dirty and should be added or updated
	isSetToAddOrUpdate(setName string) bool
	// isSetToDelete returns true if the set is dirty and should be deleted
	isSetToDelete(setName string) bool
	// printAddOrUpdateCache returns a string representation of the add/update cache
	printAddOrUpdateCache() string
	// printDeleteCache returns a string representation of the delete cache
	printDeleteCache() string
	// memberDiff returns the member diff for the set.
	// Will create a new memberDiff if the setName isn't in the dirty cache.
	memberDiff(setName string) *memberDiff
}

type dirtyCache struct {
	// all maps have keys of set names and values of members to add/delete
	toCreateCache  map[string]*memberDiff
	toUpdateCache  map[string]*memberDiff
	toDestroyCache map[string]*memberDiff
}

func newDirtyCache() *dirtyCache {
	dc := &dirtyCache{}
	dc.reset()
	return dc
}

func (dc *dirtyCache) reset() {
	dc.toCreateCache = make(map[string]*memberDiff)
	dc.toUpdateCache = make(map[string]*memberDiff)
	dc.toDestroyCache = make(map[string]*memberDiff)
}

func (dc *dirtyCache) resetAddOrUpdateCache() {
	dc.toCreateCache = make(map[string]*memberDiff)
	dc.toUpdateCache = make(map[string]*memberDiff)
}

func (dc *dirtyCache) create(set *IPSet) {
	if _, ok := dc.toCreateCache[set.Name]; ok {
		return
	}
	// error checking
	if _, ok := dc.toUpdateCache[set.Name]; ok {
		msg := fmt.Sprintf("create should not be called for set %s since it's in the toUpdateCache", set.Name)
		klog.Warning(msg)
		metrics.SendErrorLogAndMetric(util.IpsmID, msg)
		return
	}

	diff, ok := dc.toDestroyCache[set.Name]
	if ok {
		// transfer from toDestroyCache to toUpdateCache and maintain member diff
		dc.toUpdateCache[set.Name] = diff
		delete(dc.toDestroyCache, set.Name)
	} else {
		// put in the toCreateCache
		dc.toCreateCache[set.Name] = diffOnCreate(set)
	}
}

// could optimize Linux to remove from toUpdateCache if there were no member diff afterwards,
// but leaving as is prevents difference between OS caches
func (dc *dirtyCache) addMember(set *IPSet, member string) {
	diff, ok := dc.toCreateCache[set.Name]
	if !ok {
		diff, ok = dc.toUpdateCache[set.Name]
		if !ok {
			diff, ok = dc.toDestroyCache[set.Name]
			if !ok {
				diff = newMemberDiff()
			}
		}
		dc.toUpdateCache[set.Name] = diff
	}
	delete(dc.toDestroyCache, set.Name)
	diff.addMember(member)
}

// could optimize Linux to remove from toUpdateCache if there were no member diff afterwards,
// but leaving as is prevents difference between OS caches
func (dc *dirtyCache) deleteMember(set *IPSet, member string) {
	// error checking #1
	if dc.isSetToDelete(set.Name) {
		msg := fmt.Sprintf("attempting to delete member %s for set %s in the toDestroyCache", member, set.Name)
		klog.Warning(msg)
		metrics.SendErrorLogAndMetric(util.IpsmID, msg)
		return
	}
	if diff, ok := dc.toCreateCache[set.Name]; ok {
		// don't mark a member to be deleted if it never existed in the kernel
		diff.removeMemberFromDiffToAdd(member)
	} else {
		diff, ok := dc.toUpdateCache[set.Name]
		if !ok {
			diff = newMemberDiff()
		}
		dc.toUpdateCache[set.Name] = diff
		diff.deleteMember(member)
	}
}

func (dc *dirtyCache) destroy(set *IPSet) {
	if dc.isSetToDelete(set.Name) {
		return
	}

	if _, ok := dc.toCreateCache[set.Name]; !ok {
		// mark all current members as membersToDelete to accommodate force delete
		diff, ok := dc.toUpdateCache[set.Name]
		if !ok {
			diff = newMemberDiff()
		}
		if set.Kind == HashSet {
			for ip := range set.IPPodKey {
				diff.deleteMember(ip)
			}
		} else {
			for _, memberSet := range set.MemberIPSets {
				diff.deleteMember(memberSet.HashedName)
			}
		}
		// must call this after deleteMember for correct member diff
		diff.resetMembersToAdd()

		// put the set/diff in the toDestroyCache
		dc.toDestroyCache[set.Name] = diff
	}
	// remove set from toCreateCache or toUpdateCache if necessary
	// if the set/diff was in the toCreateCache before, we'll forget about it
	delete(dc.toCreateCache, set.Name)
	delete(dc.toUpdateCache, set.Name)
}

func (dc *dirtyCache) setsToAddOrUpdate() map[string]struct{} {
	sets := make(map[string]struct{}, len(dc.toCreateCache)+len(dc.toUpdateCache))
	for set := range dc.toCreateCache {
		sets[set] = struct{}{}
	}
	for set := range dc.toUpdateCache {
		sets[set] = struct{}{}
	}
	return sets
}

func (dc *dirtyCache) setsToDelete() map[string]struct{} {
	sets := make(map[string]struct{}, len(dc.toDestroyCache))
	for setName := range dc.toDestroyCache {
		sets[setName] = struct{}{}
	}
	return sets
}

func (dc *dirtyCache) numSetsToAddOrUpdate() int {
	return len(dc.toCreateCache) + len(dc.toUpdateCache)
}

func (dc *dirtyCache) numSetsToDelete() int {
	return len(dc.toDestroyCache)
}

func (dc *dirtyCache) isSetToAddOrUpdate(setName string) bool {
	_, ok1 := dc.toCreateCache[setName]
	_, ok2 := dc.toUpdateCache[setName]
	return ok1 || ok2
}

func (dc *dirtyCache) isSetToDelete(setName string) bool {
	_, ok := dc.toDestroyCache[setName]
	return ok
}

func (dc *dirtyCache) printAddOrUpdateCache() string {
	toCreate := make([]string, 0, len(dc.toCreateCache))
	for setName, diff := range dc.toCreateCache {
		toCreate = append(toCreate, fmt.Sprintf("%s: %+v", setName, diff))
	}
	toUpdate := make([]string, 0, len(dc.toUpdateCache))
	for setName, diff := range dc.toUpdateCache {
		toUpdate = append(toUpdate, fmt.Sprintf("%s: %+v", setName, diff))
	}
	return fmt.Sprintf("to create: [%+v], to update: [%+v]", strings.Join(toCreate, ","), strings.Join(toUpdate, ","))
}

func (dc *dirtyCache) printDeleteCache() string {
	return fmt.Sprintf("%+v", dc.toDestroyCache)
}

func (dc *dirtyCache) memberDiff(setName string) *memberDiff {
	if diff, ok := dc.toCreateCache[setName]; ok {
		return diff
	}
	if diff, ok := dc.toUpdateCache[setName]; ok {
		return diff
	}
	if diff, ok := dc.toDestroyCache[setName]; ok {
		return diff
	}
	return newMemberDiff()
}
