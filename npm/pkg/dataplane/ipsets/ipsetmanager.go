package ipsets

import (
	"fmt"
	"net"
	"sync"

	"github.com/Azure/azure-container-networking/log"
	"github.com/Azure/azure-container-networking/npm/metrics"
	"github.com/Azure/azure-container-networking/npm/util/errors"
)

type IPSetManager struct {
	setMap      map[string]*IPSet
	dirtyCaches map[string]struct{}
	sync.Mutex
}

func (iMgr *IPSetManager) exists(name string) bool {
	_, ok := iMgr.setMap[name]
	return ok
}

func NewIPSetManager(os string) IPSetManager {
	return IPSetManager{
		setMap:      make(map[string]*IPSet),
		dirtyCaches: make(map[string]struct{}),
	}
}

func (iMgr *IPSetManager) updateDirtyCache(setName string) {
	set, exists := iMgr.setMap[setName] // check if the Set exists
	if !exists {
		return
	}

	// If set is not referenced in netpol then ignore the update
	if len(set.NetPolReference) == 0 && len(set.SelectorReference) == 0 {
		return
	}

	iMgr.dirtyCaches[set.Name] = struct{}{}
	if getSetKind(set) == ListSet {
		// TODO check if we will need to add all the member ipsets
		// also to the dirty cache list
		for _, member := range set.MemberIPSets {
			iMgr.dirtyCaches[member.Name] = struct{}{}
		}
	}
	return
}

func (iMgr *IPSetManager) clearDirtyCache() {
	iMgr.dirtyCaches = make(map[string]struct{})
}

func (iMgr *IPSetManager) CreateIPSet(set *IPSet) error {
	iMgr.Lock()
	defer iMgr.Unlock()
	// Check if the Set already exists
	if iMgr.exists(set.Name) {
		// ipset already exists
		// we should calculate a diff if the members are different
		return nil
	}

	// Call the dataplane specifc fucntion here to
	// create the Set

	// append the cache if dataplane specific function
	// return nil as error
	iMgr.setMap[set.Name] = set

	return nil
}

func (iMgr *IPSetManager) AddToSet(addToSets []*IPSet, ip, podKey string) error {
	// check if the IP is IPV$ family
	if net.ParseIP(ip).To4() == nil {
		return errors.Errorf(errors.AppendIPSet, false, "IPV6 not supported")
	}
	iMgr.Lock()
	defer iMgr.Unlock()

	for _, updatedSet := range addToSets {
		set, exists := iMgr.setMap[updatedSet.Name] // check if the Set exists
		if !exists {
			set = NewIPSet(updatedSet.Name, updatedSet.Type)
			err := iMgr.CreateIPSet(set)
			if err != nil {
				return err
			}
		}

		if getSetKind(set) != HashSet {
			return errors.Errorf(errors.AppendIPSet, false, fmt.Sprintf("ipset %s is not a hash set", set.Name))
		}
		cachedPodKey, ok := set.IPPodKey[ip]
		if ok {
			if cachedPodKey != podKey {
				log.Logf("AddToSet: PodOwner has changed for Ip: %s, setName:%s, Old podKey: %s, new podKey: %s. Replace context with new PodOwner.",
					ip, set.Name, cachedPodKey, podKey)

				set.IPPodKey[ip] = podKey
			}
			return nil
		}

		// Now actually add the IP to the Set
		// err := addToSet(setName, ip)
		// some more error handling here

		// update the IP ownership with podkey
		set.IPPodKey[ip] = podKey
		iMgr.updateDirtyCache(set.Name)

		// Update metrics of the IpSet
		metrics.NumIPSetEntries.Inc()
		metrics.IncIPSetInventory(set.Name)
	}

	return nil
}

func (iMgr *IPSetManager) RemoveFromSet(removeFromSets []string, ip, podKey string) error {
	iMgr.Lock()
	defer iMgr.Unlock()
	for _, setName := range removeFromSets {
		set, exists := iMgr.setMap[setName] // check if the Set exists
		if !exists {
			return errors.Errorf(errors.DeleteIPSet, false, fmt.Sprintf("ipset %s does not exist", setName))
		}

		if getSetKind(set) != HashSet {
			return errors.Errorf(errors.DeleteIPSet, false, fmt.Sprintf("ipset %s is not a hash set", setName))
		}

		// in case the IP belongs to a new Pod, then ignore this Delete call as this might be stale
		cachedPodKey := set.IPPodKey[ip]
		if cachedPodKey != podKey {
			log.Logf("DeleteFromSet: PodOwner has changed for Ip: %s, setName:%s, Old podKey: %s, new podKey: %s. Ignore the delete as this is stale update",
				ip, setName, cachedPodKey, podKey)

			return nil
		}

		// Now actually delete the IP from the Set
		// err := deleteFromSet(setName, ip)
		// some more error handling here

		// update the IP ownership with podkey
		delete(set.IPPodKey, ip)
		iMgr.updateDirtyCache(set.Name)

		// Update metrics of the IpSet
		metrics.NumIPSetEntries.Dec()
		metrics.DecIPSetInventory(setName)
	}

	return nil
}

