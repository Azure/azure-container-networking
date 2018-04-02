// Copyright 2017 Microsoft. All rights reserved.
// MIT License

package cns

// Container Network Service remote API Contract
const (
	SetEnvironmentPath             = "/network/environment"
	CreateNetworkPath              = "/network/create"
	DeleteNetworkPath              = "/network/delete"
	ReserveIPAddressPath           = "/network/ip/reserve"
	ReleaseIPAddressPath           = "/network/ip/release"
	GetHostLocalIPPath             = "/network/ip/hostlocal"
	GetIPAddressUtilizationPath    = "/network/ip/utilization"
	GetUnhealthyIPAddressesPath    = "/network/ipaddresses/unhealthy"
	GetHealthReportPath            = "/network/health"
	V1Prefix                       = "/v0.1"
	V2Prefix                       = "/v0.2"
	CreateOrUpdateNetworkContainer = "/network/createorupdatenetworkcontainer"
	DeleteNetworkContainer         = "/network/deletenetworkcontainer"
	GetNetworkContainerStatus      = "/network/getnetworkcontainerstatus"
	GetInterfaceForContainer       = "/network/getinterfaceforcontainer"
)

// SetEnvironmentRequest describes the Request to set the environment in CNS.
type SetEnvironmentRequest struct {
	Location    string
	NetworkType string
}

// OverlayConfiguration describes configuration for all the nodes that are part of overlay.
type OverlayConfiguration struct {
	NodeCount     int
	LocalNodeIP   string
	OverlaySubent Subnet
	NodeConfig    []NodeConfiguration
}

// CreateNetworkRequest describes request to create the network.
type CreateNetworkRequest struct {
	NetworkName          string
	OverlayConfiguration OverlayConfiguration
	Options              map[string]interface{}
}

// DeleteNetworkRequest describes request to delete the network.
type DeleteNetworkRequest struct {
	NetworkName string
}

// ReserveIPAddressRequest describes request to reserve an IP Address
type ReserveIPAddressRequest struct {
	ReservationID string
}

// ReserveIPAddressResponse describes response to reserve an IP address.
type ReserveIPAddressResponse struct {
	Response  Response
	IPAddress string
}

// ReleaseIPAddressRequest describes request to release an IP Address.
type ReleaseIPAddressRequest struct {
	ReservationID string
}

// IPAddressesUtilizationResponse describes response for ip address utilization.
type IPAddressesUtilizationResponse struct {
	Response  Response
	Available int
	Reserved  int
	Unhealthy int
}

// GetIPAddressesResponse describes response containing requested ip addresses.
type GetIPAddressesResponse struct {
	Response    Response
	IPAddresses []string
}

// HostLocalIPAddressResponse describes reponse that returns the host local IP Address.
type HostLocalIPAddressResponse struct {
	Response  Response
	IPAddress string
}

// Subnet contains the ip address and the number of bits in prefix.
type Subnet struct {
	IPAddress    string
	PrefixLength int
}

// NodeConfiguration describes confguration for a node in overlay network.
type NodeConfiguration struct {
	NodeIP     string
	NodeID     string
	NodeSubnet Subnet
}

// Response describes generic response from CNS.
type Response struct {
	ReturnCode int
	Message    string
}

// OptionMap describes generic options that can be passed to CNS.
type OptionMap map[string]interface{}

// Response to a failed request.
type errorResponse struct {
	Err string
}

// CreateNetworkContainerRequest specifies request to create a network container or network isolation boundary.
type CreateNetworkContainerRequest struct {
	Version                          string
	NetworkContainerType             string
	NetworkContainerid               string // Mandatory input.
	PrimaryInterfaceIdentifier       string // Primary CA.
	AuthorizationToken               string
	NetworkContainerOrchestratorInfo NetworkContainerOrchestratorInfo
	IPConfiguration                  IPConfiguration
	MultiTenancyInfo                 MultiTenancyInfo
	VnetAddressSpace                 []IPSubnet // To setup SNAT (should include service endpoint vips).
	Routes                           []Route
}

// NetworkContainerOrchestratorInfo contains orchestrator type, pod name and pod namespace.
type NetworkContainerOrchestratorInfo struct {
	OrchestratorType string
	Podname          string
	PodNamespace     string
}

// MultiTenancyInfo contains encap type and id.
type MultiTenancyInfo struct {
	EncapType string
	ID        int // This can be vlanid, vxlanid, gre-key etc. (depends on EnacapType).
}

// IPConfiguration contains details about ip config to provision in the VM.
type IPConfiguration struct {
	IPSubnet         IPSubnet
	DNSServers       []string
	GatewayIPAddress string
}

// IPSubnet contains ip subnet.
type IPSubnet struct {
	IPAddress    string
	PrefixLength uint8
}

// Route describes an entry in routing table.
type Route struct {
	IPAddress        string
	GatewayIPAddress string
	InterfaceToUse   string
}

// CreateNetworkContainerResponse specifies response of creating a network container.
type CreateNetworkContainerResponse struct {
	Response Response
}

// GetNetworkContainerStatusRequest specifies the details about the request to retrieve status of a specifc network container.
type GetNetworkContainerStatusRequest struct {
	NetworkContainerid string
}

// GetNetworkContainerStatusResponse specifies response of retriving a network container status.
type GetNetworkContainerStatusResponse struct {
	NetworkContainerid string
	Version            string
	AzureHostVersion   string
	Response           Response
}

// GetNetworkContainerRequest specifies the details about the request to retrieve a specifc network container.
type GetNetworkContainerRequest struct {
}

// GetNetworkContainerResponse describes the response to retrieve a specifc network container.
type GetNetworkContainerResponse struct {
	Response Response
}

// DeleteNetworkContainerRequest specifies the details about the request to delete a specifc network container.
type DeleteNetworkContainerRequest struct {
	NetworkContainerid string
}

// DeleteNetworkContainerResponse describes the response to delete a specifc network container.
type DeleteNetworkContainerResponse struct {
	Response Response
}

// GetInterfaceForContainerRequest specifies the container ID for which interface needs to be identified.
type GetInterfaceForContainerRequest struct {
	NetworkContainerID string
}

// GetInterfaceForContainerResponse specifies the interface for a given container ID.
type GetInterfaceForContainerResponse struct {
	NetworkInterface NetworkInterface
	VnetAddressSpace []IPSubnet
	Response         Response
}

// NetworkInterface specifies the information that can be used to unquely identify an interface.
type NetworkInterface struct {
	Name      string
	IPAddress string
}
