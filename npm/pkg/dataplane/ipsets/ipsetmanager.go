package ipsets

import (
	"fmt"
	"net"
	"sync"

	"github.com/Azure/azure-container-networking/common"
	"github.com/Azure/azure-container-networking/log"
	"github.com/Azure/azure-container-networking/npm/metrics"
	npmerrors "github.com/Azure/azure-container-networking/npm/util/errors"
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
	iMgrCfg *ipSetManagerCfg
	setMap  map[string]*IPSet
	// Map with Key as IPSet name to to emulate set
	// and value as struct{} for minimal memory consumption.
	toAddOrUpdateCache map[string]struct{}
	// IPSets referred to in this cache may be in the setMap, but must be deleted from the kernel
	toDeleteCache map[string]struct{}
	ioShim        *common.IOShim
	sync.Mutex
}

type ipSetManagerCfg struct {
	ipSetMode   IPSetMode
	networkName string
}

func NewIPSetManager(networkName string, ioShim *common.IOShim) *IPSetManager {
	return &IPSetManager{
		iMgrCfg: &ipSetManagerCfg{
			ipSetMode:   ApplyOnNeed,
			networkName: networkName,
		},
		setMap:             make(map[string]*IPSet),
		toAddOrUpdateCache: make(map[string]struct{}),
		toDeleteCache:      make(map[string]struct{}),
		ioShim:             ioShim,
	}
}

