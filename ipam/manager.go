// Copyright 2017 Microsoft. All rights reserved.
// MIT License

package ipam

import (
	"sync"

	"github.com/Azure/azure-container-networking/boltwrapper"
	"github.com/Azure/azure-container-networking/common"
	"github.com/Azure/azure-container-networking/log"
	"github.com/Azure/azure-container-networking/platform"
	bolt "go.etcd.io/bbolt"
)

const (
	// IPAM database key.
	databaseKey = "IPAM"
)

// AddressManager manages the set of address spaces and pools allocated to containers.
type addressManager struct {
	Version    string
	AddrSpaces map[string]*addressSpace `json:"AddressSpaces"`
	database   *bolt.DB
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
	ReleaseAddress(asId, poolId, address string, options map[string]string) error
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
	am.database = config.Database
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

// Restore reads address manager state from the on-disk database.
func (am *addressManager) restore() error {
	// Skip if a database is not provided.
	if am.database == nil {
		log.Printf("[ipam] ipam database is nil")
		return nil
	}

	rebooted := false

	// Check if the VM is rebooted.
	if modTime, err := boltwrapper.GetModificationTime(am.database.Path()); err == nil {
		rebootTime, err := platform.GetLastRebootTime()
		log.Printf("[ipam] reboot time %v database mod time %v", rebootTime, modTime)

		if err == nil && rebootTime.After(modTime) {
			rebooted = true
		}
	}

	// Read any persisted state.
	if err := boltwrapper.Read(am.database, databaseKey, am); err != nil {
		if err == boltwrapper.ErrNotFound {
			log.Printf("[ipam] database key not found")
			return nil
		}
		log.Printf("[ipam] Failed to restore state, err:%v\n", err)
		return err
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

	// if rebooted mark the ip as not in use.
	if rebooted {
		log.Printf("[ipam] Rehydrating ipam state from persistent store")
		for _, as := range am.AddrSpaces {
			for _, ap := range as.Pools {
				ap.as = as

				for _, ar := range ap.Addresses {
					ar.InUse = false
				}
			}
		}
	}

	log.Printf("[ipam] Restored state, %+v\n", am)

	return nil
}

// Save writes address manager state to the database.
func (am *addressManager) save() error {
	// Skip if a database is not provided.
	if am.database == nil {
		log.Printf("[ipam] Not saving, no database")
		return nil
	}
	if err := boltwrapper.Write(am.database, databaseKey, am); err != nil {
		log.Printf("[ipam] Save failed, err:%v\n", err)
		return err
	}
	log.Printf("[ipam] Save succeeded.\n")
	return nil
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
func (am *addressManager) ReleaseAddress(asId string, poolId string, address string, options map[string]string) error {
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

	err = ap.releaseAddress(address, options)
	if err != nil {
		return err
	}

	err = am.save()
	if err != nil {
		return err
	}

	return nil
}
