// Copyright Microsoft Corp.
// All rights reserved.

package network

import (
	"sync"

	"github.com/Azure/Aqua/core"
)

// Network manager manages the set of networks.
type networkManager struct {
	networks map[string]*network
	sync.Mutex
}

// A container network is a set of endpoints allowed to communicate with each other.
type network struct {
	networkId string
	endpoints map[string]*endpoint
	corenw    *core.Network
	sync.Mutex
}

// Represents a container endpoint.
type endpoint struct {
	endpointId string
	networkId  string
	sandboxKey string
	coreep     *core.Endpoint
	sync.Mutex
}

//
// Network Manager
//

// Creates a new network manager.
func newNetworkManager() (*networkManager, error) {
	return &networkManager{
		networks: make(map[string]*network),
	}, nil
}

// Creates a new network object.
func (nm *networkManager) newNetwork(networkId string, options map[string]interface{}, ipv4Data, ipv6Data []ipamData) (*network, error) {
	var err error

	nm.Lock()
	defer nm.Unlock()

	if nm.networks[networkId] != nil {
		return nil, errNetworkExists
	}

	nw := &network{
		networkId: networkId,
		endpoints: make(map[string]*endpoint),
	}

	nw.corenw, err = core.CreateNetwork(networkId, ipv4Data[0].Pool, "")
	if err != nil {
		return nil, err
	}

	nm.networks[networkId] = nw

	return nw, nil
}

// Deletes a network object.
func (nm *networkManager) deleteNetwork(networkId string) error {
	nm.Lock()
	defer nm.Unlock()

	nw := nm.networks[networkId]
	if nw == nil {
		return errNetworkNotFound
	}

	err := core.DeleteNetwork(nw.corenw)
	if err != nil {
		return err
	}

	delete(nm.networks, networkId)

	return nil
}

// Returns the network with the given ID.
func (plugin *netPlugin) getNetwork(networkId string) (*network, error) {
	nw := plugin.nm.networks[networkId]

	if nw == nil {
		return nil, errNetworkNotFound
	}

	return nw, nil
}

// Returns the endpoint with the given ID.
func (plugin *netPlugin) getEndpoint(networkId string, endpointId string) (*endpoint, error) {
	nw, err := plugin.getNetwork(networkId)
	if err != nil {
		return nil, err
	}

	ep, err := nw.getEndpoint(endpointId)
	if err != nil {
		return nil, errEndpointNotFound
	}

	return ep, nil
}

//
// Network
//

// Creates a new endpoint in the network.
func (nw *network) newEndpoint(endpointId string, ipAddress string) (*endpoint, error) {
	nw.Lock()
	defer nw.Unlock()

	if nw.endpoints[endpointId] != nil {
		return nil, errEndpointExists
	}

	ep := endpoint{
		endpointId: endpointId,
		networkId:  nw.networkId,
	}

	var err error
	ep.coreep, err = core.CreateEndpoint(nw.corenw, endpointId, ipAddress)
	if err != nil {
		return nil, err
	}

	nw.endpoints[endpointId] = &ep

	return &ep, nil
}

// Deletes an endpoint from the network.
func (nw *network) deleteEndpoint(endpointId string) error {
	nw.Lock()
	defer nw.Unlock()

	ep, err := nw.getEndpoint(endpointId)
	if err != nil {
		return err
	}

	err = core.DeleteEndpoint(ep.coreep)
	if err != nil {
		return err
	}

	delete(nw.endpoints, endpointId)

	return nil
}

// Returns the endpoint with the given ID.
func (nw *network) getEndpoint(endpointId string) (*endpoint, error) {
	ep := nw.endpoints[endpointId]

	if ep == nil {
		return nil, errEndpointNotFound
	}

	return ep, nil
}

//
// Endpoint
//

// Joins an endpoint to a sandbox.
func (ep *endpoint) join(sandboxKey string) error {
	ep.Lock()
	defer ep.Unlock()

	if ep.sandboxKey != "" {
		return errEndpointInUse
	}

	ep.sandboxKey = sandboxKey

	return nil
}

// Removes an endpoint from a sandbox.
func (ep *endpoint) leave() error {
	ep.Lock()
	defer ep.Unlock()

	if ep.sandboxKey == "" {
		return errEndpointNotInUse
	}

	ep.sandboxKey = ""

	return nil
}
