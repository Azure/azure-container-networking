package cns

import "encoding/json"

// Container Network Service DNC Contract
const (
	SetOrchestratorType                      = "/network/setorchestratortype"
	CreateOrUpdateNetworkContainer           = "/network/createorupdatenetworkcontainer"
	DeleteNetworkContainer                   = "/network/deletenetworkcontainer"
	GetNetworkContainerStatus                = "/network/getnetworkcontainerstatus"
	GetInterfaceForContainer                 = "/network/getinterfaceforcontainer"
	GetNetworkContainerByOrchestratorContext = "/network/getnetworkcontainerbyorchestratorcontext"
	AttachContainerToNetwork                 = "/network/attachcontainertonetwork"
	DetachContainerFromNetwork               = "/network/detachcontainerfromnetwork"
)

// NetworkContainer Prefixes
const (
	SwiftPrefix = "Swift_"
)

// NetworkContainer Types
const (
	AzureContainerInstance = "AzureContainerInstance"
	WebApps                = "WebApps"
	Docker                 = "Docker"
	Basic                  = "Basic"
	JobObject              = "JobObject"
	COW                    = "COW" // Container on Windows
)

// Orchestrator Types
const (
	Kubernetes      = "Kubernetes"
	ServiceFabric   = "ServiceFabric"
	Batch           = "Batch"
	DBforPostgreSQL = "DBforPostgreSQL"
	AzureFirstParty = "AzureFirstParty"
)

// Encap Types
const (
	Vlan  = "Vlan"
	Vxlan = "Vxlan"
)

// CreateNetworkContainerRequest specifies request to create a network container or network isolation boundary.
type CreateNetworkContainerRequest struct {
	Version                    string
	NetworkContainerType       string
	NetworkContainerid         string // Mandatory input.
	PrimaryInterfaceIdentifier string // Primary CA.
	AuthorizationToken         string
	LocalIPConfiguration       IPConfiguration
	OrchestratorContext        json.RawMessage
	IPConfiguration            IPConfiguration
	MultiTenancyInfo           MultiTenancyInfo
	CnetAddressSpace           []IPSubnet // To setup SNAT (should include service endpoint vips).
	Routes                     []Route
	AllowHostToNCCommunication bool
	AllowNCToHostCommunication bool
}

// ConfigureContainerNetworkingRequest - specifies request to attach/detach container to network.
type ConfigureContainerNetworkingRequest struct {
	Containerid        string
	NetworkContainerid string
}

// KubernetesPodInfo is an OrchestratorContext that holds PodName and PodNamespace.
type KubernetesPodInfo struct {
	PodName      string
	PodNamespace string
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

// SetOrchestratorTypeRequest specifies the orchestrator type for the node.
type SetOrchestratorTypeRequest struct {
	OrchestratorType string
	DncPartitionKey  string
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
	NetworkContainerid  string
	OrchestratorContext json.RawMessage
}

// GetNetworkContainerResponse describes the response to retrieve a specifc network container.
type GetNetworkContainerResponse struct {
	IPConfiguration            IPConfiguration
	Routes                     []Route
	CnetAddressSpace           []IPSubnet
	MultiTenancyInfo           MultiTenancyInfo
	PrimaryInterfaceIdentifier string
	LocalIPConfiguration       IPConfiguration
	Response                   Response
	AllowHostToNCCommunication bool
	AllowNCToHostCommunication bool
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
	NetworkContainerVersion string
	NetworkInterface        NetworkInterface
	CnetAddressSpace        []IPSubnet
	DNSServers              []string
	Response                Response
}

// AttachContainerToNetworkResponse specifies response of attaching network container to network.
type AttachContainerToNetworkResponse struct {
	Response Response
}

// DetachContainerFromNetworkResponse specifies response of detaching network container from network.
type DetachContainerFromNetworkResponse struct {
	Response Response
}

// NetworkInterface specifies the information that can be used to unquely identify an interface.
type NetworkInterface struct {
	Name      string
	IPAddress string
}
