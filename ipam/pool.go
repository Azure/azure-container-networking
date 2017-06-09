// Copyright 2017 Microsoft. All rights reserved.
// MIT License

package ipam

import (
	"fmt"
	"net"
	"strings"

	"github.com/Azure/azure-container-networking/log"
	"github.com/Azure/azure-container-networking/platform"
)

const (
	// Default address space IDs.
	LocalDefaultAddressSpaceId  = "local"
	GlobalDefaultAddressSpaceId = "global"
)

const (
	// Address space scopes.
	LocalScope = iota
	GlobalScope
)

var (
	// Azure VNET well-known host IDs.
	defaultGatewayHostId = net.ParseIP("::1")
	dnsPrimaryHostId     = net.ParseIP("::2")
	dnsSecondaryHostId   = net.ParseIP("::3")

	// Azure DNS host proxy well-known address.
	dnsHostProxyAddress = net.ParseIP("168.63.129.16")
)

// Represents the key to an address pool.
type addressPoolId struct {
	AsId        string
	Subnet      string
	ChildSubnet string
}

// Represents a set of non-overlapping address pools.
type addressSpace struct {
	Id    string
	Scope int
	Pools map[string]*addressPool
	epoch int
}

// Represents a subnet and the set of addresses in it.
type addressPool struct {
	as        *addressSpace
	Id        string
	IfName    string
	Subnet    net.IPNet
	Gateway   net.IP
	Addresses map[string]*addressRecord
	addrsByID map[string]*addressRecord
	IsIPv6    bool
	Priority  int
	RefCount  int
	epoch     int
}

// AddressPoolInfo contains information about an address pool.
type AddressPoolInfo struct {
	Subnet         net.IPNet
	Gateway        net.IP
	DnsServers     []net.IP
	UnhealthyAddrs []net.IP
	IsIPv6         bool
	Available      int
	Capacity       int
}

// Represents an IP address in a pool.
type addressRecord struct {
	ID        string
	Addr      net.IP
	InUse     bool
	unhealthy bool
	epoch     int
}

//
// AddressPoolId
//

// Creates a new address pool ID object.
func NewAddressPoolId(asId string, subnet string, childSubnet string) *addressPoolId {
	return &addressPoolId{
		AsId:        asId,
		Subnet:      subnet,
		ChildSubnet: childSubnet,
	}
}

// Creates a new pool ID from a string representation.
func NewAddressPoolIdFromString(s string) (*addressPoolId, error) {
	var pid addressPoolId

	p := strings.Split(s, "|")
	if len(p) > 3 {
		return nil, errInvalidPoolId
	}

	pid.AsId = p[0]
	if len(p) >= 2 {
		pid.Subnet = p[1]
	}
	if len(p) == 3 {
		pid.ChildSubnet = p[2]
	}

	return &pid, nil
}

// Returns the string representation of a pool ID.
func (pid *addressPoolId) String() string {
	s := fmt.Sprintf("%s|%s", pid.AsId, pid.Subnet)
	if pid.ChildSubnet != "" {
		s = fmt.Sprintf("%s|%s", s, pid.ChildSubnet)
	}
	return s
}

//
// AddressSpace
//

// Creates a new addressSpace object.
func (am *addressManager) newAddressSpace(id string, scope int) (*addressSpace, error) {
	if scope != LocalScope && scope != GlobalScope {
		return nil, errInvalidScope
	}

	return &addressSpace{
		Id:    id,
		Scope: scope,
		Pools: make(map[string]*addressPool),
	}, nil
}

// Returns the address space with the given ID.
func (am *addressManager) getAddressSpace(id string) (*addressSpace, error) {
	as := am.AddrSpaces[id]
	if as == nil {
		return nil, errInvalidAddressSpace
	}

	return as, nil
}

// Sets a new or updates an existing address space.
func (am *addressManager) setAddressSpace(as *addressSpace) error {
	as1, ok := am.AddrSpaces[as.Id]
	if !ok {
		am.AddrSpaces[as.Id] = as
	} else {
		as1.merge(as)
	}

	// Notify NetPlugin of external interfaces.
	if am.netApi != nil {
		for _, ap := range as.Pools {
			am.netApi.AddExternalInterface(ap.IfName, ap.Subnet.String())
		}
	}

	am.save()

	return nil
}

