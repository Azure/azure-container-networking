// Copyright 2017 Microsoft. All rights reserved.
// MIT License

package ipam

import (
	"sync"
	"time"

	"github.com/Azure/azure-container-networking/common"
	"github.com/Azure/azure-container-networking/log"
	"github.com/Azure/azure-container-networking/platform"
	"github.com/Azure/azure-container-networking/store"
)

const (
	// IPAM store key.
	storeKey = "IPAM"
)

// AddressManager manages the set of address spaces and pools allocated to containers.
type addressManager struct {
	Version    string
	TimeStamp  time.Time
	AddrSpaces map[string]*addressSpace `json:"AddressSpaces"`
	store      store.KeyValueStore
	source     addressConfigSource
	netApi     common.NetApi
	sync.Mutex
}

// AddressManager API.
type AddressManager interface {
	Initialize(config *common.PluginConfig, options map[string]interface{}) error
	Uninitialize()

	StartSource(options map[string]interface{}) error
	StopSource()

	GetDefaultAddressSpaces() (string, string)

	RequestPool(asId, poolId, subPoolId string, options map[string]string, v6 bool) (string, string, error)
	ReleasePool(asId, poolId string) error
	GetPoolInfo(asId, poolId string) (*AddressPoolInfo, error)

	RequestAddress(asId, poolId, address string, options map[string]string) (string, error)
	ReleaseAddress(asId, poolId, address string) error
}

// AddressConfigSource configures the address pools managed by AddressManager.
type addressConfigSource interface {
	start(sink addressConfigSink) error
	stop()
	refresh() error
}

// AddressConfigSink interface is used by AddressConfigSources to configure address pools.
type addressConfigSink interface {
	newAddressSpace(id string, scope int) (*addressSpace, error)
	setAddressSpace(*addressSpace) error
}

// Creates a new address manager.
func NewAddressManager() (AddressManager, error) {
	am := &addressManager{
		AddrSpaces: make(map[string]*addressSpace),
	}

	return am, nil
}

// Initialize configures address manager.
func (am *addressManager) Initialize(config *common.PluginConfig, options map[string]interface{}) error {
	am.Version = config.Version
	am.store = config.Store
	am.netApi = config.NetApi

	// Restore persisted state.
	err := am.restore()
	if err != nil {
		return err
	}

	// Start source.
	err = am.StartSource(options)

	return err
}

// Uninitialize cleans up address manager.
func (am *addressManager) Uninitialize() {
	am.StopSource()
}

// Restore reads address manager state from persistent store.
func (am *addressManager) restore() error {
	// Skip if a store is not provided.
	if am.store == nil {
		return nil
	}

	// After a reboot, all address resources are implicitly released.
	// Ignore the persisted state if it is older than the last reboot time.
	modTime, err := am.store.GetModificationTime()
	if err == nil {
		log.Printf("[ipam] Store timestamp is %v.", modTime)

		rebootTime, err := platform.GetLastRebootTime()
		if err == nil && rebootTime.After(modTime) {
			log.Printf("[ipam] Ignoring stale state older than last reboot %v.", rebootTime)
			return nil
		}
	}

	// Read any persisted state.
	err = am.store.Read(storeKey, am)
	if err != nil {
		if err == store.ErrKeyNotFound {
			return nil
		} else {
			log.Printf("[ipam] Failed to restore state, err:%v\n", err)
			return err
		}
	}

	// Populate pointers.
	for _, as := range am.AddrSpaces {
		for _, ap := range as.Pools {
			ap.as = as
			ap.addrsByID = make(map[string]*addressRecord)

			for _, ar := range ap.Addresses {
				if ar.ID != "" {
					ap.addrsByID[ar.ID] = ar
				}
			}
		}
	}

	log.Printf("[ipam] Restored state, %+v\n", am)

	return nil
}

// Save writes address manager state to persistent store.
func (am *addressManager) save() error {
	// Skip if a store is not provided.
	if am.store == nil {
		return nil
	}

	// Update time stamp.
	am.TimeStamp = time.Now()

	err := am.store.Write(storeKey, am)
	if err == nil {
		log.Printf("[ipam] Save succeeded.\n")
	} else {
		log.Printf("[ipam] Save failed, err:%v\n", err)
	}
	return err
}

