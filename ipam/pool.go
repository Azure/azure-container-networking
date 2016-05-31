// Copyright Microsoft Corp.
// All rights reserved.

package ipam

import (
	"fmt"
	"net"
	"strings"
	"sync"
)

const (
	// Default address space IDs.
	localDefaultAddressSpaceId  = "LocalDefaultAddressSpace"
	globalDefaultAddressSpaceId = "GlobalDefaultAddressSpace"

	// Address space scopes.
	localScope  = "local"
	globalScope = "global"
)

// Represents the key to an address pool.
type addressPoolId struct {
	asId        string
	subnet      string
	childSubnet string
}

// Represents a set of non-overlapping address pools.
type addressSpace struct {
	id    string
	scope string
	pools map[string]*addressPool
	epoch int
	sync.Mutex
}

// Represents a subnet and the set of addresses in it.
type addressPool struct {
	id        *addressPoolId
	as        *addressSpace
	ifName    string
	subnet    net.IPNet
	addresses map[string]*addressRecord
	v6        bool
	epoch     int
	ref       int
}

// Represents an IP address in a pool.
type addressRecord struct {
	addr  net.IP
	inUse bool
	epoch int
}

//
// AddressPoolId
//

// Creates a new address pool ID object.
func newAddressPoolId(asId string, subnet string, childSubnet string) *addressPoolId {
	return &addressPoolId{
		asId:        asId,
		subnet:      subnet,
		childSubnet: childSubnet,
	}
}

// Creates a new pool ID from a string representation.
func newAddressPoolIdFromString(s string) (*addressPoolId, error) {
	var pid addressPoolId

	p := strings.Split(s, "|")
	if len(p) > 3 {
		return nil, errInvalidPoolId
	}

	pid.asId = p[0]
	if len(p) >= 2 {
		pid.subnet = p[1]
	}
	if len(p) == 3 {
		pid.childSubnet = p[2]
	}

	return &pid, nil
}

// Returns the string representation of a pool ID.
func (pid *addressPoolId) String() string {
	s := fmt.Sprintf("%s|%s", pid.asId, pid.subnet)
	if pid.childSubnet != "" {
		s = fmt.Sprintf("%s|%s", s, pid.childSubnet)
	}
	return s
}

//
// AddressSpace
//

// Creates a new addressSpace object.
func newAddressSpace(id string, scope string) (*addressSpace, error) {
	if scope != localScope && scope != globalScope {
		return nil, errInvalidScope
	}

	return &addressSpace{
		id:    id,
		scope: scope,
		pools: make(map[string]*addressPool),
	}, nil
}

// Merges a new address space to an existing one.
func (as *addressSpace) merge(newas *addressSpace) {
	as.Lock()
	defer as.Unlock()

	// The new epoch after the merge.
	as.epoch++

	// Add new pools and addresses.
	for pk, pv := range newas.pools {
		ap := as.pools[pk]

		if ap == nil {
			// This is a new address pool.
			// Merge it to the existing address space.
			as.pools[pk] = pv
			delete(newas.pools, pk)
			pv.epoch = as.epoch
		} else {
			// This pool already exists.
			// Compare address records one by one.
			for ak, av := range pv.addresses {
				ar := ap.addresses[ak]

				if ar == nil {
					// This is a new address record.
					// Merge it to the existing address pool.
					ap.addresses[ak] = av
					delete(ap.addresses, ak)
					av.epoch = as.epoch
				} else {
					// This address record already exists.
					ar.epoch = as.epoch
				}
			}

			ap.epoch = as.epoch
		}
	}

	// Cleanup stale pools and addresses from the old epoch.
	for pk, pv := range as.pools {
		if pv.epoch < as.epoch {
			for ak, av := range pv.addresses {
				if !av.inUse {
					delete(pv.addresses, ak)
				}
			}

			if pv.ref == 0 {
				delete(as.pools, pk)
			}
		}
	}

	return
}

