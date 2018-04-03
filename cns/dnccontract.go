package cns

// Container Network Service DNC Contract
const (
	CreateOrUpdateNetworkContainer = "/network/createorupdatenetworkcontainer"
	DeleteNetworkContainer         = "/network/deletenetworkcontainer"
	GetNetworkContainerStatus      = "/network/getnetworkcontainerstatus"
	GetInterfaceForContainer       = "/network/getinterfaceforcontainer"
)

// Orchestrator Types
const (
	AzureContainerInstance = "AzureContainerInstance"
)

// CreateNetworkContainerRequest specifies request to create a network container or network isolation boundary.
type CreateNetworkContainerRequest struct {
	Version                    string
	NetworkContainerType       string
	NetworkContainerid         string // Mandatory input.
	PrimaryInterfaceIdentifier string // Primary CA.
	AuthorizationToken         string
	OrchestratorInfo           OrchestratorInfo
	IPConfiguration            IPConfiguration
	MultiTenancyInfo           MultiTenancyInfo
	VnetAddressSpace           []IPSubnet // To setup SNAT (should include service endpoint vips).
	Routes                     []Route
}

// OrchestratorInfo contains orchestrator type which is used to cast OrchestratorContext.
type OrchestratorInfo struct {
	OrchestratorType    string
	OrchestratorContext interface{}
}

// AzureContainerInstanceInfo is an OrchestratorContext that holds PodName and PodNamespace.
type AzureContainerInstanceInfo struct {
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
