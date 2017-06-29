// Copyright 2017 Microsoft. All rights reserved.
// MIT License

package ipam

const (
	// Libnetwork IPAM plugin endpoint type
	EndpointType = "IpamDriver"

	// Libnetwork IPAM plugin remote API paths
	GetCapabilitiesPath  = "/IpamDriver.GetCapabilities"
	GetAddressSpacesPath = "/IpamDriver.GetDefaultAddressSpaces"
	RequestPoolPath      = "/IpamDriver.RequestPool"
	ReleasePoolPath      = "/IpamDriver.ReleasePool"
	GetPoolInfoPath      = "/IpamDriver.GetPoolInfo"
	RequestAddressPath   = "/IpamDriver.RequestAddress"
	ReleaseAddressPath   = "/IpamDriver.ReleaseAddress"

	// Libnetwork IPAM plugin options
	OptAddressType        = "RequestAddressType"
	OptAddressTypeGateway = "com.docker.network.gateway"
)

// Request sent by libnetwork when querying plugin capabilities.
type GetCapabilitiesRequest struct {
}

// Response sent by plugin when registering its capabilities with libnetwork.
type GetCapabilitiesResponse struct {
	Err                   string
	RequiresMACAddress    bool
	RequiresRequestReplay bool
}

// Request sent by libnetwork when querying the default address space names.
type GetDefaultAddressSpacesRequest struct {
}

// Response sent by plugin when returning the default address space names.
type GetDefaultAddressSpacesResponse struct {
	Err                       string
	LocalDefaultAddressSpace  string
	GlobalDefaultAddressSpace string
}

// Request sent by libnetwork when acquiring a reference to an address pool.
type RequestPoolRequest struct {
	AddressSpace string
	Pool         string
	SubPool      string
	Options      map[string]string
	V6           bool
}

// Response sent by plugin when an address pool is successfully referenced.
type RequestPoolResponse struct {
	Err    string
	PoolID string
	Pool   string
	Data   map[string]string
}

// Request sent by libnetwork when releasing a previously registered address pool.
type ReleasePoolRequest struct {
	PoolID string
}

// Response sent by plugin when an address pool is successfully released.
type ReleasePoolResponse struct {
	Err string
}

// Request sent when querying address pool information.
type GetPoolInfoRequest struct {
	PoolID string
}

// Response sent by plugin when returning address pool information.
type GetPoolInfoResponse struct {
	Err                string
	Capacity           int
	Available          int
	UnhealthyAddresses []string
}

// Request sent by libnetwork when reserving an address from a pool.
type RequestAddressRequest struct {
	PoolID  string
	Address string
	Options map[string]string
}

// Response sent by plugin when an address is successfully reserved.
type RequestAddressResponse struct {
	Err     string
	Address string
	Data    map[string]string
}

// Request sent by libnetwork when releasing an address back to the pool.
type ReleaseAddressRequest struct {
	PoolID  string
	Address string
	Options map[string]string
}

// Response sent by plugin when an address is successfully released.
type ReleaseAddressResponse struct {
	Err string
}
