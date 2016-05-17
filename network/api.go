// Copyright Microsoft Corp.
// All rights reserved.

package network

import (
	"fmt"
)

const (
	// Libnetwork network plugin endpoint type
	endpointType = "NetworkDriver"

	// Libnetwork network plugin remote API paths
	getCapabilitiesPath  = "/NetworkDriver.GetCapabilities"
	createNetworkPath    = "/NetworkDriver.CreateNetwork"
	deleteNetworkPath    = "/NetworkDriver.DeleteNetwork"
	createEndpointPath   = "/NetworkDriver.CreateEndpoint"
	deleteEndpointPath   = "/NetworkDriver.DeleteEndpoint"
	joinPath             = "/NetworkDriver.Join"
	leavePath            = "/NetworkDriver.Leave"
	endpointOperInfoPath = "/NetworkDriver.EndpointOperInfo"
)

var (
	// Error response messages returned by plugin.
	errNetworkExists    = fmt.Errorf("Network already exists")
	errNetworkNotFound  = fmt.Errorf("Network not found")
	errEndpointExists   = fmt.Errorf("Endpoint already exists")
	errEndpointNotFound = fmt.Errorf("Endpoint not found")
	errEndpointInUse    = fmt.Errorf("Endpoint is already joined to a sandbox")
	errEndpointNotInUse = fmt.Errorf("Endpoint is not joined to a sandbox")
)

// Request sent by libnetwork when querying plugin capabilities.
type getCapabilitiesRequest struct {
}

// Response sent by plugin when registering its capabilities with libnetwork.
type getCapabilitiesResponse struct {
	Scope string
}

// Request sent by libnetwork when creating a new network.
type createNetworkRequest struct {
	NetworkID string
	Options   map[string]interface{}
	IPv4Data  []ipamData
	IPv6Data  []ipamData
}

// IPAMData represents the per-network IP operational information.
type ipamData struct {
	AddressSpace string
	Pool         string
	Gateway      string
	AuxAddresses map[string]string
}

// Response sent by plugin when a network is created.
type createNetworkResponse struct {
}

// Request sent by libnetwork when deleting an existing network.
type deleteNetworkRequest struct {
	NetworkID string
}

// Response sent by plugin when a network is deleted.
type deleteNetworkResponse struct {
}

// Request sent by libnetwork when creating a new endpoint.
type createEndpointRequest struct {
	NetworkID  string
	EndpointID string
	Options    map[string]interface{}
	Interface  *endpointInterface
}

// Represents a libnetwork endpoint interface.
type endpointInterface struct {
	Address     string
	AddressIPv6 string
	MacAddress  string
}

// Response sent by plugin when an endpoint is created.
type createEndpointResponse struct {
	Interface *endpointInterface
}

// Request sent by libnetwork when deleting an existing endpoint.
type deleteEndpointRequest struct {
	NetworkID  string
	EndpointID string
}

// Response sent by plugin when an endpoint is deleted.
type deleteEndpointResponse struct {
}

// Request sent by libnetwork when joining an endpoint to a sandbox.
type joinRequest struct {
	NetworkID  string
	EndpointID string
	SandboxKey string
	Options    map[string]interface{}
}

// Response sent by plugin when an endpoint is joined to a sandbox.
type joinResponse struct {
	InterfaceName interfaceName
	Gateway       string
	GatewayIPv6   string
	StaticRoutes  []staticRoute
}

// Represents naming information for a joined interface.
type interfaceName struct {
	SrcName   string
	DstName   string
	DstPrefix string
}

// Represents a static route to be added in a sandbox for a joined interface.
type staticRoute struct {
	Destination string
	RouteType   int
	NextHop     string
}

// Request sent by libnetwork when removing an endpoint from its sandbox.
type leaveRequest struct {
	NetworkID  string
	EndpointID string
}

// Response sent by plugin when an endpoint is removed from its sandbox.
type leaveResponse struct {
}

// Request sent by libnetwork when querying operational info of an endpoint.
type endpointOperInfoRequest struct {
	NetworkID  string
	EndpointID string
}

// Response sent by plugin when returning operational info of an endpoint.
type endpointOperInfoResponse struct {
	Value map[string]interface{}
}
