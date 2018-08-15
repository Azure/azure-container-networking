// Copyright 2017 Microsoft. All rights reserved.
// MIT License

package network

import (
	"net"

	"github.com/Azure/azure-container-networking/log"
	"github.com/Azure/azure-container-networking/network/policy"
)

const (
	InfraVnet = 0
)

// Endpoint represents a container network interface.
type endpoint struct {
	Id               string
	HnsId            string `json:",omitempty"`
	SandboxKey       string
	IfName           string
	HostIfName       string
	MacAddress       net.HardwareAddr
	InfraVnetIP      net.IPNet
	IPAddresses      []net.IPNet
	Gateways         []net.IP
	DNS              DNSInfo
	Routes           []RouteInfo
	VlanID           int
	EnableSnatOnHost bool
	EnableInfraVnet  bool
}

// EndpointInfo contains read-only information about an endpoint.
type EndpointInfo struct {
	Id               string
	ContainerID      string
	NetNsPath        string
	IfName           string
	SandboxKey       string
	IfIndex          int
	MacAddress       net.HardwareAddr
	DNS              DNSInfo
	IPAddresses      []net.IPNet
	InfraVnetIP      net.IPNet
	Routes           []RouteInfo
	Policies         []policy.Policy
	Gateways         []net.IP
	EnableSnatOnHost bool
	EnableInfraVnet  bool
	Data             map[string]interface{}
}

// RouteInfo contains information about an IP route.
type RouteInfo struct {
	Dst     net.IPNet
	Gw      net.IP
	DevName string
}

// NewEndpoint creates a new endpoint in the network.
func (nw *network) newEndpoint(epInfo *EndpointInfo) (*endpoint, error) {
	var ep *endpoint
	var err error

	log.Printf("[net] Creating endpoint %+v in network %v.", epInfo, nw.Id)
	defer func() {
		if err != nil {
			log.Printf("[net] Failed to create endpoint %v, err:%v.", epInfo.Id, err)
		}
	}()

	// Call the platform implementation.
	ep, err = nw.newEndpointImpl(epInfo)
	if err != nil {
		return nil, err
	}

	nw.Endpoints[epInfo.Id] = ep
	log.Printf("[net] Created endpoint %+v.", ep)

	return ep, nil
}

// DeleteEndpoint deletes an existing endpoint from the network.
func (nw *network) deleteEndpoint(endpointId string) error {
	var err error

	log.Printf("[net] Deleting endpoint %v from network %v.", endpointId, nw.Id)
	defer func() {
		if err != nil {
			log.Printf("[net] Failed to delete endpoint %v, err:%v.", endpointId, err)
		}
	}()

	// Look up the endpoint.
	ep, err := nw.getEndpoint(endpointId)
	if err != nil {
		log.Printf("[net] Endpoint %v not found. Not Returning error", endpointId)
		return nil
	}

	// Call the platform implementation.
	err = nw.deleteEndpointImpl(ep)
	if err != nil {
		return err
	}

	// Remove the endpoint object.
	delete(nw.Endpoints, endpointId)

	log.Printf("[net] Deleted endpoint %+v.", ep)

	return nil
}

// GetEndpoint returns the endpoint with the given ID.
func (nw *network) getEndpoint(endpointId string) (*endpoint, error) {
	log.Printf("Trying to retrieve endpoint id %v", endpointId)

	ep := nw.Endpoints[endpointId]

	if ep == nil {
		return nil, errEndpointNotFound
	}

	return ep, nil
}

//
// Endpoint
//

// GetInfo returns information about the endpoint.
func (ep *endpoint) getInfo() *EndpointInfo {
	info := &EndpointInfo{
		Id:               ep.Id,
		IPAddresses:      ep.IPAddresses,
		InfraVnetIP:      ep.InfraVnetIP,
		Data:             make(map[string]interface{}),
		MacAddress:       ep.MacAddress,
		SandboxKey:       ep.SandboxKey,
		IfIndex:          0, // Azure CNI supports only one interface
		DNS:              ep.DNS,
		EnableSnatOnHost: ep.EnableSnatOnHost,
		EnableInfraVnet:  ep.EnableInfraVnet,
	}

	for _, route := range ep.Routes {
		info.Routes = append(info.Routes, route)
	}

	for _, gw := range ep.Gateways {
		info.Gateways = append(info.Gateways, gw)
	}

	// Call the platform implementation.
	ep.getInfoImpl(info)

	return info
}

// Attach attaches an endpoint to a sandbox.
func (ep *endpoint) attach(sandboxKey string) error {
	if ep.SandboxKey != "" {
		return errEndpointInUse
	}

	ep.SandboxKey = sandboxKey

	log.Printf("[net] Attached endpoint %v to sandbox %v.", ep.Id, sandboxKey)

	return nil
}

// Detach detaches an endpoint from its sandbox.
func (ep *endpoint) detach() error {
	if ep.SandboxKey == "" {
		return errEndpointNotInUse
	}

	log.Printf("[net] Detached endpoint %v from sandbox %v.", ep.Id, ep.SandboxKey)

	ep.SandboxKey = ""

	return nil
}