// Starts configuration source.
func (am *addressManager) StartSource(options map[string]interface{}) error {
	var err error

	environment, _ := options[common.OptEnvironment].(string)

	switch environment {
	case common.OptEnvironmentAzure:
		am.source, err = newAzureSource(options)

	case common.OptEnvironmentMAS:
		am.source, err = newMasSource(options)

	case "null":
		am.source, err = newNullSource()

	case "":
		am.source = nil

	default:
		return errInvalidConfiguration
	}

	if am.source != nil {
		log.Printf("[ipam] Starting source %v.", environment)
		err = am.source.start(am)
	}

	if err != nil {
		log.Printf("[ipam] Failed to start source %v, err:%v.", environment, err)
	}

	return err
}

// Stops the configuration source.
func (am *addressManager) StopSource() {
	if am.source != nil {
		am.source.stop()
		am.source = nil
	}
}

// Signals configuration source to refresh.
func (am *addressManager) refreshSource() {
	if am.source != nil {
		log.Printf("[ipam] Refreshing address source.")
		err := am.source.refresh()
		if err != nil {
			log.Printf("[ipam] Source refresh failed, err:%v.\n", err)
		}
	}
}

//
// AddressManager API
//
// Provides atomic stateful wrappers around core IPAM functionality.
//

// GetDefaultAddressSpaces returns the default local and global address space IDs.
func (am *addressManager) GetDefaultAddressSpaces() (string, string) {
	var localId, globalId string

	am.Lock()
	defer am.Unlock()

	am.refreshSource()

	local := am.AddrSpaces[LocalDefaultAddressSpaceId]
	if local != nil {
		localId = local.Id
	}

	global := am.AddrSpaces[GlobalDefaultAddressSpaceId]
	if global != nil {
		globalId = global.Id
	}

	return localId, globalId
}

// RequestPool reserves an address pool.
func (am *addressManager) RequestPool(asId, poolId, subPoolId string, options map[string]string, v6 bool) (string, string, error) {
	am.Lock()
	defer am.Unlock()

	am.refreshSource()

	as, err := am.getAddressSpace(asId)
	if err != nil {
		return "", "", err
	}

	pool, err := as.requestPool(poolId, subPoolId, options, v6)
	if err != nil {
		return "", "", err
	}

	err = am.save()
	if err != nil {
		return "", "", err
	}

	return pool.Id, pool.Subnet.String(), nil
}

// ReleasePool releases a previously reserved address pool.
func (am *addressManager) ReleasePool(asId string, poolId string) error {
	am.Lock()
	defer am.Unlock()

	am.refreshSource()

	as, err := am.getAddressSpace(asId)
	if err != nil {
		return err
	}

	err = as.releasePool(poolId)
	if err != nil {
		return err
	}

	err = am.save()
	if err != nil {
		return err
	}

	return nil
}

// GetPoolInfo returns information about the given address pool.
func (am *addressManager) GetPoolInfo(asId string, poolId string) (*AddressPoolInfo, error) {
	am.Lock()
	defer am.Unlock()

	as, err := am.getAddressSpace(asId)
	if err != nil {
		return nil, err
	}

	ap, err := as.getAddressPool(poolId)
	if err != nil {
		return nil, err
	}

	return ap.getInfo(), nil
}

// RequestAddress reserves a new address from the address pool.
func (am *addressManager) RequestAddress(asId, poolId, address string, options map[string]string) (string, error) {
	am.Lock()
	defer am.Unlock()

	am.refreshSource()

	as, err := am.getAddressSpace(asId)
	if err != nil {
		return "", err
	}

	ap, err := as.getAddressPool(poolId)
	if err != nil {
		return "", err
	}

	addr, err := ap.requestAddress(address, options)
	if err != nil {
		return "", err
	}

	err = am.save()
	if err != nil {
		return "", err
	}

	return addr, nil
}

// ReleaseAddress releases a previously reserved address.
func (am *addressManager) ReleaseAddress(asId string, poolId string, address string) error {
	am.Lock()
	defer am.Unlock()

	am.refreshSource()

	as, err := am.getAddressSpace(asId)
	if err != nil {
		return err
	}

	ap, err := as.getAddressPool(poolId)
	if err != nil {
		return err
	}

	err = ap.releaseAddress(address)
	if err != nil {
		return err
	}

	err = am.save()
	if err != nil {
		return err
	}

	return nil
}
