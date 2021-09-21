package ipsets

import (
	"fmt"
	"net"
	"sync"

	"github.com/Azure/azure-container-networking/log"
	"github.com/Azure/azure-container-networking/npm/metrics"
	npmerrors "github.com/Azure/azure-container-networking/npm/util/errors"
)

type IPSetManager struct {
	setMap map[string]*IPSet
	// Map with Key as IPSet name to to emulate set
	// and value as struct{} for minimal memory consumption.
	additionOrUpdateDirtyCache map[string]struct{}
	// IPSets referred to in this cache may be in the setMap, but must be deleted from the kernel
	deletionDirtyCache map[string]struct{}
	sync.Mutex
}

func (iMgr *IPSetManager) exists(name string) bool {
	_, ok := iMgr.setMap[name]
	return ok
}

func NewIPSetManager() *IPSetManager {
	return &IPSetManager{
		setMap:                     make(map[string]*IPSet),
		additionOrUpdateDirtyCache: make(map[string]struct{}),
		deletionDirtyCache:         make(map[string]struct{}),
	}
}

func (iMgr *IPSetManager) modifyCacheForKernelRemoval(setName string) {
	iMgr.deletionDirtyCache[setName] = struct{}{}
	delete(iMgr.additionOrUpdateDirtyCache, setName)
	metrics.DecNumIPSets()
}

func (iMgr *IPSetManager) modifyCacheForKernelAddition(setName string) {
	iMgr.additionOrUpdateDirtyCache[setName] = struct{}{}
	delete(iMgr.deletionDirtyCache, setName)
	metrics.IncNumIPSets()
}

func (iMgr *IPSetManager) modifyCacheForKernelUpdate(setName string) {
	set := iMgr.setMap[setName]
	if set.shouldBeInKernel() {
		iMgr.additionOrUpdateDirtyCache[set.Name] = struct{}{}
	}
}

func (iMgr *IPSetManager) addListToKernel(listName string) {
	set := iMgr.setMap[listName]
	iMgr.modifyCacheForKernelAddition(listName)
	for _, member := range set.MemberIPSets {
		memberWasInKernel := member.shouldBeInKernel()
		member.incKernelCount()
		if !memberWasInKernel {
			iMgr.modifyCacheForKernelAddition(member.Name)
		}
	}
}

func (iMgr *IPSetManager) removeListFromKernel(listName string) {
	set := iMgr.setMap[listName]
	iMgr.modifyCacheForKernelRemoval(listName)
	for _, member := range set.MemberIPSets {
		memberWasInKernel := member.shouldBeInKernel()
		member.incKernelCount()
		if !memberWasInKernel {
			iMgr.modifyCacheForKernelRemoval(member.Name)
		}
	}
}

func (iMgr *IPSetManager) clearDirtyCache() {
	iMgr.additionOrUpdateDirtyCache = make(map[string]struct{})
	iMgr.deletionDirtyCache = make(map[string]struct{})
}

func (iMgr *IPSetManager) CreateIPSet(setName string, setType SetType) error {
	iMgr.Lock()
	defer iMgr.Unlock()
	return iMgr.createIPSet(setName, setType)
}

func (iMgr *IPSetManager) createIPSet(setName string, setType SetType) error {
	if iMgr.exists(setName) {
		return npmerrors.Errorf(npmerrors.CreateIPSet, false, fmt.Sprintf("ipset %s already exists", setName))
	}
	iMgr.setMap[setName] = NewIPSet(setName, setType)
	return nil
}

func (iMgr *IPSetManager) AddToSet(addToSets []string, ip, podKey string) error {
	// check if the IP is IPV4 family
	if net.ParseIP(ip).To4() == nil {
		return npmerrors.Errorf(npmerrors.AppendIPSet, false, "IPV6 not supported")
	}
	iMgr.Lock()
	defer iMgr.Unlock()

	if err := iMgr.checkForIPUpdateErrors(addToSets, npmerrors.AppendIPSet); err != nil {
		return err
	}

	for _, setName := range addToSets {
		set := iMgr.setMap[setName]
		cachedPodKey, ok := set.IPPodKey[ip]
		if ok && cachedPodKey != podKey {
			log.Logf("AddToSet: PodOwner has changed for Ip: %s, setName:%s, Old podKey: %s, new podKey: %s. Replace context with new PodOwner.",
				ip, set.Name, cachedPodKey, podKey)

			set.IPPodKey[ip] = podKey
		}

		// update the IP ownership with podkey
		set.IPPodKey[ip] = podKey
		iMgr.modifyCacheForKernelUpdate(set.Name)

		metrics.AddEntryToIPSet(set.Name)
	}
	return nil
}

