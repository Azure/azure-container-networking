// Copyright 2017 Microsoft. All rights reserved.
// MIT License

package ipam

const (
	// Libnetwork IPAM plugin endpoint type
	EndpointType = "IpamDriver"

	// Libnetwork IPAM plugin remote API paths
	getCapabilitiesPath  = "/IpamDriver.GetCapabilities"
	getAddressSpacesPath = "/IpamDriver.GetDefaultAddressSpaces"
	requestPoolPath      = "/IpamDriver.RequestPool"
	releasePoolPath      = "/IpamDriver.ReleasePool"
	getPoolInfoPath      = "/IpamDriver.GetPoolInfo"
	requestAddressPath   = "/IpamDriver.RequestAddress"
	releaseAddressPath   = "/IpamDriver.ReleaseAddress"

	// Libnetwork IPAM plugin options
	OptAddressType        = "RequestAddressType"
	OptAddressTypeGateway = "com.docker.network.gateway"
)

// Request sent by libnetwork when querying plugin capabilities.
type getCapabilitiesRequest struct {
}

// Response sent by plugin when registering its capabilities with libnetwork.
type getCapabilitiesResponse struct {
	RequiresMACAddress    bool
	RequiresRequestReplay bool
}

// Request sent by libnetwork when querying the default address space names.
type getDefaultAddressSpacesRequest struct {
}

// Response sent by plugin when returning the default address space names.
type getDefaultAddressSpacesResponse struct {
	LocalDefaultAddressSpace  string
	GlobalDefaultAddressSpace string
}

// Request sent by libnetwork when acquiring a reference to an address pool.
type requestPoolRequest struct {
	AddressSpace string
	Pool         string
	SubPool      string
	Options      map[string]string
	V6           bool
}

// Response sent by plugin when an address pool is successfully referenced.
type requestPoolResponse struct {
	PoolID string
	Pool   string
	Data   map[string]string
}

// Request sent by libnetwork when releasing a previously registered address pool.
type releasePoolRequest struct {
	PoolID string
}

// Response sent by plugin when an address pool is successfully released.
type releasePoolResponse struct {
}

// Request sent when querying address pool information.
type getPoolInfoRequest struct {
	PoolID string
}

// Response sent by plugin when returning address pool information.
type getPoolInfoResponse struct {
	Capacity           int
	Available          int
	UnhealthyAddresses []string
}

// Request sent by libnetwork when reserving an address from a pool.
type requestAddressRequest struct {
	PoolID  string
	Address string
	Options map[string]string
}

// Response sent by plugin when an address is successfully reserved.
type requestAddressResponse struct {
	Address string
	Data    map[string]string
}

// Request sent by libnetwork when releasing an address back to the pool.
type releaseAddressRequest struct {
	PoolID  string
	Address string
}

// Response sent by plugin when an address is successfully released.
type releaseAddressResponse struct {
}
