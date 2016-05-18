// Copyright Microsoft Corp.
// All rights reserved.

package network

import (
	"github.com/Azure/Aqua/core"
)

// Network manager manages the set of networks.
type networkManager struct {
	networks map[string]*network
}

// A container network is a set of endpoints allowed to communicate with each other.
type network struct {
	networkId string
	endpoints map[string]*endpoint
	*core.Network
}

// Represents a container endpoint.
type endpoint struct {
	endpointId string
	networkId  string
	sandboxKey string
	*core.Endpoint
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

	if nm.networks[networkId] != nil {
		return nil, errNetworkExists
	}

	nw := &network{
		networkId: networkId,
		endpoints: make(map[string]*endpoint),
	}

	pool := ""
	if len(ipv4Data) > 0 {
		pool = ipv4Data[0].Pool
	}

	nw.Network, err = core.CreateNetwork(networkId, pool, "")
	if err != nil {
		return nil, err
	}

	nm.networks[networkId] = nw

	return nw, nil
}

// Deletes a network object.
func (nm *networkManager) deleteNetwork(networkId string) error {
	nw := nm.networks[networkId]
	if nw == nil {
		return errNetworkNotFound
	}

	err := core.DeleteNetwork(nw.Network)
	if err != nil {
		return err
	}

	delete(nm.networks, networkId)

	return nil
}

// Returns the network with the given ID.
func (nm *networkManager) getNetwork(networkId string) (*network, error) {
	nw := nm.networks[networkId]
	if nw == nil {
		return nil, errNetworkNotFound
	}

	return nw, nil
}

// Returns the endpoint with the given ID.
func (nm *networkManager) getEndpoint(networkId string, endpointId string) (*endpoint, error) {
	nw, err := nm.getNetwork(networkId)
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
	if nw.endpoints[endpointId] != nil {
		return nil, errEndpointExists
	}

	ep := endpoint{
		endpointId: endpointId,
		networkId:  nw.networkId,
	}

	var err error
	ep.Endpoint, err = core.CreateEndpoint(nw.Network, endpointId, ipAddress)
	if err != nil {
		return nil, err
	}

	nw.endpoints[endpointId] = &ep

	return &ep, nil
}

// Deletes an endpoint from the network.
func (nw *network) deleteEndpoint(endpointId string) error {
	ep, err := nw.getEndpoint(endpointId)
	if err != nil {
		return err
	}

	err = core.DeleteEndpoint(ep.Endpoint)
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
func (ep *endpoint) join(sandboxKey string, options map[string] interface{}) error {
	if ep.sandboxKey != "" {
		return errEndpointInUse
	}

	ep.sandboxKey = sandboxKey

	return nil
}

// Removes an endpoint from a sandbox.
func (ep *endpoint) leave() error {
	if ep.sandboxKey == "" {
		return errEndpointNotInUse
	}

	ep.sandboxKey = ""

	return nil
}