func (iMgr *IPSetManager) RemoveFromSet(removeFromSets []string, ip, podKey string) error {
	iMgr.Lock()
	defer iMgr.Unlock()

	if err := iMgr.checkForIPUpdateErrors(removeFromSets, npmerrors.DeleteIPSet); err != nil {
		return err
	}

	for _, setName := range removeFromSets {
		set := iMgr.setMap[setName] // check if the Set exists

		// in case the IP belongs to a new Pod, then ignore this Delete call as this might be stale
		cachedPodKey := set.IPPodKey[ip]
		if cachedPodKey != podKey {
			log.Logf("DeleteFromSet: PodOwner has changed for Ip: %s, setName:%s, Old podKey: %s, new podKey: %s. Ignore the delete as this is stale update",
				ip, setName, cachedPodKey, podKey)
		}

		// update the IP ownership with podkey
		delete(set.IPPodKey, ip)
		iMgr.modifyCacheForKernelUpdate(setName)

		metrics.RemoveEntryFromIPSet(setName)
	}
	return nil
}

func (iMgr *IPSetManager) checkForIPUpdateErrors(setNames []string, npmErrorString string) error {
	for _, setName := range setNames {
		set, exists := iMgr.setMap[setName]
		if !exists {
			return npmerrors.Errorf(npmErrorString, false, fmt.Sprintf("ipset %s does not exist", setName))
		}

		if set.Kind != HashSet {
			return npmerrors.Errorf(npmerrors.DeleteIPSet, false, fmt.Sprintf("ipset %s is not a hash set", setName))
		}
	}
	return nil
}

func (iMgr *IPSetManager) AddToList(listName string, setNames []string) error {
	iMgr.Lock()
	defer iMgr.Unlock()

	if err := iMgr.checkForAddToListErrors(listName, setNames); err != nil {
		return err
	}

	for _, setName := range setNames {
		iMgr.addMemberIPSet(listName, setName)
	}
	iMgr.modifyCacheForKernelUpdate(listName)
	return nil
}

func (iMgr *IPSetManager) RemoveFromList(listName string, setNames []string) error {
	iMgr.Lock()
	defer iMgr.Unlock()

	if err := iMgr.checkForRemoveFromListErrors(listName, setNames); err != nil {
		return err
	}

	for _, setName := range setNames {
		iMgr.removeMemberIPSet(listName, setName)
	}
	iMgr.modifyCacheForKernelUpdate(listName)
	return nil
}

func (iMgr *IPSetManager) checkForAddToListErrors(listName string, setNames []string) error {
	if err := iMgr.checkForMemberUpdateErrors(listName, setNames, npmerrors.AppendIPSet); err != nil {
		return err
	}

	list := iMgr.setMap[listName]
	for _, setName := range setNames {
		if list.hasMember(setName) {
			return npmerrors.Errorf(npmerrors.AppendIPSet, false, fmt.Sprintf("ipset %s is already a member of ipset %s", setName, listName))
		}
	}
	return nil
}

func (iMgr *IPSetManager) checkForRemoveFromListErrors(listName string, setNames []string) error {
	if err := iMgr.checkForMemberUpdateErrors(listName, setNames, npmerrors.DeleteIPSet); err != nil {
		return err
	}

	list := iMgr.setMap[listName]
	for _, setName := range setNames {
		if !list.hasMember(setName) {
			return npmerrors.Errorf(npmerrors.DeleteIPSet, false, fmt.Sprintf("ipset %s is not a member of ipset %s", setName, listName))
		}
	}
	return nil
}

func (iMgr *IPSetManager) checkForMemberUpdateErrors(listName string, memberNames []string, npmErrorString string) error {
	if err := iMgr.checkIfExists(listName, npmErrorString); err != nil {
		return err
	}

	list := iMgr.setMap[listName]
	if list.Kind != ListSet {
		return npmerrors.Errorf(npmErrorString, false, fmt.Sprintf("ipset %s is not a list set", listName))
	}

	for _, memberName := range memberNames {
		if listName == memberName {
			return npmerrors.Errorf(npmErrorString, false, fmt.Sprintf("ipset %s cannot be added to itself", listName))
		}
		if err := iMgr.checkIfExists(memberName, npmErrorString); err != nil {
			return err
		}
		member := iMgr.setMap[memberName]

		// Nested IPSets are only supported for windows
		// Check if we want to actually use that support
		if member.Kind != HashSet {
			return npmerrors.Errorf(npmErrorString, false, fmt.Sprintf("ipset %s is not a hash set and nested list sets are not supported", memberName))
		}
	}
	return nil
}

