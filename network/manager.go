// Copyright 2017 Microsoft. All rights reserved.
// MIT License

package network

import (
	"sync"

	"github.com/Azure/azure-container-networking/boltwrapper"
	"github.com/Azure/azure-container-networking/common"
	"github.com/Azure/azure-container-networking/log"
	"github.com/Azure/azure-container-networking/platform"
	bolt "go.etcd.io/bbolt"
)

const (
	// Network database key.
	databaseKey = "Network"
	VlanIDKey   = "VlanID"
)

type NetworkClient interface {
	CreateBridge() error
	DeleteBridge() error
	AddL2Rules(extIf *externalInterface) error
	DeleteL2Rules(extIf *externalInterface)
	SetBridgeMasterToHostInterface() error
	SetHairpinOnHostInterface(bool) error
}

type EndpointClient interface {
	AddEndpoints(epInfo *EndpointInfo) error
	AddEndpointRules(epInfo *EndpointInfo) error
	DeleteEndpointRules(ep *endpoint)
	MoveEndpointsToContainerNS(epInfo *EndpointInfo, nsID uintptr) error
	SetupContainerInterfaces(epInfo *EndpointInfo) error
	ConfigureContainerInterfacesAndRoutes(epInfo *EndpointInfo) error
	DeleteEndpoints(ep *endpoint) error
}

// NetworkManager manages the set of container networking resources.
type networkManager struct {
	Version            string
	ExternalInterfaces map[string]*externalInterface
	database           *bolt.DB
	sync.Mutex
}

// NetworkManager API.
type NetworkManager interface {
	Initialize(config *common.PluginConfig) error
	Uninitialize()

	AddExternalInterface(ifName string, subnet string) error

	CreateNetwork(nwInfo *NetworkInfo) error
	DeleteNetwork(networkId string) error
	GetNetworkInfo(networkId string) (*NetworkInfo, error)

	CreateEndpoint(networkId string, epInfo *EndpointInfo) error
	DeleteEndpoint(networkId string, endpointId string) error
	GetEndpointInfo(networkId string, endpointId string) (*EndpointInfo, error)
	AttachEndpoint(networkId string, endpointId string, sandboxKey string) (*endpoint, error)
	DetachEndpoint(networkId string, endpointId string) error
}

// Creates a new network manager.
func NewNetworkManager() (NetworkManager, error) {
	nm := &networkManager{
		ExternalInterfaces: make(map[string]*externalInterface),
	}

	return nm, nil
}

// Initialize configures network manager.
func (nm *networkManager) Initialize(config *common.PluginConfig) error {
	nm.Version = config.Version
	nm.database = config.Database

	// Restore persisted state.
	err := nm.restore()
	return err
}

// Uninitialize cleans up network manager.
func (nm *networkManager) Uninitialize() {
}

// Restore reads network manager state from persistent store.
func (nm *networkManager) restore() error {
	// Skip if a database is not provided.
	if nm.database == nil {
		log.Printf("[net] network database is nil")
		return nil
	}

	rebooted := false
	// After a reboot, all address resources are implicitly released.
	// Ignore the persisted state if it is older than the last reboot time.
	if err := boltwrapper.Read(nm.database, databaseKey, nm); err != nil {
		if err == boltwrapper.ErrNotFound {
			log.Printf("[net] database key %s not found", databaseKey)
			// Considered successful.
			return nil
		} else {
			log.Printf("[net] Failed to restore state, err:%v\n", err)
			return err
		}
	}

	if modTime, err := boltwrapper.GetModificationTime(nm.database.Path()); err == nil {
		rebootTime, err := platform.GetLastRebootTime()
		log.Printf("[net] reboot time %v database mod time %v", rebootTime, modTime)
		if err == nil && rebootTime.After(modTime) {
			rebooted = true
		}
	}

	// Populate pointers.
	for _, extIf := range nm.ExternalInterfaces {
		for _, nw := range extIf.Networks {
			nw.extIf = extIf
		}
	}

	// if rebooted recreate the network that existed before reboot.
	if rebooted {
		log.Printf("[net] Rehydrating network state from persistent store")
		for _, extIf := range nm.ExternalInterfaces {
			for _, nw := range extIf.Networks {
				nwInfo, err := nm.GetNetworkInfo(nw.Id)
				if err != nil {
					log.Printf("[net] Failed to fetch network info for network %v extif %v err %v. This should not happen", nw, extIf, err)
					return err
				}

				extIf.BridgeName = ""

				_, err = nm.newNetworkImpl(nwInfo, extIf)
				if err != nil {
					log.Printf("[net] Restoring network failed for nwInfo %v extif %v. This should not happen %v", nwInfo, extIf, err)
					return err
				}
			}
		}
	}

	log.Printf("[net] Restored state, %+v\n", nm)
	for _, extIf := range nm.ExternalInterfaces {
		log.Printf("External Interface %v", extIf)
		for _, nw := range extIf.Networks {
			log.Printf("network %v", nw)
			for _, ep := range nw.Endpoints {
				log.Printf("endpoint %v", ep)
			}
		}
	}

	return nil
}