func (iMgr *IPSetManager) AddToList(listName string, setNames []string) error {
	iMgr.Lock()
	defer iMgr.Unlock()

	for _, setName := range setNames {

		if listName == setName {
			return errors.Errorf(errors.AppendIPSet, false, fmt.Sprintf("list %s cannot be added to itself", listName))
		}
		set, exists := iMgr.setMap[setName] // check if the Set exists
		if !exists {
			return errors.Errorf(errors.AppendIPSet, false, fmt.Sprintf("member ipset %s does not exist", setName))
		}

		// Nested IPSets are only supported for windows
		// Check if we want to actually use that support
		if getSetKind(set) != HashSet {
			return errors.Errorf(errors.DeleteIPSet, false, fmt.Sprintf("member ipset %s is not a Set type and nestetd ipsets are not supported", setName))
		}

		list, exists := iMgr.setMap[listName] // check if the Set exists
		if !exists {
			return errors.Errorf(errors.AppendIPSet, false, fmt.Sprintf("ipset %s does not exist", listName))
		}

		if getSetKind(list) != ListSet {
			return errors.Errorf(errors.AppendIPSet, false, fmt.Sprintf("ipset %s is not a list set", listName))
		}

		// check if Set is a member of List
		listSet, exists := list.MemberIPSets[setName]
		if exists {
			if listSet == set {
				// Set is already a member of List
				return nil
			}
			// Update the ipset in list
			list.MemberIPSets[setName] = set
			return nil
		}

		// Now actually add the Set to the List
		// err := addToList(listName, setName)
		// some more error handling here

		// update the Ipset member list of list
		list.AddMemberIPSet(set)
		set.IncIpsetReferCount()

		// Update metrics of the IpSet
		metrics.NumIPSetEntries.Inc()
		metrics.IncIPSetInventory(setName)
	}

	iMgr.updateDirtyCache(listName)

	return nil
}

func (iMgr *IPSetManager) RemoveFromList(listName string, setNames []string) error {
	iMgr.Lock()
	defer iMgr.Unlock()
	for _, setName := range setNames {
		set, exists := iMgr.setMap[setName] // check if the Set exists
		if !exists {
			return errors.Errorf(errors.DeleteIPSet, false, fmt.Sprintf("ipset %s does not exist", setName))
		}

		if getSetKind(set) != HashSet {
			return errors.Errorf(errors.DeleteIPSet, false, fmt.Sprintf("ipset %s is not a hash set", setName))
		}

		// Nested IPSets are only supported for windows
		//Check if we want to actually use that support
		if getSetKind(set) != HashSet {
			return errors.Errorf(errors.DeleteIPSet, false, fmt.Sprintf("member ipset %s is not a Set type and nestetd ipsets are not supported", setName))
		}

		list, exists := iMgr.setMap[listName] // check if the Set exists
		if !exists {
			return errors.Errorf(errors.DeleteIPSet, false, fmt.Sprintf("ipset %s does not exist", listName))
		}

		if getSetKind(list) != ListSet {
			return errors.Errorf(errors.DeleteIPSet, false, fmt.Sprintf("ipset %s is not a list set", listName))
		}

		// check if Set is a member of List
		_, exists = list.MemberIPSets[setName]
		if !exists {
			return nil
		}

		// Now actually delete the Set from the List
		// err := deleteFromList(listName, setName)
		// some more error handling here

		// delete IPSet from the list
		delete(list.MemberIPSets, setName)
		set.DecIpsetReferCount()

		// Update metrics of the IpSet
		metrics.NumIPSetEntries.Dec()
		metrics.DecIPSetInventory(setName)
	}
	iMgr.updateDirtyCache(listName)

	return nil
}

func (iMgr *IPSetManager) DeleteList(name string) error {
	iMgr.Lock()
	defer iMgr.Unlock()
	set, exists := iMgr.setMap[name] // check if the Set exists
	if !exists {
		return errors.Errorf(errors.AppendIPSet, false, fmt.Sprintf("member ipset %s does not exist", set.Name))
	}

	if !set.CanBeDeleted() {
		return errors.Errorf(errors.DeleteIPSet, false, fmt.Sprintf("ipset %s cannot be deleted", set.Name))
	}

	delete(iMgr.setMap, name)
	return nil
}

func (iMgr *IPSetManager) DeleteSet(name string) error {
	iMgr.Lock()
	defer iMgr.Unlock()
	set, exists := iMgr.setMap[name] // check if the Set exists
	if !exists {
		return errors.Errorf(errors.AppendIPSet, false, fmt.Sprintf("member ipset %s does not exist", set.Name))
	}

	if !set.CanBeDeleted() {
		return errors.Errorf(errors.DeleteIPSet, false, fmt.Sprintf("ipset %s cannot be deleted", set.Name))
	}
	delete(iMgr.setMap, name)
	return nil
}

func (iMgr *IPSetManager) ApplyIPSets() error {
	iMgr.Lock()
	defer iMgr.Unlock()

	for setName := range iMgr.dirtyCaches {
		set, exists := iMgr.setMap[setName] // check if the Set exists
		if !exists {
			return errors.Errorf(errors.AppendIPSet, false, fmt.Sprintf("member ipset %s does not exist", setName))
		}

		fmt.Printf(set.Name)

	}
	iMgr.clearDirtyCache()
	return nil
}