// Merges a new address space to an existing one.
func (as *addressSpace) merge(newas *addressSpace) {
	// The new epoch after the merge.
	as.epoch++

	// Add new pools and addresses.
	for pk, pv := range newas.Pools {
		ap := as.Pools[pk]

		if ap == nil {
			// This is a new address pool.
			// Merge it to the existing address space.
			as.Pools[pk] = pv
			pv.as = as
			pv.epoch = as.epoch
		} else {
			// This pool already exists.
			// Compare address records one by one.
			for ak, av := range pv.Addresses {
				ar := ap.Addresses[ak]

				if ar == nil {
					// This is a new address record.
					// Merge it to the existing address pool.
					ap.Addresses[ak] = av
					av.epoch = as.epoch
				} else {
					// This address record already exists.
					ar.epoch = as.epoch
					ar.unhealthy = false
				}

				delete(pv.Addresses, ak)
			}

			pv.as = nil
		}

		delete(newas.Pools, pk)
	}

	// Cleanup stale pools and addresses from the old epoch.
	// Those currently in use will be deleted after they are released.
	for pk, pv := range as.Pools {
		if pv.epoch < as.epoch {
			// This pool may have stale addresses.
			for ak, av := range pv.Addresses {
				if av.epoch == as.epoch {
					// Pool has at least one valid or in-use address.
					pv.epoch = as.epoch
				} else if av.InUse {
					// Address is no longer valid, but still in use.
					pv.epoch = as.epoch
					av.unhealthy = true
				} else {
					// This address is no longer available.
					delete(pv.Addresses, ak)
				}
			}

			// Delete the pool if it has no addresses left.
			if pv.epoch < as.epoch && !pv.isInUse() {
				pv.as = nil
				delete(as.Pools, pk)
			}
		}
	}

	return
}

// Creates a new addressPool object.
func (as *addressSpace) newAddressPool(ifName string, priority int, subnet *net.IPNet) (*addressPool, error) {
	id := subnet.String()

	pool, ok := as.Pools[id]
	if ok {
		return pool, errAddressPoolExists
	}

	v6 := (subnet.IP.To4() == nil)

	pool = &addressPool{
		as:        as,
		Id:        id,
		IfName:    ifName,
		Subnet:    *subnet,
		Gateway:   platform.GenerateAddress(subnet, defaultGatewayHostId),
		Addresses: make(map[string]*addressRecord),
		addrsByID: make(map[string]*addressRecord),
		IsIPv6:    v6,
		Priority:  priority,
		epoch:     as.epoch,
	}

	as.Pools[id] = pool

	return pool, nil
}

// Returns the address pool with the given pool ID.
func (as *addressSpace) getAddressPool(poolId string) (*addressPool, error) {
	ap := as.Pools[poolId]
	if ap == nil {
		return nil, errInvalidPoolId
	}

	return ap, nil
}

// Requests a new address pool from the address space.
func (as *addressSpace) requestPool(poolId string, subPoolId string, options map[string]string, v6 bool) (*addressPool, error) {
	var ap *addressPool
	var err error

	log.Printf("[ipam] Requesting pool with poolId:%v options:%+v v6:%v.", poolId, options, v6)

	if poolId != "" {
		// Return the specific address pool requested.
		// Note sharing of pools is allowed when specifically requested.
		ap = as.Pools[poolId]
		if ap == nil {
			err = errAddressPoolNotFound
		}
	} else {
		// Return any available address pool.
		ifName := options[OptInterfaceName]

		for _, pool := range as.Pools {
			log.Printf("[ipam] Checking pool %v.", pool.Id)

			// Skip if pool is already in use.
			if pool.isInUse() {
				log.Printf("[ipam] Pool is in use.")
				continue
			}

			// Pick a pool from the same address family.
			if pool.IsIPv6 != v6 {
				log.Printf("[ipam] Pool is of a different address family.")
				continue
			}

			// Skip if pool is not on the requested interface.
			if ifName != "" && ifName != pool.IfName {
				log.Printf("[ipam] Pool is not on the requested interface.")
				continue
			}

			log.Printf("[ipam] Pool %v matches requirements.", pool.Id)

			if ap == nil {
				ap = pool
				continue
			}

			// Prefer the pool with the highest priority.
			if pool.Priority > ap.Priority {
				log.Printf("[ipam] Pool is preferred because of priority.")
				ap = pool
			}

			// Prefer the pool with the highest number of addresses.
			if len(pool.Addresses) > len(ap.Addresses) {
				log.Printf("[ipam] Pool is preferred because of capacity.")
				ap = pool
			}
		}

		if ap == nil {
			err = errNoAvailableAddressPools
		}
	}

	if ap != nil {
		ap.RefCount++
	}

	log.Printf("[ipam] Pool request completed with pool:%+v err:%v.", ap, err)

	return ap, err
}