// Save writes network manager state to its database.
func (nm *networkManager) save() error {
	// Skip if a database is not provided.
	if nm.database == nil {
		return nil
	}

	if err := boltwrapper.Write(nm.database, databaseKey, nm); err != nil {
		log.Printf("[net] Save failed, err:%v\n", err)
		return err
	}

	log.Printf("[net] Save succeeded.\n")
	return nil
}

//
// NetworkManager API
//
// Provides atomic stateful wrappers around core networking functionality.
//

// AddExternalInterface adds a host interface to the list of available external interfaces.
func (nm *networkManager) AddExternalInterface(ifName string, subnet string) error {
	nm.Lock()
	defer nm.Unlock()

	err := nm.newExternalInterface(ifName, subnet)
	if err != nil {
		return err
	}

	err = nm.save()
	if err != nil {
		return err
	}

	return nil
}

// CreateNetwork creates a new container network.
func (nm *networkManager) CreateNetwork(nwInfo *NetworkInfo) error {
	nm.Lock()
	defer nm.Unlock()

	_, err := nm.newNetwork(nwInfo)
	if err != nil {
		return err
	}

	err = nm.save()
	if err != nil {
		return err
	}

	return nil
}

// DeleteNetwork deletes an existing container network.
func (nm *networkManager) DeleteNetwork(networkId string) error {
	nm.Lock()
	defer nm.Unlock()

	err := nm.deleteNetwork(networkId)
	if err != nil {
		return err
	}

	err = nm.save()
	if err != nil {
		return err
	}

	return nil
}

// GetNetworkInfo returns information about the given network.
func (nm *networkManager) GetNetworkInfo(networkId string) (*NetworkInfo, error) {
	nm.Lock()
	defer nm.Unlock()

	nw, err := nm.getNetwork(networkId)
	if err != nil {
		return nil, err
	}

	nwInfo := &NetworkInfo{
		Id:      networkId,
		Subnets: nw.Subnets,
		Mode:    nw.Mode,
		Options: make(map[string]interface{}),
	}

	getNetworkInfoImpl(nwInfo, nw)

	if nw.extIf != nil {
		nwInfo.BridgeName = nw.extIf.BridgeName
	}

	return nwInfo, nil
}

// CreateEndpoint creates a new container endpoint.
func (nm *networkManager) CreateEndpoint(networkId string, epInfo *EndpointInfo) error {
	nm.Lock()
	defer nm.Unlock()

	nw, err := nm.getNetwork(networkId)
	if err != nil {
		return err
	}

	if nw.VlanId != 0 {
		if epInfo.Data[VlanIDKey] == nil {
			log.Printf("overriding endpoint vlanid with network vlanid")
			epInfo.Data[VlanIDKey] = nw.VlanId
		}
	}

	_, err = nw.newEndpoint(epInfo)
	if err != nil {
		return err
	}

	err = nm.save()
	if err != nil {
		return err
	}

	return nil
}

// DeleteEndpoint deletes an existing container endpoint.
func (nm *networkManager) DeleteEndpoint(networkId string, endpointId string) error {
	nm.Lock()
	defer nm.Unlock()

	nw, err := nm.getNetwork(networkId)
	if err != nil {
		return err
	}

	err = nw.deleteEndpoint(endpointId)
	if err != nil {
		return err
	}

	err = nm.save()
	if err != nil {
		return err
	}

	return nil
}

// GetEndpointInfo returns information about the given endpoint.
func (nm *networkManager) GetEndpointInfo(networkId string, endpointId string) (*EndpointInfo, error) {
	nm.Lock()
	defer nm.Unlock()

	nw, err := nm.getNetwork(networkId)
	if err != nil {
		return nil, err
	}

	ep, err := nw.getEndpoint(endpointId)
	if err != nil {
		return nil, err
	}

	return ep.getInfo(), nil
}

// AttachEndpoint attaches an endpoint to a sandbox.
func (nm *networkManager) AttachEndpoint(networkId string, endpointId string, sandboxKey string) (*endpoint, error) {
	nm.Lock()
	defer nm.Unlock()

	nw, err := nm.getNetwork(networkId)
	if err != nil {
		return nil, err
	}

	ep, err := nw.getEndpoint(endpointId)
	if err != nil {
		return nil, err
	}

	err = ep.attach(sandboxKey)
	if err != nil {
		return nil, err
	}

	err = nm.save()
	if err != nil {
		return nil, err
	}

	return ep, nil
}

// DetachEndpoint detaches an endpoint from its sandbox.
func (nm *networkManager) DetachEndpoint(networkId string, endpointId string) error {
	nm.Lock()
	defer nm.Unlock()

	nw, err := nm.getNetwork(networkId)
	if err != nil {
		return err
	}

	ep, err := nw.getEndpoint(endpointId)
	if err != nil {
		return err
	}

	err = ep.detach()
	if err != nil {
		return err
	}

	err = nm.save()
	if err != nil {
		return err
	}

	return nil
}