// Creates a new addressPool object.
func (as *addressSpace) newAddressPool(ifName string, subnet *net.IPNet) (*addressPool, error) {
	id := newAddressPoolId(as.id, subnet.String(), "")

	as.Lock()
	defer as.Unlock()

	pool, ok := as.pools[id.String()]
	if ok {
		return pool, errAddressPoolExists
	}

	pool = &addressPool{
		id:        id,
		as:        as,
		ifName:    ifName,
		subnet:    *subnet,
		addresses: make(map[string]*addressRecord),
		v6:        subnet.IP.To16() != nil,
		epoch:     as.epoch,
	}

	as.pools[id.String()] = pool

	return pool, nil
}

// Returns the address pool with the given pool ID.
func (as *addressSpace) getAddressPool(poolId string) (*addressPool, error) {
	as.Lock()
	defer as.Unlock()

	ap := as.pools[poolId]
	if ap == nil {
		return nil, errInvalidPoolId
	}

	return ap, nil
}

// Requests a new address pool from the address space.
func (as *addressSpace) requestPool(pool string, subPool string, options map[string]string, v6 bool) (*addressPoolId, error) {
	var ap *addressPool

	as.Lock()
	defer as.Unlock()

	if pool != "" {
		// Return the specific address pool requested.
		ap = as.pools[pool]
		if ap == nil {
			return nil, errAddressPoolNotFound
		}
	} else {
		// Return any available address pool.
		for _, ap = range as.pools {
			if ap.v6 == v6 {
				break
			}
		}

		if ap == nil {
			return nil, errNoAvailableAddressPools
		}
	}

	ap.ref++

	return ap.id, nil
}

// Releases a previously requested address pool back to its address space.
func (as *addressSpace) releasePool(poolId string) error {
	as.Lock()
	defer as.Unlock()

	ap, ok := as.pools[poolId]
	if !ok {
		return errAddressPoolNotFound
	}

	ap.ref--

	// Delete address pool if it is no longer available.
	if ap.ref == 0 && ap.epoch < as.epoch {
		delete(as.pools, poolId)
	}

	return nil
}

//
// AddressPool
//

// Creates a new addressRecord object.
func (ap *addressPool) newAddressRecord(addr *net.IP) (*addressRecord, error) {
	id := addr.String()

	if !ap.subnet.Contains(*addr) {
		return nil, errInvalidAddress
	}

	ap.as.Lock()
	defer ap.as.Unlock()

	ar, ok := ap.addresses[id]
	if ok {
		return ar, errAddressExists
	}

	ar = &addressRecord{
		addr:  *addr,
		epoch: ap.epoch,
	}

	ap.addresses[id] = ar

	return ar, nil
}

// Requests a new address from the address pool.
func (ap *addressPool) requestAddress(address string, options map[string]string) (string, error) {
	var ar *addressRecord

	ap.as.Lock()
	defer ap.as.Unlock()

	if address != "" {
		// Return the specific address requested.
		ar = ap.addresses[address]
		if ar == nil {
			return "", errAddressNotFound
		}
		if ar.inUse {
			return "", errAddressInUse
		}
	} else {
		// Return any available address.
		for _, ar = range ap.addresses {
			if !ar.inUse {
				break
			}
		}

		if ar == nil {
			return "", errNoAvailableAddresses
		}
	}

	ar.inUse = true

	// Return address in CIDR notation.
	addr := net.IPNet{
		IP:   ar.addr,
		Mask: ap.subnet.Mask,
	}

	return addr.String(), nil
}

// Releases a previously requested address back to its address pool.
func (ap *addressPool) releaseAddress(address string) error {
	ap.as.Lock()
	defer ap.as.Unlock()

	ar := ap.addresses[address]
	if ar == nil {
		return errAddressNotFound
	}
	if !ar.inUse {
		return errAddressNotInUse
	}

	ar.inUse = false

	// Delete address record if it is no longer available.
	if ar.epoch < ap.as.epoch {
		delete(ap.addresses, address)
	}

	return nil
}