// Releases a previously requested address pool back to its address space.
func (as *addressSpace) releasePool(poolId string) error {
	var err error

	log.Printf("[ipam] Releasing pool with poolId:%v.", poolId)

	ap, ok := as.Pools[poolId]
	if !ok {
		err = errAddressPoolNotFound
	} else {
		if !ap.isInUse() {
			err = errAddressPoolNotInUse
		}
	}

	if err != nil {
		log.Printf("[ipam] Failed to release pool, err:%v.", err)
		return err
	}

	ap.RefCount--

	// Delete address pool if it is no longer available.
	if ap.epoch < as.epoch && !ap.isInUse() {
		log.Printf("[ipam] Deleting stale pool with poolId:%v.", poolId)
		delete(as.Pools, poolId)
	}

	return nil
}

//
// AddressPool
//
// Returns address pool information.
func (ap *addressPool) getInfo() *AddressPoolInfo {
	var available int
	var unhealthyAddrs []net.IP

	for _, ar := range ap.Addresses {
		if !ar.InUse {
			available++
		}
		if ar.unhealthy {
			unhealthyAddrs = append(unhealthyAddrs, ar.Addr)
		}
	}

	info := &AddressPoolInfo{
		Subnet:         ap.Subnet,
		Gateway:        ap.Gateway,
		DnsServers:     []net.IP{dnsHostProxyAddress},
		UnhealthyAddrs: unhealthyAddrs,
		IsIPv6:         ap.IsIPv6,
		Available:      available,
		Capacity:       len(ap.Addresses),
	}

	return info
}

// Returns if an address pool is currently in use.
func (ap *addressPool) isInUse() bool {
	return ap.RefCount > 0
}

// Creates a new addressRecord object.
func (ap *addressPool) newAddressRecord(addr *net.IP) (*addressRecord, error) {
	id := addr.String()

	if !ap.Subnet.Contains(*addr) {
		return nil, errInvalidAddress
	}

	ar, ok := ap.Addresses[id]
	if ok {
		return ar, errAddressExists
	}

	ar = &addressRecord{
		Addr:  *addr,
		epoch: ap.epoch,
	}

	ap.Addresses[id] = ar

	return ar, nil
}

// Requests a new address from the address pool.
func (ap *addressPool) requestAddress(address string, options map[string]string) (string, error) {
	var ar *addressRecord
	var addr *net.IPNet
	var err error
	id := options[OptAddressID]

	log.Printf("[ipam] Requesting address with address:%v options:%+v.", address, options)
	defer func() { log.Printf("[ipam] Address request completed with address:%v err:%v.", addr, err) }()

	if address != "" {
		// Return the specific address requested.
		ar = ap.Addresses[address]
		if ar == nil {
			err = errAddressNotFound
			return "", err
		}
		if ar.InUse {
			// Return the same address if IDs match.
			if id == "" || id != ar.ID {
				err = errAddressInUse
				return "", err
			}
		}
	} else if options[OptAddressType] == OptAddressTypeGateway {
		// Return the pre-assigned gateway address.
		ar = &addressRecord{
			Addr: ap.Gateway,
		}
		id = ""
	} else if id != "" {
		// Return the address with the matching identifier.
		ar = ap.addrsByID[id]
	}

	// If no address was found, return any available address.
	if ar == nil {
		for _, ar = range ap.Addresses {
			if !ar.InUse {
				break
			}
			ar = nil
		}

		if ar == nil {
			return "", errNoAvailableAddresses
		}
	}

	if id != "" {
		ap.addrsByID[id] = ar
	}

	ar.ID = id
	ar.InUse = true

	// Return address in CIDR notation.
	addr = &net.IPNet{
		IP:   ar.Addr,
		Mask: ap.Subnet.Mask,
	}

	return addr.String(), nil
}

// Releases a previously requested address back to its address pool.
func (ap *addressPool) releaseAddress(address string) error {
	var err error

	log.Printf("[ipam] Releasing address %v.", address)
	defer func() { log.Printf("[ipam] Address release completed with err:%v.", err) }()

	ar := ap.Addresses[address]
	if ar == nil {
		// Handle pre-assigned addresses.
		if address == ap.Gateway.String() {
			return nil
		}
		err = errAddressNotFound
		return err
	}

	if !ar.InUse {
		err = errAddressNotInUse
		return err
	}

	if ar.ID != "" {
		delete(ap.addrsByID, ar.ID)
	}

	ar.ID = ""
	ar.InUse = false

	// Delete address record if it is no longer available.
	if ar.epoch < ap.as.epoch {
		delete(ap.Addresses, address)
	}

	return nil
}