func (iMgr *IPSetManager) checkIfExists(setName, npmErrorString string) error {
	if !iMgr.exists(setName) {
		return npmerrors.Errorf(npmErrorString, false, fmt.Sprintf("ipset %s does not exist", setName))
	}
	return nil
}

func (iMgr *IPSetManager) addMemberIPSet(listName, memberName string) {
	list := iMgr.setMap[listName]
	member := iMgr.setMap[memberName]

	list.MemberIPSets[member.Name] = member

	listIsInKernel := list.shouldBeInKernel()
	member.incIPSetReferCount()
	if listIsInKernel {
		memberWasInKernel := member.shouldBeInKernel()
		member.incKernelCount()
		if !memberWasInKernel {
			iMgr.modifyCacheForKernelAddition(member.Name)
		}
	}

	metrics.AddEntryToIPSet(list.Name)
}

func (iMgr *IPSetManager) removeMemberIPSet(listName, memberName string) {
	list := iMgr.setMap[listName]
	member := iMgr.setMap[memberName]

	delete(list.MemberIPSets, member.Name)

	listIsInKernel := list.shouldBeInKernel()
	member.decIPSetReferCount()
	if listIsInKernel {
		wasInKernel := member.shouldBeInKernel()
		member.decKernelCount()
		if wasInKernel && !member.shouldBeInKernel() {
			iMgr.modifyCacheForKernelRemoval(member.Name)
		}
	}

	metrics.RemoveEntryFromIPSet(list.Name)
}

func (iMgr *IPSetManager) DeleteList(name string) error {
	return iMgr.DeleteSet(name)
}

func (iMgr *IPSetManager) DeleteSet(name string) error {
	iMgr.Lock()
	defer iMgr.Unlock()
	if err := iMgr.checkIfExists(name, npmerrors.DeleteIPSet); err != nil {
		return err
	}

	set := iMgr.setMap[name]
	if !set.canBeDeleted() {
		return npmerrors.Errorf(npmerrors.DeleteIPSet, false, fmt.Sprintf("ipset %s cannot be deleted", name))
	}

	// the set will not be in the kernel, so there's no need to update the dirty cache
	delete(iMgr.setMap, set.Name)
	return nil
}

func (iMgr *IPSetManager) AddSelectorReference(setName string, selectorName string) error {
	if err := iMgr.checkIfExists(setName, npmerrors.AddSelectorReference); err != nil {
		return err
	}
	set := iMgr.setMap[setName]
	iMgr.addReference(set, selectorName, set.addSelectorReference)
	return nil
}

func (iMgr *IPSetManager) AddNetPolReference(setName string, netPolName string) error {
	if err := iMgr.checkIfExists(setName, npmerrors.AddNetPolReference); err != nil {
		return err
	}
	set := iMgr.setMap[setName]
	iMgr.addReference(set, netPolName, set.addNetPolReference)
	return nil
}

func (iMgr *IPSetManager) DeleteSelectorReference(setName string, selectorName string) error {
	if err := iMgr.checkIfExists(setName, npmerrors.DeleteSelectorReference); err != nil {
		return err
	}
	set := iMgr.setMap[setName]
	iMgr.deleteReference(set, selectorName, set.deleteSelectorReference)
	return nil
}

func (iMgr *IPSetManager) DeleteNetPolReference(setName string, netPolName string) error {
	if err := iMgr.checkIfExists(setName, npmerrors.DeleteNetPolReference); err != nil {
		return err
	}
	set := iMgr.setMap[setName]
	iMgr.deleteReference(set, netPolName, set.deleteNetPolReference)
	return nil
}

func (iMgr *IPSetManager) addReference(set *IPSet, referenceName string, addFunction func(string)) {
	wasInKernel := set.shouldBeInKernel()
	addFunction(referenceName)
	if !wasInKernel {
		iMgr.addListToKernel(set.Name)
	}
}

func (iMgr *IPSetManager) deleteReference(set *IPSet, referenceName string, deleteFunction func(string)) {
	wasInKernel := set.shouldBeInKernel()
	deleteFunction(referenceName)
	if wasInKernel {
		iMgr.removeListFromKernel(set.Name)
	}
}

func (iMgr *IPSetManager) ApplyIPSets(networkID string) error {
	iMgr.Lock()
	defer iMgr.Unlock()

	// Call the appropriate apply ipsets
	err := iMgr.applyIPSets(networkID)
	if err != nil {
		return err
	}

	iMgr.clearDirtyCache()
	// TODO set the number of ipsets in NPM (not necessarily in kernel) using len(iMgr.setMap)
	// or update that number within CreateSet/DeleteSet/List
	return nil
}