func (iMgr *IPSetManager) CreateIPSet(setMetadata *IPSetMetadata) {
	iMgr.Lock()
	defer iMgr.Unlock()
	prefixedName := setMetadata.GetPrefixName()
	if iMgr.exists(prefixedName) {
		return
	}
	iMgr.setMap[prefixedName] = NewIPSet(setMetadata)
	metrics.IncNumIPSets()
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

	// the set will not be in the kernel since there are no references, so there's no need to update the dirty cache
	delete(iMgr.setMap, name)
	metrics.DecNumIPSets()
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
	wasInKernel := set.shouldBeInKernel()
	set.addReference(referenceName, referenceType)
	if !wasInKernel {
		iMgr.modifyCacheForKernelCreation(set.Name)

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
	wasInKernel := set.shouldBeInKernel() // required because the set may have 0 references i.e. this reference doesn't exist
	set.deleteReference(referenceName, referenceType)
	if wasInKernel && !set.shouldBeInKernel() {
		iMgr.modifyCacheForKernelRemoval(set.Name)

		// if set.Kind == HashSet, then this for loop will do nothing
		for _, member := range set.MemberIPSets {
			iMgr.decKernelReferCountAndModifyCache(member)
		}
	}
	return nil
}

func (iMgr *IPSetManager) AddToSet(addToSets []*IPSetMetadata, ip, podKey string) error {
	// check if the IP is IPV4 family
	if net.ParseIP(ip).To4() == nil {
		return npmerrors.Errorf(npmerrors.AppendIPSet, false, "IPV6 not supported")
	}
	iMgr.Lock()
	defer iMgr.Unlock()

	if err := iMgr.checkForIPUpdateErrors(addToSets, npmerrors.AppendIPSet); err != nil {
		return err
	}

	for _, setMetadata := range addToSets {
		prefixedName := setMetadata.GetPrefixName()
		set := iMgr.setMap[prefixedName]
		cachedPodKey, ok := set.IPPodKey[ip]
		set.IPPodKey[ip] = podKey
		if ok && cachedPodKey != podKey {
			log.Logf("AddToSet: PodOwner has changed for Ip: %s, setName:%s, Old podKey: %s, new podKey: %s. Replace context with new PodOwner.",
				ip, set.Name, cachedPodKey, podKey)
			continue
		}

		iMgr.modifyCacheForKernelMemberUpdate(prefixedName)
		metrics.AddEntryToIPSet(prefixedName)
	}
	return nil
}

func (iMgr *IPSetManager) RemoveFromSet(removeFromSets []*IPSetMetadata, ip, podKey string) error {
	iMgr.Lock()
	defer iMgr.Unlock()

	if err := iMgr.checkForIPUpdateErrors(removeFromSets, npmerrors.DeleteIPSet); err != nil {
		return err
	}

	for _, setMetadata := range removeFromSets {
		prefixedName := setMetadata.GetPrefixName()
		set := iMgr.setMap[prefixedName]

		// in case the IP belongs to a new Pod, then ignore this Delete call as this might be stale
		cachedPodKey, exists := set.IPPodKey[ip]
		if !exists {
			continue
		}
		if cachedPodKey != podKey {
			log.Logf("DeleteFromSet: PodOwner has changed for Ip: %s, setName:%s, Old podKey: %s, new podKey: %s. Ignore the delete as this is stale update",
				ip, prefixedName, cachedPodKey, podKey)
			continue
		}

		// update the IP ownership with podkey
		delete(set.IPPodKey, ip)
		iMgr.modifyCacheForKernelMemberUpdate(prefixedName)
		metrics.RemoveEntryFromIPSet(prefixedName)
	}
	return nil
}

func (iMgr *IPSetManager) AddToList(listMetadata *IPSetMetadata, setMetadatas []*IPSetMetadata) error {
	iMgr.Lock()
	defer iMgr.Unlock()

	if err := iMgr.checkForListMemberUpdateErrors(listMetadata, setMetadatas, npmerrors.AppendIPSet); err != nil {
		return err
	}

	listName := listMetadata.GetPrefixName()
	for _, setMetadata := range setMetadatas {
		setName := setMetadata.GetPrefixName()
		iMgr.addMemberIPSet(listName, setName)
	}
	iMgr.modifyCacheForKernelMemberUpdate(listName)
	metrics.AddEntryToIPSet(listName)
	return nil
}

func (iMgr *IPSetManager) RemoveFromList(listMetadata *IPSetMetadata, setMetadatas []*IPSetMetadata) error {
	iMgr.Lock()
	defer iMgr.Unlock()

	if err := iMgr.checkForListMemberUpdateErrors(listMetadata, setMetadatas, npmerrors.DeleteIPSet); err != nil {
		return err
	}

	listName := listMetadata.GetPrefixName()
	for _, setMetadata := range setMetadatas {
		setName := setMetadata.GetPrefixName()
		iMgr.removeMemberIPSet(listName, setName)
	}
	iMgr.modifyCacheForKernelMemberUpdate(listName)
	metrics.RemoveEntryFromIPSet(listName)
	return nil
}

func (iMgr *IPSetManager) ApplyIPSets(networkID string) error {
	iMgr.Lock()
	defer iMgr.Unlock()

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
	wasInKernel := member.shouldBeInKernel()
	member.incKernelReferCount()
	if !wasInKernel {
		iMgr.modifyCacheForKernelCreation(member.Name)
	}
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
	if !member.shouldBeInKernel() {
		iMgr.modifyCacheForKernelRemoval(member.Name)
	}
}

func (iMgr *IPSetManager) checkForIPUpdateErrors(setNames []*IPSetMetadata, npmErrorString string) error {
	for _, set := range setNames {
		prefixedSetName := set.GetPrefixName()
		if !iMgr.exists(prefixedSetName) {
			return npmerrors.Errorf(npmErrorString, false, fmt.Sprintf("ipset %s does not exist", prefixedSetName))
		}

		set := iMgr.setMap[prefixedSetName]
		if set.Kind != HashSet {
			return npmerrors.Errorf(npmErrorString, false, fmt.Sprintf("ipset %s is not a hash set", prefixedSetName))
		}
	}
	return nil
}

func (iMgr *IPSetManager) modifyCacheForKernelMemberUpdate(setName string) {
	set := iMgr.setMap[setName]
	if set.shouldBeInKernel() {
		iMgr.toAddOrUpdateCache[setName] = struct{}{}
		/*
			TODO kernel-based prometheus metrics

			if isAdd {
				metrics.AddEntryToKernelIPSet(setName)
			} else {
				metrics.RemoveEntryFromKernelIPSet(setName)
			}
		*/
	}
}

func (iMgr *IPSetManager) checkForListMemberUpdateErrors(listMetadata *IPSetMetadata, memberMetadatas []*IPSetMetadata, npmErrorString string) error {
	prefixedListName := listMetadata.GetPrefixName()
	if !iMgr.exists(prefixedListName) {
		return npmerrors.Errorf(npmErrorString, false, fmt.Sprintf("ipset %s does not exist", prefixedListName))
	}

	list := iMgr.setMap[prefixedListName]
	if list.Kind != ListSet {
		return npmerrors.Errorf(npmErrorString, false, fmt.Sprintf("ipset %s is not a list set", prefixedListName))
	}

	for _, memberMetadata := range memberMetadatas {
		memberName := memberMetadata.GetPrefixName()
		if prefixedListName == memberName {
			return npmerrors.Errorf(npmErrorString, false, fmt.Sprintf("ipset %s cannot be added to itself", prefixedListName))
		}
		if !iMgr.exists(memberName) {
			return npmerrors.Errorf(npmErrorString, false, fmt.Sprintf("ipset %s does not exist", memberName))
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

func (iMgr *IPSetManager) addMemberIPSet(listName, memberName string) {
	list := iMgr.setMap[listName]
	if list.hasMember(memberName) {
		return
	}

	member := iMgr.setMap[memberName]

	list.MemberIPSets[memberName] = member
	member.incIPSetReferCount()
	metrics.AddEntryToIPSet(list.Name)
	listIsInKernel := list.shouldBeInKernel()
	if listIsInKernel {
		iMgr.incKernelReferCountAndModifyCache(member)
	}
}

func (iMgr *IPSetManager) removeMemberIPSet(listName, memberName string) {
	list := iMgr.setMap[listName]
	if !list.hasMember(memberName) {
		return
	}

	member := iMgr.setMap[memberName]
	delete(list.MemberIPSets, member.Name)
	member.decIPSetReferCount()
	metrics.RemoveEntryFromIPSet(list.Name)
	listIsInKernel := list.shouldBeInKernel()
	if listIsInKernel {
		iMgr.decKernelReferCountAndModifyCache(member)
	}
}

func (iMgr *IPSetManager) clearDirtyCache() {
	iMgr.toAddOrUpdateCache = make(map[string]struct{})
	iMgr.toDeleteCache = make(map[string]struct{})
}
